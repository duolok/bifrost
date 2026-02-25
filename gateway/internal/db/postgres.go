package db

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	maxDatabaseConnections = 20
	minDatabaseConnections = 2
	maxConnectionLifetime  = 30 * time.Minute
	maxConnectionIdleTime  = 5 * time.Minute
)

func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database config: %w", err)
	}

	config.MaxConns = maxDatabaseConnections
	config.MinConns = minDatabaseConnections
	config.MaxConnLifetime = maxConnectionLifetime
	config.MaxConnIdleTime = maxConnectionIdleTime

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("database ping failed: %w", err)
	}

	stats := pool.Stat()
	slog.Info("database connected",
		"max_conns", config.MaxConns,
		"min_conns", config.MinConns,
		"total_conns", stats.TotalConns(),
	)

	return pool, nil
}
