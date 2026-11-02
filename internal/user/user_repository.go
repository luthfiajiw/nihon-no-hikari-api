package user

import (
	"context"

	"nihon-no-hikari-api/pkg/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	UseTx(tx pgx.Tx) Repository
	FindBy(ctx context.Context, params FindUserParam) (User, error)
	Create(ctx context.Context, params UserParams) (User, error)
}

type dbRepository struct {
	q db.Querier
}

func (r *dbRepository) UseTx(tx pgx.Tx) Repository {
	return &dbRepository{q: tx}
}

// Create implements [Repository].
func (r *dbRepository) Create(ctx context.Context, params UserParams) (User, error) {
	q := `
		INSERT INTO users (email, display_name, password_hash) VALUES ($1, $2, $3) RETURNING id, role
	`

	var userID uuid.UUID
	var role Role
	err := r.q.QueryRow(
		ctx,
		q,
		params.Email,
		params.DisplayName,
		params.Password,
	).Scan(&userID, &role)

	if err != nil {
		return User{}, err
	}

	return User{
		ID:          userID,
		Email:       params.Email,
		DisplayName: params.DisplayName,
		AvatarURL:   nil,
		Role:        role,
	}, nil
}

// FindBy implements [Repository].
func (r *dbRepository) FindBy(ctx context.Context, params FindUserParam) (User, error) {
	var row pgx.Row

	q := `
		SELECT id, email, display_name, avatar_url, password_hash, role FROM users
	`

	if params.ID != uuid.Nil {
		q += "WHERE id = $1"
		row = r.q.QueryRow(ctx, q, params.ID)
	}
	if params.Email != "" {
		q += "WHERE email = $1"
		row = r.q.QueryRow(ctx, q, params.Email)
	}

	var user User
	err := row.Scan(&user.ID, &user.Email, &user.DisplayName, &user.AvatarURL, &user.Password, &user.Role)

	if err != nil {
		return User{}, err
	}

	return user, nil
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &dbRepository{q: pool}
}
