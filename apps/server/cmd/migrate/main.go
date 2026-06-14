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
	if err := db.Seed(ctx, conn); err != nil {
		log.Fatalf("seed: %v", err)
	}
	log.Println("migrate + seed done")
}
