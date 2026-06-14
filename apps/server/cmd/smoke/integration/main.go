// integration smoke：登录 → /api/me → 上传 → 下载 → 删除 → 审计。等价 apps/api/src/scripts/integration-smoke.ts。
// 需 docker PG+MinIO。in-process httptest 起 Go 服务，避免依赖外部已启动实例。
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"

	"medoffice/server/internal/config"
	"medoffice/server/internal/db"
	"medoffice/server/internal/server"
	"medoffice/server/internal/storage"
)

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

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	// 登录
	lb, _ := json.Marshal(map[string]string{"username": "admin", "password": "admin123"})
	res, err := client.Post(base+"/api/auth/login", "application/json", bytes.NewReader(lb))
	must(err, "login")
	if res.StatusCode != 200 {
		log.Fatalf("login failed: %d", res.StatusCode)
	}
	res.Body.Close()

	// 会话
	res, err = client.Get(base + "/api/me")
	must(err, "me")
	if res.StatusCode != 200 {
		log.Fatalf("session failed: %d", res.StatusCode)
	}
	res.Body.Close()
	fmt.Println("login + me OK")

	// 上传
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("file", "smoke.txt")
	fw.Write([]byte("integration test"))
	w.WriteField("space", "my")
	w.Close()
	req, _ := http.NewRequest("POST", base+"/api/documents/upload", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	res, err = client.Do(req)
	must(err, "upload")
	if res.StatusCode != 200 {
		b, _ := io.ReadAll(res.Body)
		log.Fatalf("upload failed: %d %s", res.StatusCode, b)
	}
	var up struct {
		DocumentID string `json:"documentId"`
		FileHash   string `json:"fileHash"`
	}
	json.NewDecoder(res.Body).Decode(&up)
	res.Body.Close()
	fmt.Println("upload OK", up.DocumentID, up.FileHash)

	// 下载
	res, err = client.Get(base + "/api/documents/" + up.DocumentID + "/download")
	must(err, "download")
	if res.StatusCode != 200 {
		log.Fatalf("download failed: %d", res.StatusCode)
	}
	res.Body.Close()
	fmt.Println("download OK")

	// 删除
	req, _ = http.NewRequest("DELETE", base+"/api/documents/"+up.DocumentID, nil)
	res, err = client.Do(req)
	must(err, "delete")
	if res.StatusCode != 200 {
		log.Fatalf("delete failed: %d", res.StatusCode)
	}
	res.Body.Close()
	fmt.Println("delete OK")

	// 审计
	res, err = client.Get(base + "/api/admin/audit-logs")
	must(err, "audit")
	if res.StatusCode != 200 {
		log.Fatalf("audit failed: %d", res.StatusCode)
	}
	var logs struct {
		Logs []json.RawMessage `json:"logs"`
	}
	json.NewDecoder(res.Body).Decode(&logs)
	res.Body.Close()
	fmt.Println("audit entries:", len(logs.Logs))

	fmt.Println("Integration smoke passed")
}

func must(err error, where string) {
	if err != nil {
		log.Fatalf("%s: %v", where, err)
	}
}
