// Package database provides the pgxpool factory matching the Rust
// shared/database.rs setup.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Config is the Postgres connection config. Zero-valued limits fall back
// to pgxpool defaults.
type Config struct {
	URL                string
	MaxConnections     int
	MinConnections     int
	MaxLifetimeSeconds int
}

// NewPool creates a pgxpool.Pool tuned to FlowCatalyst defaults.
func NewPool(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	return NewPoolWithBeforeConnect(ctx, cfg, nil)
}

// NewPoolWithBeforeConnect is NewPool with a per-connection BeforeConnect hook.
// The hook runs before every new connection is established (including pool
// growth + lifetime-recycled connections), so it can inject freshly-rotated
// credentials — used by the AWS Secrets Manager rotation refresher. nil hook =
// plain NewPool behaviour.
func NewPoolWithBeforeConnect(ctx context.Context, cfg Config, beforeConnect func(context.Context, *pgx.ConnConfig) error) (*pgxpool.Pool, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("database URL is empty")
	}
	pgCfg, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse db URL: %w", err)
	}
	if beforeConnect != nil {
		pgCfg.BeforeConnect = beforeConnect
	}
	if cfg.MaxConnections > 0 {
		pgCfg.MaxConns = int32(cfg.MaxConnections)
	}
	if cfg.MinConnections > 0 {
		pgCfg.MinConns = int32(cfg.MinConnections)
	}
	if cfg.MaxLifetimeSeconds > 0 {
		pgCfg.MaxConnLifetime = time.Duration(cfg.MaxLifetimeSeconds) * time.Second
	}
	pool, err := pgxpool.NewWithConfig(ctx, pgCfg)
	if err != nil {
		return nil, fmt.Errorf("pgxpool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return pool, nil
}
