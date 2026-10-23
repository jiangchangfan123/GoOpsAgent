// Package store 统一数据访问层
// 所有数据库和 Redis 操作集中在此包中，通过 Store struct 暴露。
// 参考 Grafana SqlStore / PocketBase app 模式：一个 struct，按文件拆分方法。
package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	_ "github.com/go-sql-driver/mysql"
	goredis "github.com/redis/go-redis/v9"
)

// Store 统一数据访问入口，持有 DB 连接池和 Redis 客户端
type Store struct {
	db    *sql.DB
	redis *goredis.Client
}

// New 创建 Store 实例
func New(db *sql.DB, rdb *goredis.Client) *Store {
	return &Store{db: db, redis: rdb}
}

// NewDB 创建 MySQL 连接池并运行自动迁移（幂等）
func NewDB(dsn string, maxOpen, maxIdle int, connMaxLifetime time.Duration) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}

	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(connMaxLifetime)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping mysql: %w", err)
	}

	slog.Info("mysql connected")

	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return db, nil
}

// NewRedis 创建 Redis 客户端
func NewRedis(addr, password string) (*goredis.Client, error) {
	rdb := goredis.NewClient(&goredis.Options{
		Addr:     addr,
		Password: password,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	slog.Info("redis connected", "addr", addr)
	return rdb, nil
}