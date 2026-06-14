package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"medoffice/server/internal/config"
	"medoffice/server/internal/db"
	"medoffice/server/internal/parsing"
	"medoffice/server/internal/server"
	"medoffice/server/internal/storage"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	gormDB, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	store, err := storage.New(ctx, cfg.Storage)
	if err != nil {
		log.Fatalf("storage: %v", err)
	}

	engine := server.New(server.Deps{Config: cfg, DB: gormDB, Storage: store})

	srv := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Handler: engine,
	}

	go func() {
		log.Printf("API listening on http://%s:%d", cfg.Host, cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	// c03 后台解析 worker（消费 document_events → 建作业 → 执行）。ctx 取消即停。
	workerCtx, cancelWorker := context.WithCancel(context.Background())
	parsing.StartWorker(workerCtx, gormDB, parsing.NewEngine(store), cfg.Model.ParseWorkerIntervalMs)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Println("shutting down...")
	cancelWorker()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
