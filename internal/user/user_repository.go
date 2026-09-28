package user

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	FindBy(ctx context.Context, params FindUserParam) (User, error)
	CreateTx(ctx context.Context, tx pgx.Tx, params UserParams) (User, error)
}

type dbRepository struct {
	db *pgxpool.Pool
}

// CreateTx implements [Repository].
func (d *dbRepository) CreateTx(ctx context.Context, tx pgx.Tx, params UserParams) (User, error) {
	q := `
		INSERT INTO users (email, display_name, password_hash) VALUES ($1, $2, $3) RETURNING id
	`

	var userID uuid.UUID
	err := tx.QueryRow(
		ctx,
		q,
		params.Email,
		params.DisplayName,
		params.Password,
	).Scan(&userID)

	if err != nil {
		return User{}, err
	}

	return User{
		ID:          userID,
		Email:       params.Email,
		DisplayName: params.DisplayName,
		AvatarURL:   nil,
	}, nil
}

// GetById implements [Repository].
func (d *dbRepository) FindBy(ctx context.Context, params FindUserParam) (User, error) {
	var row pgx.Row

	q := `
		SELECT id, email, display_name, avatar_url, password_hash FROM users
	`

	if params.ID != uuid.Nil {
		q += "WHERE id = $1"
		row = d.db.QueryRow(ctx, q, params.ID)
	}
	if params.Email != "" {
		q += "WHERE email = $1"
		row = d.db.QueryRow(ctx, q, params.Email)
	}

	var user User
	err := row.Scan(&user.ID, &user.Email, &user.DisplayName, &user.AvatarURL, &user.Password)

	if err != nil {
		return User{}, err
	}

	return user, nil
}

func NewRepository(db *pgxpool.Pool) Repository {
	return &dbRepository{db: db}
}
