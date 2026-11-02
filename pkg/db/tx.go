package db

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Querier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type WithTx interface {
	Exec(ctx context.Context, fn func(tx pgx.Tx) error) error
}

type Tx struct {
	Pool *pgxpool.Pool
}

func NewTx(pool *pgxpool.Pool) *Tx {
	return &Tx{Pool: pool}
}

func (t *Tx) Exec(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := t.Pool.Begin(ctx)
	if err != nil {
		return err
	}

	if err := fn(tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}

	return tx.Commit(ctx)
}
