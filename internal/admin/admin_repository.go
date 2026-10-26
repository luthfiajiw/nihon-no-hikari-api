package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"nihon-no-hikari-api/internal/admin/model"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository binds data operations to transactions owned by db.WithTx.
type Repository interface {
	UseTx(pgx.Tx) Repository
	Catalog(context.Context, *uuid.UUID) (CatalogData, error)
	Levels(context.Context) ([]Level, error)
	Templates(context.Context) ([]StrokeTemplate, error)
	ListContent(context.Context, string) ([]json.RawMessage, error)
	Content(context.Context, string, uuid.UUID) (json.RawMessage, error)
	ContentState(context.Context, string, uuid.UUID) (model.ContentState, error)
	Draft(context.Context, string, uuid.UUID) (json.RawMessage, error)
	SaveDraft(context.Context, string, uuid.UUID, json.RawMessage) error
	DeleteDraft(context.Context, string, uuid.UUID) error
	SaveCore(context.Context, string, uuid.UUID, coreInput, bool) error
	PrerequisiteState(context.Context, string, uuid.UUID, []uuid.UUID) (int, bool, error)
	SetPublished(context.Context, string, uuid.UUID, bool) error
	HasHistory(context.Context, string, uuid.UUID) (bool, error)
	Archive(context.Context, string, uuid.UUID) error
	DeleteContent(context.Context, string, uuid.UUID) (bool, error)
	RestoreContent(context.Context, string, uuid.UUID) error
	WithQuestions(context.Context, json.RawMessage, uuid.UUID) (json.RawMessage, error)
	QuestionSetState(context.Context, uuid.UUID) (model.QuestionSetState, error)
	InsertSet(context.Context, uuid.UUID, uuid.UUID, int, setInput) error
	UpdateSet(context.Context, uuid.UUID, setInput) error
	DeleteQuestions(context.Context, uuid.UUID) error
	InsertQuestions(context.Context, uuid.UUID, setInput) error
	LockSiblings(context.Context, string, uuid.UUID) ([]uuid.UUID, error)
	ApplyOrder(context.Context, string, ReorderRequest) error
	Template(context.Context, uuid.UUID, bool) (StrokeTemplateDetail, error)
	InsertTemplate(context.Context, StrokeTemplateDetail) error
	UpdateTemplate(context.Context, StrokeTemplateDetail) error
}

type dbQuerier interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type dbRepository struct {
	query dbQuerier
}

func NewRepository(pool *pgxpool.Pool) Repository { return &dbRepository{query: pool} }

// UseTx only binds queries; begin, commit and rollback belong to pkg/db.
func (r *dbRepository) UseTx(tx pgx.Tx) Repository {
	return &dbRepository{query: tx}
}

func databaseError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrConflict) || errors.Is(err, ErrInvalidRelation) {
		return err
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: %w", ErrNotFound, err)
	}
	var p *pgconn.PgError
	if errors.As(err, &p) {
		switch p.Code {
		case "23505":
			return fmt.Errorf("%w: %w", ErrConflict, err)
		case "23503", "23514":
			return fmt.Errorf("%w: %w", ErrInvalidRelation, err)
		}
	}
	return err
}

type databaseRow struct{ pgx.Row }

func (r databaseRow) Scan(dest ...any) error { return databaseError(r.Row.Scan(dest...)) }

type databaseRows struct{ pgx.Rows }

func (r databaseRows) Scan(dest ...any) error { return databaseError(r.Rows.Scan(dest...)) }
func (r databaseRows) Err() error             { return databaseError(r.Rows.Err()) }
func (r *dbRepository) QueryRow(ctx context.Context, q string, args ...any) pgx.Row {
	return databaseRow{r.query.QueryRow(ctx, q, args...)}
}

func (r *dbRepository) Query(ctx context.Context, q string, args ...any) (pgx.Rows, error) {
	rows, err := r.query.Query(ctx, q, args...)
	if err != nil {
		return nil, databaseError(err)
	}
	return databaseRows{rows}, nil
}

func (r *dbRepository) Exec(ctx context.Context, q string, args ...any) (pgconn.CommandTag, error) {
	tag, err := r.query.Exec(ctx, q, args...)
	return tag, databaseError(err)
}
