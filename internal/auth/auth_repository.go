package auth

import (
	"context"
	"errors"

	"nihon-no-hikari-api/pkg/db"
	"nihon-no-hikari-api/pkg/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	UseTx(tx pgx.Tx) Repository
	CreateSession(ctx context.Context, session UserSession) error
	GetSession(ctx context.Context, refreshToken string) (UserSession, error)
	UpdateRefreshToken(ctx context.Context, sessionId uuid.UUID, refreshToken string) error
	BlockSessionById(ctx context.Context, sessionId string) error
}

type dbRepository struct {
	q db.Querier
}

func (r *dbRepository) UseTx(tx pgx.Tx) Repository {
	return &dbRepository{q: tx}
}

// BlockSessionById implements [Repository].
func (r *dbRepository) BlockSessionById(ctx context.Context, sessionId string) error {
	sql := `
		UPDATE user_sessions
		SET is_blocked = $1
		WHERE id = $2
	`

	cmdTag, err := r.q.Exec(ctx, sql, true, sessionId)
	if err != nil {
		return err
	}
	if cmdTag.RowsAffected() == 0 {
		return utils.ErrSessionNotFound
	}

	return nil
}

// CreateSession implements [Repository].
func (r *dbRepository) CreateSession(ctx context.Context, session UserSession) error {
	q := `
		INSERT INTO user_sessions (
			id, user_id, refresh_token, user_agent, client_ip, is_blocked, expires_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7)
	`

	_, err := r.q.Exec(
		ctx,
		q,
		session.ID,
		session.UserID,
		session.RefreshToken,
		session.UserAgent,
		session.ClientIP,
		session.isBlocked,
		session.ExpiresAt,
	)
	if err != nil {
		return err
	}

	return nil
}

// GetSession implements [Repository].
func (r *dbRepository) GetSession(ctx context.Context, refreshToken string) (UserSession, error) {
	sql := `
		SELECT id, user_id, refresh_token, user_agent, client_ip, expires_at
		FROM user_sessions
		WHERE refresh_token = $1 AND is_blocked = false;
	`
	row := r.q.QueryRow(ctx, sql, refreshToken)

	var session UserSession
	err := row.Scan(
		&session.ID,
		&session.UserID,
		&session.RefreshToken,
		&session.UserAgent,
		&session.ClientIP,
		&session.ExpiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return UserSession{}, utils.ErrSessionNotFound
		}
		return UserSession{}, err
	}

	return session, nil
}

// UpdateRefreshToken implements [Repository].
func (r *dbRepository) UpdateRefreshToken(ctx context.Context, sessionId uuid.UUID, refreshToken string) error {
	sql := `
		UPDATE user_sessions
		SET refresh_token = $1
		WHERE id = $2
	`

	_, err := r.q.Exec(ctx, sql, refreshToken, sessionId)
	if err != nil {
		return err
	}

	return nil
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &dbRepository{q: pool}
}
