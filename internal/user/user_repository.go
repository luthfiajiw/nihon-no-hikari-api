package user

import (
	"context"
	"errors"
	"time"

	"nihon-no-hikari-api/pkg/db"
	"nihon-no-hikari-api/pkg/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	UseTx(tx pgx.Tx) Repository
	FindBy(ctx context.Context, params FindUserParam) (User, error)
	Create(ctx context.Context, params UserParams) (User, error)
	CreateOAuth(ctx context.Context, params OAuthUserParams) (User, error)
	GetProfile(ctx context.Context, userID uuid.UUID, currentSessionID uuid.UUID) (ProfileData, error)
	UpdateProfile(ctx context.Context, userID uuid.UUID, displayName string, avatarURL *string) (User, error)
	UpdatePassword(ctx context.Context, userID uuid.UUID, passwordHash string) error
	HasIdentity(ctx context.Context, userID uuid.UUID, provider string) (bool, error)
	FindIdentityUserID(ctx context.Context, provider string, sub string) (uuid.UUID, error)
	CreateIdentity(ctx context.Context, userID uuid.UUID, provider string, sub string, email string) error
	DeleteIdentity(ctx context.Context, userID uuid.UUID, provider string) error
	RevokeOtherSessions(ctx context.Context, userID uuid.UUID, currentSessionID uuid.UUID) error
	CreateEmailVerification(ctx context.Context, userID uuid.UUID, purpose string, codeHash string, expiresAt time.Time) (uuid.UUID, error)
	LatestEmailVerification(ctx context.Context, userID uuid.UUID, purpose string) (*EmailVerification, error)
	CountRecentEmailVerifications(ctx context.Context, userID uuid.UUID, purpose string, since time.Time) (int, error)
	IncrementVerificationAttempts(ctx context.Context, id uuid.UUID) (int, error)
	SetVerificationGrant(ctx context.Context, id uuid.UUID, grantHash string, grantExpiresAt time.Time) error
	FindValidVerificationGrant(ctx context.Context, userID uuid.UUID, purpose string, grantHash string) (*EmailVerification, error)
	ConsumeVerificationGrant(ctx context.Context, id uuid.UUID) error
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
	var passwordHash *string
	err := row.Scan(&user.ID, &user.Email, &user.DisplayName, &user.AvatarURL, &passwordHash, &user.Role)

	if err != nil {
		return User{}, err
	}
	// OAuth-only users have a NULL password_hash; an empty hash never matches a password.
	if passwordHash != nil {
		user.Password = *passwordHash
	}

	return user, nil
}

// CreateOAuth implements [Repository].
func (r *dbRepository) CreateOAuth(ctx context.Context, params OAuthUserParams) (User, error) {
	q := `
		INSERT INTO users (email, display_name, avatar_url) VALUES ($1, $2, $3) RETURNING id, role
	`

	var userID uuid.UUID
	var role Role
	err := r.q.QueryRow(ctx, q, params.Email, params.DisplayName, params.AvatarURL).Scan(&userID, &role)
	if err != nil {
		return User{}, err
	}

	return User{
		ID:          userID,
		Email:       params.Email,
		DisplayName: params.DisplayName,
		AvatarURL:   params.AvatarURL,
		Role:        role,
	}, nil
}

// GetProfile implements [Repository].
func (r *dbRepository) GetProfile(ctx context.Context, userID uuid.UUID, currentSessionID uuid.UUID) (ProfileData, error) {
	var profile ProfileData
	var passwordHash *string

	userQuery := `
		SELECT id, email, display_name, avatar_url, role, password_hash
		FROM users
		WHERE id = $1
	`
	err := r.q.QueryRow(ctx, userQuery, userID).Scan(
		&profile.ID,
		&profile.Email,
		&profile.DisplayName,
		&profile.AvatarURL,
		&profile.Role,
		&passwordHash,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ProfileData{}, utils.ErrUserNotFound
		}
		return ProfileData{}, err
	}
	profile.HasPassword = passwordHash != nil && *passwordHash != ""

	// Check if Google is linked in user_identities
	googleQuery := `
		SELECT EXISTS(
			SELECT 1 FROM user_identities WHERE user_id = $1 AND provider = 'google'
		)
	`
	err = r.q.QueryRow(ctx, googleQuery, userID).Scan(&profile.GoogleLinked)
	if err != nil {
		return ProfileData{}, err
	}

	// Fetch active sessions
	sessionsQuery := `
		SELECT id, client_ip, user_agent, created_at, expires_at
		FROM user_sessions
		WHERE user_id = $1 AND is_blocked = false AND expires_at > NOW()
		ORDER BY created_at DESC
	`
	rows, err := r.q.Query(ctx, sessionsQuery, userID)
	if err != nil {
		return ProfileData{}, err
	}
	defer rows.Close()

	profile.Sessions = make([]ProfileSessionSummary, 0)
	for rows.Next() {
		var s ProfileSessionSummary
		if err := rows.Scan(&s.ID, &s.ClientIP, &s.UserAgent, &s.CreatedAt, &s.ExpiresAt); err != nil {
			return ProfileData{}, err
		}
		s.IsCurrent = (s.ID == currentSessionID)
		profile.Sessions = append(profile.Sessions, s)
	}
	if rows.Err() != nil {
		return ProfileData{}, rows.Err()
	}

	return profile, nil
}

// UpdateProfile implements [Repository].
func (r *dbRepository) UpdateProfile(ctx context.Context, userID uuid.UUID, displayName string, avatarURL *string) (User, error) {
	query := `
		UPDATE users
		SET display_name = $1, avatar_url = $2, updated_at = NOW()
		WHERE id = $3
		RETURNING id, email, display_name, avatar_url, role
	`
	var user User
	err := r.q.QueryRow(ctx, query, displayName, avatarURL, userID).Scan(
		&user.ID,
		&user.Email,
		&user.DisplayName,
		&user.AvatarURL,
		&user.Role,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, utils.ErrUserNotFound
		}
		return User{}, err
	}
	return user, nil
}

// UpdatePassword implements [Repository].
func (r *dbRepository) UpdatePassword(ctx context.Context, userID uuid.UUID, passwordHash string) error {
	query := `
		UPDATE users
		SET password_hash = $1, updated_at = NOW()
		WHERE id = $2
	`
	cmdTag, err := r.q.Exec(ctx, query, passwordHash, userID)
	if err != nil {
		return err
	}
	if cmdTag.RowsAffected() == 0 {
		return utils.ErrUserNotFound
	}
	return nil
}

// HasIdentity implements [Repository].
func (r *dbRepository) HasIdentity(ctx context.Context, userID uuid.UUID, provider string) (bool, error) {
	var exists bool
	err := r.q.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM user_identities WHERE user_id = $1 AND provider = $2)
	`, userID, provider).Scan(&exists)
	return exists, err
}

// FindIdentityUserID implements [Repository].
func (r *dbRepository) FindIdentityUserID(ctx context.Context, provider string, sub string) (uuid.UUID, error) {
	var userID uuid.UUID
	err := r.q.QueryRow(ctx, `
		SELECT user_id FROM user_identities WHERE provider = $1 AND provider_sub = $2
	`, provider, sub).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, utils.ErrUserNotFound
		}
		return uuid.Nil, err
	}
	return userID, nil
}

// CreateIdentity implements [Repository].
func (r *dbRepository) CreateIdentity(ctx context.Context, userID uuid.UUID, provider string, sub string, email string) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO user_identities (user_id, provider, provider_sub, email) VALUES ($1,$2,$3,$4)
	`, userID, provider, sub, email)
	return err
}

// DeleteIdentity implements [Repository].
func (r *dbRepository) DeleteIdentity(ctx context.Context, userID uuid.UUID, provider string) error {
	cmdTag, err := r.q.Exec(ctx, `
		DELETE FROM user_identities WHERE user_id = $1 AND provider = $2
	`, userID, provider)
	if err != nil {
		return err
	}
	if cmdTag.RowsAffected() == 0 {
		return utils.ErrGoogleNotLinked
	}
	return nil
}

// RevokeOtherSessions implements [Repository].
func (r *dbRepository) RevokeOtherSessions(ctx context.Context, userID uuid.UUID, currentSessionID uuid.UUID) error {
	query := `
		UPDATE user_sessions
		SET is_blocked = true
		WHERE user_id = $1 AND id != $2 AND is_blocked = false
	`
	_, err := r.q.Exec(ctx, query, userID, currentSessionID)
	return err
}

func (r *dbRepository) CreateEmailVerification(ctx context.Context, userID uuid.UUID, purpose string, codeHash string, expiresAt time.Time) (uuid.UUID, error) {
	query := `
		INSERT INTO email_verifications (user_id, purpose, code_hash, expires_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`
	var id uuid.UUID
	err := r.q.QueryRow(ctx, query, userID, purpose, codeHash, expiresAt).Scan(&id)
	return id, err
}

func (r *dbRepository) LatestEmailVerification(ctx context.Context, userID uuid.UUID, purpose string) (*EmailVerification, error) {
	query := `
		SELECT id, user_id, purpose, code_hash, attempts, expires_at, verified_at, grant_hash, grant_expires_at, consumed_at, created_at
		FROM email_verifications
		WHERE user_id = $1 AND purpose = $2
		ORDER BY created_at DESC
		LIMIT 1
	`
	var ev EmailVerification
	err := r.q.QueryRow(ctx, query, userID, purpose).Scan(
		&ev.ID,
		&ev.UserID,
		&ev.Purpose,
		&ev.CodeHash,
		&ev.Attempts,
		&ev.ExpiresAt,
		&ev.VerifiedAt,
		&ev.GrantHash,
		&ev.GrantExpiresAt,
		&ev.ConsumedAt,
		&ev.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &ev, nil
}

func (r *dbRepository) CountRecentEmailVerifications(ctx context.Context, userID uuid.UUID, purpose string, since time.Time) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM email_verifications
		WHERE user_id = $1 AND purpose = $2 AND created_at >= $3
	`
	var count int
	err := r.q.QueryRow(ctx, query, userID, purpose, since).Scan(&count)
	return count, err
}

func (r *dbRepository) IncrementVerificationAttempts(ctx context.Context, id uuid.UUID) (int, error) {
	query := `
		UPDATE email_verifications
		SET attempts = attempts + 1
		WHERE id = $1
		RETURNING attempts
	`
	var attempts int
	err := r.q.QueryRow(ctx, query, id).Scan(&attempts)
	return attempts, err
}

func (r *dbRepository) SetVerificationGrant(ctx context.Context, id uuid.UUID, grantHash string, grantExpiresAt time.Time) error {
	query := `
		UPDATE email_verifications
		SET verified_at = NOW(), grant_hash = $1, grant_expires_at = $2
		WHERE id = $3
	`
	_, err := r.q.Exec(ctx, query, grantHash, grantExpiresAt, id)
	return err
}

func (r *dbRepository) FindValidVerificationGrant(ctx context.Context, userID uuid.UUID, purpose string, grantHash string) (*EmailVerification, error) {
	query := `
		SELECT id, user_id, purpose, code_hash, attempts, expires_at, verified_at, grant_hash, grant_expires_at, consumed_at, created_at
		FROM email_verifications
		WHERE user_id = $1 AND purpose = $2 AND grant_hash = $3 AND consumed_at IS NULL AND grant_expires_at > NOW()
		ORDER BY created_at DESC
		LIMIT 1
	`
	var ev EmailVerification
	err := r.q.QueryRow(ctx, query, userID, purpose, grantHash).Scan(
		&ev.ID,
		&ev.UserID,
		&ev.Purpose,
		&ev.CodeHash,
		&ev.Attempts,
		&ev.ExpiresAt,
		&ev.VerifiedAt,
		&ev.GrantHash,
		&ev.GrantExpiresAt,
		&ev.ConsumedAt,
		&ev.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &ev, nil
}

func (r *dbRepository) ConsumeVerificationGrant(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE email_verifications
		SET consumed_at = NOW()
		WHERE id = $1 AND consumed_at IS NULL
	`
	cmd, err := r.q.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return utils.ErrVerificationTokenInvalid
	}
	return nil
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &dbRepository{q: pool}
}
