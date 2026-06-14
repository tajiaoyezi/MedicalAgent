// authz smoke：证明跨租户/越权 4 类缺陷被拒。等价 apps/api/src/scripts/authz-smoke.ts。
// 直连 DB 种入第 2 个租户 B + bob，in-process httptest 起服务断言，结束清理租户 B。
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

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"medoffice/server/internal/config"
	"medoffice/server/internal/db"
	"medoffice/server/internal/server"
	"medoffice/server/internal/storage"
)

const tenantBName = "B医院-authz冒烟"

var pass int

func main() {
	cfg := config.Load()
	ctx := context.Background()
	gormDB, err := db.Open(cfg.DatabaseURL)
	must(err, "db.Open")
	store, err := storage.New(ctx, cfg.Storage)
	must(err, "storage")
	engine := server.New(server.Deps{Config: cfg, DB: gormDB, Storage: store})
	ts := httptest.NewServer(engine)
	defer ts.Close()
	base := ts.URL

	// 租户 A（admin 所在）与普通用户 user
	var tenantA, tenantAName string
	must(gormDB.Raw(`SELECT t.tenant_id FROM tenants t JOIN users u ON u.tenant_id = t.tenant_id WHERE u.username = 'admin' LIMIT 1`).Scan(&tenantA).Error, "tenantA")
	must(gormDB.Raw(`SELECT name FROM tenants WHERE tenant_id = ?`, tenantA).Scan(&tenantAName).Error, "tenantAName")
	var normalUserID string
	_ = gormDB.Raw(`SELECT user_id FROM users WHERE tenant_id = ? AND username = 'user' LIMIT 1`, tenantA).Scan(&normalUserID).Error

	// 清理可能残留的同名测试租户（使脚本可重复运行）
	var stale []string
	_ = gormDB.Raw(`SELECT tenant_id FROM tenants WHERE name = ?`, tenantBName).Scan(&stale).Error
	for _, id := range stale {
		purgeTenant(gormDB, id)
	}

	// 种入租户 B + 角色 + bob
	var tenantB string
	must(gormDB.Raw(`INSERT INTO tenants (name) VALUES (?) RETURNING tenant_id`, tenantBName).Scan(&tenantB).Error, "seed tenantB")
	var roleB string
	must(gormDB.Raw(`INSERT INTO roles (tenant_id, name, slug) VALUES (?, '普通用户', 'user') RETURNING role_id`, tenantB).Scan(&roleB).Error, "seed roleB")
	hash, _ := bcrypt.GenerateFromPassword([]byte("bob123"), 10)
	var bobID string
	must(gormDB.Raw(`INSERT INTO users (tenant_id, username, password_hash, display_name) VALUES (?, 'bob', ?, 'Bob') RETURNING user_id`, tenantB, string(hash)).Scan(&bobID).Error, "seed bob")
	must(gormDB.Exec(`INSERT INTO user_roles (user_id, role_id) VALUES (?, ?)`, bobID, roleB).Error, "seed user_roles")

	var docID string
	defer func() {
		if docID != "" {
			_ = gormDB.Exec(`DELETE FROM document_events WHERE document_id = ?`, docID).Error
			_ = gormDB.Exec(`DELETE FROM documents WHERE document_id = ?`, docID).Error
		}
		purgeTenant(gormDB, tenantB)
	}()

	fmt.Println("#4 登录租户隔离")
	st, _ := login(base, "admin", "admin123", "")
	ok(st == 400, "多租户下未指定租户的登录被拒(400)")
	stAdminA, adminA := login(base, "admin", "admin123", tenantAName)
	ok(stAdminA == 200, "指定租户 A 的 admin 登录成功(200)")
	stBobA, _ := login(base, "bob", "bob123", tenantAName)
	ok(stBobA == 401, "bob 在租户 A 下登录被拒(401，跨租户同名隔离)")
	stBobB, _ := login(base, "bob", "bob123", tenantBName)
	ok(stBobB == 200, "bob 在租户 B 下登录成功(200)")

	fmt.Println("#1 管理员跨租户 PATCH 用户")
	ok(doJSON(adminA, "PATCH", base+"/api/admin/users/"+bobID, map[string]any{"isEnabled": false}) == 404, "admin(租户A) 改租户B 的 bob 被拒(404)")

	fmt.Println("#2 文档授权写入 principal 校验")
	upStatus, upBody := uploadDoc(adminA, base)
	ok(upStatus == 200 || upStatus == 201, "admin 上传测试文档成功")
	docID = upBody.DocumentID
	ok(doJSON(adminA, "POST", base+"/api/documents/"+docID+"/permissions", map[string]any{"principalType": "user", "principalId": bobID, "permissionLevel": "view"}) == 400, "给外租户用户 bob 授权被拒(400)")
	ok(doJSON(adminA, "POST", base+"/api/documents/"+docID+"/permissions", map[string]any{"principalType": "role", "principalId": "不存在的角色", "permissionLevel": "view"}) == 400, "给不存在的角色授权被拒(400)")

	if normalUserID != "" {
		fmt.Println("#3 禁用后重新启用可再登录")
		doJSON(adminA, "PATCH", base+"/api/admin/users/"+normalUserID, map[string]any{"isEnabled": false})
		stDisabled, _ := login(base, "user", "user123", tenantAName)
		ok(stDisabled == 403, "禁用后 user 登录被拒(403)")
		doJSON(adminA, "PATCH", base+"/api/admin/users/"+normalUserID, map[string]any{"isEnabled": true})
		stReenabled, _ := login(base, "user", "user123", tenantAName)
		ok(stReenabled == 200, "重新启用后 user 登录成功(200，revoke 已清)")
	}

	fmt.Printf("\nauthz 冒烟通过：%d 条断言全部成立\n", pass)
}

func login(base, username, password, tenant string) (int, *http.Client) {
	jar, _ := cookiejar.New(nil)
	cl := &http.Client{Jar: jar}
	b, _ := json.Marshal(map[string]string{"username": username, "password": password, "tenant": tenant})
	res, err := cl.Post(base+"/api/auth/login", "application/json", bytes.NewReader(b))
	must(err, "login "+username)
	res.Body.Close()
	return res.StatusCode, cl
}

func doJSON(cl *http.Client, method, url string, body any) int {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(method, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	res, err := cl.Do(req)
	must(err, method+" "+url)
	res.Body.Close()
	return res.StatusCode
}

func uploadDoc(cl *http.Client, base string) (int, struct {
	DocumentID string `json:"documentId"`
}) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("file", "authz.txt")
	fw.Write([]byte("authz"))
	w.WriteField("space", "my")
	w.Close()
	req, _ := http.NewRequest("POST", base+"/api/documents/upload", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	res, err := cl.Do(req)
	must(err, "upload")
	var out struct {
		DocumentID string `json:"documentId"`
	}
	json.NewDecoder(res.Body).Decode(&out)
	res.Body.Close()
	return res.StatusCode, out
}

func purgeTenant(db *gorm.DB, tenantID string) {
	stmts := []string{
		`DELETE FROM audit_logs WHERE tenant_id = ?`,
		`DELETE FROM recent_tasks WHERE tenant_id = ?`,
		`DELETE FROM document_events WHERE tenant_id = ?`,
		`DELETE FROM documents WHERE tenant_id = ?`,
		`DELETE FROM user_roles WHERE user_id IN (SELECT user_id FROM users WHERE tenant_id = ?)`,
		`DELETE FROM users WHERE tenant_id = ?`,
		`DELETE FROM role_permissions WHERE role_id IN (SELECT role_id FROM roles WHERE tenant_id = ?)`,
		`DELETE FROM roles WHERE tenant_id = ?`,
		`DELETE FROM tenants WHERE tenant_id = ?`,
	}
	for _, s := range stmts {
		_ = db.Exec(s, tenantID).Error
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
