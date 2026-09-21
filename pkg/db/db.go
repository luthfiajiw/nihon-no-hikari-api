package db

import (
	"context"

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
