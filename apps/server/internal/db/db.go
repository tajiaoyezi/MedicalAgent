// Package db 提供 gorm 运行时连接与租户作用域。schema 由 SQL 迁移拥有（见 migrate.go），不使用 AutoMigrate。
package db

import (
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open 建立 gorm 运行时连接（底层 pgx stdlib）。
func Open(databaseURL string) (*gorm.DB, error) {
	gdb, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, err
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxIdleTime(30 * time.Second)
	return gdb, nil
}

// TenantScope 显式租户过滤（001 未建 RLS，靠显式 WHERE tenant_id 隔离）。
func TenantScope(tenantID string) func(*gorm.DB) *gorm.DB {
	return func(d *gorm.DB) *gorm.DB { return d.Where("tenant_id = ?", tenantID) }
}
