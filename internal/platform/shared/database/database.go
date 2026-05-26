// Package database provides the pgxpool factory matching the Rust
// shared/database.rs setup.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/config"
)

// NewPool creates a pgxpool.Pool tuned to FlowCatalyst defaults.
func NewPool(ctx context.Context, cfg config.DBConfig) (*pgxpool.Pool, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("database URL is empty")
	}
	pgCfg, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse db URL: %w", err)
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
