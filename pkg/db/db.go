package db

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPostgresPool(connsString string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(connsString)
	if err != nil {
		return nil, err
	}

	// Disable statement caching to fix "prepared statement already exists"
	// errors when using connection poolers like PgBouncer in transaction mode.
	config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec

	// Keep warm connections ready and allow parallel frontend calls without queuing,
	// unless overridden explicitly via DSN pool_* parameters.
	if !strings.Contains(connsString, "pool_max_conns") {
		config.MaxConns = 10
	}
	if !strings.Contains(connsString, "pool_min_conns") {
		config.MinConns = 2
	}
	config.MaxConnIdleTime = 5 * time.Minute
	config.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}
