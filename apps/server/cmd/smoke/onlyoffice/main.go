// onlyoffice smoke：JWT 签验、编辑器配置签发、文件类型路由、会话查找。等价 apps/api/src/scripts/onlyoffice-smoke.ts。
// 离线降级：DS/API 未起时仍可通过配置/JWT/路由自检。需 MinIO（构造 storage）。
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"medoffice/server/internal/config"
	"medoffice/server/internal/docperm"
	"medoffice/server/internal/editor"
	"medoffice/server/internal/storage"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()
	store, err := storage.New(ctx, cfg.Storage)
	if err != nil {
		log.Fatalf("storage: %v", err)
	}
	svc := editor.NewService(cfg.OnlyOffice, store)

	fmt.Println("ONLYOFFICE bridge smoke...")
	checkJWTRoundtrip(svc)
	checkEditorConfig(cfg.OnlyOffice, svc)
	checkDsHealth(cfg.OnlyOffice)
	fmt.Println("ONLYOFFICE smoke passed（离线降级：DS 未启动时仍可通过配置/JWT/路由自检）")
}

func checkJWTRoundtrip(svc *editor.Service) {
	token := svc.JWT.Sign(map[string]any{"test": true, "document": map[string]any{"key": "smoke-key"}})
	if token == "" {
		fmt.Println("  JWT 未启用，跳过签名检查")
		return
	}
	if _, ok := svc.JWT.Verify(token); !ok {
		log.Fatal("JWT 验签失败")
	}
	fmt.Println("  JWT 签名/验签 OK")
}

func checkEditorConfig(cfg config.OnlyOffice, svc *editor.Service) {
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
		Session: session, Filename: "smoke.docx", DocumentType: "word",
		Permission: docperm.Edit, UserID: session.UserID, DisplayName: "Smoke User",
	})
	if ec["documentType"] == nil {
		log.Fatal("编辑器配置缺少顶层 documentType")
	}
	doc, _ := ec["document"].(map[string]any)
	if doc == nil || doc["key"] == nil {
		log.Fatal("JWT 包装后顶层必须保留 document")
	}
	if cfg.JWTEnabled && ec["token"] == nil {
		log.Fatal("JWT 启用时配置顶层必须包含 token")
	}
	if editor.ResolveEditorRoute("report.xlsx").DocumentType != "cell" {
		log.Fatal("xlsx 路由错误")
	}
	if editor.ResolveEditorRoute("archive.zip").Route != editor.RouteUnsupported {
		log.Fatal("zip 应被拒绝")
	}
	if svc.Sessions.GetByOpenToken(session.OpenToken) == nil {
		log.Fatal("open_token 会话丢失")
	}
	fmt.Println("  编辑器配置签发 OK")
	fmt.Println("  文件类型路由 OK")
}

func checkDsHealth(cfg config.OnlyOffice) {
	res, err := http.Get(cfg.DSURL + "/healthcheck")
	if err != nil || res.StatusCode != 200 {
		fmt.Println("  ONLYOFFICE DS 未就绪（", cfg.DSURL, "）— 跳过 DS 在线检查")
		return
	}
	res.Body.Close()
	fmt.Println("  ONLYOFFICE DS healthcheck OK")
}
