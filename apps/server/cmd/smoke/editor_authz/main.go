// editor-authz smoke：comment-only 配置、写回意图 arm/peek/confirm、JWT 包装形状、DS URL 白名单、
// parse-status 表缺失降级、回调 JWT 必需。等价 apps/api/src/scripts/editor-authz-smoke.ts。
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"

	"gorm.io/gorm"

	"medoffice/server/internal/config"
	"medoffice/server/internal/db"
	"medoffice/server/internal/docperm"
	"medoffice/server/internal/editor"
	"medoffice/server/internal/server"
	"medoffice/server/internal/storage"
)

var pass int

func main() {
	cfg := config.Load()
	ctx := context.Background()
	gormDB, err := db.Open(cfg.DatabaseURL)
	must(err, "db.Open")
	store, err := storage.New(ctx, cfg.Storage)
	must(err, "storage")
	svc := editor.NewService(cfg.OnlyOffice, store)
	engine := server.New(server.Deps{Config: cfg, DB: gormDB, Storage: store})
	ts := httptest.NewServer(engine)
	defer ts.Close()

	fmt.Println("c02 editor-authz smoke...")
	testCommentOnlyConfig(cfg.OnlyOffice, svc)
	testWritebackPeekStatus(svc)
	testJWTConfigShape(cfg.OnlyOffice, svc)
	testDsURLAllowlist(cfg.OnlyOffice)
	testParseStatusWhenTableMissing(gormDB, ts.URL)
	testCallbackJWTRequired(cfg.OnlyOffice, ts.URL)
	fmt.Printf("editor-authz smoke passed (%d assertions)\n", pass)
}

func testCommentOnlyConfig(cfg config.OnlyOffice, svc *editor.Service) {
	svc.Sessions.ClearAll()
	session := svc.Sessions.Create(editor.CreateInput{
		DocumentID:  "00000000-0000-0000-0000-000000000001",
		DocumentKey: editor.BuildDocumentKey("00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002"),
		TenantID:    "00000000-0000-0000-0000-000000000099",
		UserID:      "00000000-0000-0000-0000-000000000003",
		VersionID:   "00000000-0000-0000-0000-000000000002",
		Revision:    "abc123",
	})
	ec := editor.BuildEditorConfig(cfg, svc.JWT, editor.BuildConfigInput{
		Session: session, Filename: "note.docx", DocumentType: "word",
		Permission: docperm.Comment, UserID: session.UserID, DisplayName: "Comment User",
	})
	editorCfg, _ := ec["editorConfig"].(map[string]any)
	doc, _ := ec["document"].(map[string]any)
	perms, _ := doc["permissions"].(map[string]any)
	editFlag, _ := perms["edit"].(bool)
	commentFlag, _ := perms["comment"].(bool)
	ok(editorCfg["mode"] == "edit", "comment-only mode 为 edit")
	ok(!editFlag, "comment-only permissions.edit=false")
	ok(commentFlag, "comment-only permissions.comment=true")
	ok(editorCfg["callbackUrl"] != nil, "comment-only 有 callbackUrl")
}

func testWritebackPeekStatus(svc *editor.Service) {
	svc.Sessions.ClearAll()
	session := svc.Sessions.Create(editor.CreateInput{
		DocumentID: "00000000-0000-0000-0000-000000000001", DocumentKey: "key1",
		TenantID: "00000000-0000-0000-0000-000000000099", UserID: "00000000-0000-0000-0000-000000000003",
		VersionID: "00000000-0000-0000-0000-000000000002", Revision: "rev1",
	})
	intentID := svc.Sessions.CreateSaveIntent(session, "ai_writeback")
	_, has := svc.Sessions.PeekPendingWritebackSave(session, 6)
	ok(!has, "未 arm 时 status=6 不消费")
	ok(svc.Sessions.ArmWritebackSaveIntent(session, intentID), "arm 写回意图成功")
	_, has = svc.Sessions.PeekPendingWritebackSave(session, 2)
	ok(!has, "armed 后 status=2 仍为 user_edit")
	src, has := svc.Sessions.PeekPendingWritebackSave(session, 6)
	ok(has && src == "ai_writeback", "armed 后 status=6 可 peek ai_writeback")
	svc.Sessions.ConfirmPendingWritebackSave(session)
	_, has = svc.Sessions.PeekPendingWritebackSave(session, 6)
	ok(!has, "confirm 后不再 peek")
}

func testJWTConfigShape(cfg config.OnlyOffice, svc *editor.Service) {
	wrapped := svc.JWT.Wrap(map[string]any{
		"documentType": "word",
		"document":     map[string]any{"key": "k"},
		"editorConfig": map[string]any{"mode": "edit"},
	})
	ok(wrapped["documentType"] != nil, "JWT 包装保留 documentType")
	ok(wrapped["document"] != nil, "JWT 包装保留 document")
	if cfg.JWTEnabled {
		ok(wrapped["token"] != nil, "JWT 启用时含 token")
	}
}

func testDsURLAllowlist(cfg config.OnlyOffice) {
	dsBase, _ := url.Parse(cfg.DSURL)
	allowed := dsBase.Scheme + "://" + dsBase.Host + "/cache/files/docx"
	ok(editor.AssertDsDownloadURL(cfg.DSURL, allowed) == nil, "DS 同源下载 URL 通过")
	ok(editor.AssertDsDownloadURL(cfg.DSURL, "http://evil.example.com/steal") != nil, "非 DS 主机 URL 被拒绝")
}

func testParseStatusWhenTableMissing(gormDB *gorm.DB, base string) {
	var reg *string
	_ = gormDB.Raw(`SELECT to_regclass('public.document_parse_jobs')::text AS t`).Scan(&reg).Error
	if reg != nil && *reg != "" {
		fmt.Println("  ⚠ document_parse_jobs 已存在（c03 已落地），跳过表缺失 parse-status 端点断言")
		return
	}
	var tenantName string
	_ = gormDB.Raw(`SELECT t.name FROM tenants t JOIN users u ON u.tenant_id = t.tenant_id WHERE u.username = 'admin' LIMIT 1`).Scan(&tenantName).Error
	if tenantName == "" {
		fmt.Println("  ⚠ 无 admin 种子用户，跳过 parse-status 端点检查")
		return
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	lb, _ := json.Marshal(map[string]string{"username": "admin", "password": "admin123", "tenant": tenantName})
	res, err := client.Post(base+"/api/auth/login", "application/json", bytes.NewReader(lb))
	if err != nil || res.StatusCode != 200 {
		log.Fatalf("login for parse-status failed")
	}
	res.Body.Close()

	var documentID string
	_ = gormDB.Raw(`SELECT d.document_id FROM documents d JOIN users u ON u.tenant_id = d.tenant_id WHERE u.username = 'admin' AND d.is_deleted = FALSE LIMIT 1`).Scan(&documentID).Error
	if documentID == "" {
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		fw, _ := w.CreateFormFile("file", "smoke-parse.png")
		fw.Write([]byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a})
		w.WriteField("space", "my")
		w.Close()
		req, _ := http.NewRequest("POST", base+"/api/documents/upload", &buf)
		req.Header.Set("Content-Type", w.FormDataContentType())
		up, _ := client.Do(req)
		var uploaded struct {
			DocumentID string `json:"documentId"`
		}
		json.NewDecoder(up.Body).Decode(&uploaded)
		up.Body.Close()
		documentID = uploaded.DocumentID
		ok(documentID != "", "无种子文档时上传 smoke-parse.png 供 parse-status 用例")
	}

	res2, _ := client.Get(base + "/api/preview/" + documentID + "/parse-status")
	var pbody struct {
		Status  string `json:"status"`
		Jobs    []any  `json:"jobs"`
		Message string `json:"message"`
	}
	json.NewDecoder(res2.Body).Decode(&pbody)
	code := res2.StatusCode
	res2.Body.Close()
	ok(code == 200, "表缺失时 GET /parse-status 返回 200")
	ok(pbody.Status == "pending", "表缺失时 parse-status JSON status=pending")
	ok(len(pbody.Jobs) == 0, "表缺失时 parse-status jobs=[]")
	ok(pbody.Message != "", "表缺失时 parse-status 含说明 message")
}

func testCallbackJWTRequired(cfg config.OnlyOffice, base string) {
	body, _ := json.Marshal(map[string]any{"status": 2, "url": "http://evil.test/x"})
	res, err := http.Post(base+"/api/editor/callback?token=fake", "application/json", bytes.NewReader(body))
	if err != nil {
		log.Fatalf("callback: %v", err)
	}
	res.Body.Close()
	if cfg.JWTEnabled {
		ok(res.StatusCode == 403, "JWT 开启时无 body.token 回调返回 403")
	}
}

func ok(cond bool, msg string) {
	if !cond {
		log.Fatalf("断言失败: %s", msg)
	}
	pass++
	fmt.Println("  ✓", msg)
}

func must(err error, where string) {
	if err != nil {
		log.Fatalf("%s: %v", where, err)
	}
}
