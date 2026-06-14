package db

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate 复刻 migrate.ts runMigrations：建 schema_migrations，按文件名字典序应用未执行的 .sql 并登记。
// 多语句 .sql 用 pgx 简单协议（Exec 无参数）执行，等价 pg client.query(sql)。
func Migrate(ctx context.Context, conn *pgx.Conn) error {
	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`); err != nil {
		return err
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	var files []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, f := range files {
		version := strings.TrimSuffix(f, ".sql")
		var one int
		err := conn.QueryRow(ctx, `SELECT 1 FROM schema_migrations WHERE version = $1`, version).Scan(&one)
		if err == nil {
			fmt.Printf("Skip migration %s\n", version)
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("check migration %s: %w", version, err)
		}
		sqlBytes, err := migrationsFS.ReadFile("migrations/" + f)
		if err != nil {
			return err
		}
		if _, err := conn.Exec(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("migration %s failed: %w", version, err)
		}
		if _, err := conn.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1) ON CONFLICT (version) DO NOTHING`, version); err != nil {
			return err
		}
		fmt.Printf("Applied migration %s\n", version)
	}
	return nil
}
