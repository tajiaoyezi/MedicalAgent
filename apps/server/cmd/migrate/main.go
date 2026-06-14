package main

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5"

	"medoffice/server/internal/config"
	"medoffice/server/internal/db"
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
	log.Println("migrate done")
}
