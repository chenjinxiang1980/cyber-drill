// Package db 提供 MySQL 连接池与基础操作封装。
package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"drill-system/internal/config"

	_ "github.com/go-sql-driver/mysql"
)

// DB 是对 *sql.DB 的薄封装，附带上下文/日志辅助方法。
type DB struct {
	*sql.DB
	cfg *config.Config
}

// Open 根据配置建立连接池并 ping。失败时返回 error（由调用方决定是否 Fatal）。
func Open(cfg *config.Config) (*DB, error) {
	pool, err := sql.Open("mysql", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("打开 MySQL 连接失败: %w", err)
	}
	// mysql driver 内部是懒创建连接，必须 Ping 才能确认可达
	// 10/5 稳定默认值 — 真正的 race condition 根因是 handler 内 rows.Next() 未 Close 就发新 Query/Exec
	pool.SetMaxOpenConns(10)
	pool.SetMaxIdleConns(5)
	pool.SetConnMaxLifetime(10 * time.Minute)
	pool.SetConnMaxIdleTime(1 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := pool.PingContext(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("MySQL Ping 失败(%s:%d/%s): %w",
			cfg.DBHost, cfg.DBPort, cfg.DBName, err)
	}
	slog.Info("MySQL 连接成功",
		"host", cfg.DBHost, "port", cfg.DBPort,
		"db", cfg.DBName, "user", cfg.DBUser)
	return &DB{DB: pool, cfg: cfg}, nil
}

// Now 返回本地时区的 YYYY-MM-DD HH:mm:ss 字符串（与 SQLite 旧库格式一致）。
// MySQL DATETIME 直接存这个字符串即可（driver 已 parseTime=true）。
func Now() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

// NowSec 与 Node 版 util.nowSec 对齐：YYYY-MM-DD HH:mm（秒可选）
func NowSec() string {
	return time.Now().Format("2006-01-02 15:04")
}
