package main

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5"

	"medoffice/server/internal/config"
	"medoffice/server/internal/db"
	"medoffice/server/internal/knowledge"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)

	if err := db.Migrate(ctx, conn); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	// 安全：生产环境不注入已知口令的演示 admin/user，避免默认凭据落到生产。
	if cfg.NodeEnv == "production" {
		log.Println("生产环境：跳过演示数据 seed")
	} else if err := db.Seed(ctx, conn); err != nil {
		log.Fatalf("seed: %v", err)
	}
	// 13 个预置知识库（§11.2）：平台级种子，幂等补齐每个租户（与演示账号 seed 分离，
	// 既有库也能补上 13 库；生产同样需要预置库，故不受 NodeEnv 跳过约束）。
	if err := db.SeedKnowledgeBases(ctx, conn); err != nil {
		log.Fatalf("seed knowledge bases: %v", err)
	}
	// 每个预置库装载 ≥1 份授权/开放演示文档（c06 tasks 2.3，c06 为唯一资产装载 owner）：经真实受控导入管线
	// （PreviewImport→ConfirmImport→HandleIndexReady）装载、幂等。该步用服务层（gorm），故另开一条 gorm 连接。
	g, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open gorm: %v", err)
	}
	if _, err := knowledge.SeedDemoDocuments(g); err != nil {
		log.Fatalf("seed demo documents: %v", err)
	}
	log.Println("migrate done")
}
