// infra smoke：对象存储 put/head/get/hash/presign/delete + /api/health（in-process）+ AES-GCM 往返与跨实现 golden。
// 需 docker-compose 起 PG+MinIO。等价 apps/api/src/scripts/smoke.ts + 跨实现加密验证。
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"time"

	"medoffice/server/internal/config"
	"medoffice/server/internal/cryptox"
	"medoffice/server/internal/db"
	"medoffice/server/internal/server"
	"medoffice/server/internal/storage"
)

// Node（apps/api crypto.ts）用 dev 占位密钥加密 "medoffice-golden-credential" 的产物，验证 Go 解密兼容。
const (
	nodeGolden  = "puO7HlsMsaK2qpEE.RmMevnKKx1frfC/yH/inVw==.NrbA2ZpEa9b+BxrxOr0IYzNhfYEKSL4d33Ow"
	goldenPlain = "medoffice-golden-credential"
	devSecret   = "dev-local-model-credential-do-not-deploy"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	// 1) 对象存储全链路
	st, err := storage.New(ctx, cfg.Storage)
	must(err, "storage.New")
	key := storage.ObjectKeyForVersion("smoke-tenant", "smoke-doc", "smoke-ver")
	body := []byte("medoffice smoke test")
	hash := storage.ComputeFileHash(body)

	fmt.Println("Object storage smoke...")
	must(st.Put(ctx, key, body, "text/plain"), "put")
	head, err := st.HeadObject(ctx, key)
	must(err, "head")
	fmt.Println("  headObject size:", head.Size)
	got, err := st.Get(ctx, key)
	must(err, "get")
	if storage.ComputeFileHash(got) != hash {
		log.Fatal("hash mismatch on get")
	}
	fmt.Println("  get OK")
	url, err := st.PresignedURL(ctx, key, 60*time.Second)
	must(err, "presign")
	fmt.Println("  presignedUrl:", trunc(url, 60))
	must(st.Delete(ctx, key), "delete")
	fmt.Println("  delete OK")

	// 2) /api/health（in-process httptest）
	gormDB, err := db.Open(cfg.DatabaseURL)
	must(err, "db.Open")
	engine := server.New(server.Deps{Config: cfg, DB: gormDB, Storage: st})
	ts := httptest.NewServer(engine)
	defer ts.Close()
	res, err := http.Get(ts.URL + "/api/health")
	must(err, "health")
	hb, _ := io.ReadAll(res.Body)
	res.Body.Close()
	fmt.Println("API health:", string(hb))
	if res.StatusCode != http.StatusOK {
		log.Fatalf("health status %d", res.StatusCode)
	}

	// 3) AES-GCM 往返 + 跨实现 golden（Node 加密 → Go 解密）
	codec := cryptox.New(cfg.Model.CredentialSecret)
	plain := "sk-medoffice-smoke-1234567890"
	if dec := codec.Decrypt(codec.Encrypt(plain)); dec != plain {
		log.Fatalf("crypto round-trip mismatch: %q", dec)
	}
	fmt.Println("  crypto round-trip OK")
	goldenCodec := cryptox.New(devSecret)
	if dec := goldenCodec.Decrypt(nodeGolden); dec != goldenPlain {
		log.Fatalf("cross-impl golden mismatch: got %q want %q", dec, goldenPlain)
	}
	fmt.Println("  cross-impl (Node→Go) golden OK")

	fmt.Println("infra smoke passed")
}

func must(err error, where string) {
	if err != nil {
		log.Fatalf("%s: %v", where, err)
	}
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
