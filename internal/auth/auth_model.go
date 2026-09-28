package auth

import (
	"time"

	"github.com/google/uuid"
)

type RegisterRequest struct {
	DisplayName     string `json:"display_name"`
	Email           string `json:"email"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirm_password"`
}

type SignInRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type UserSession struct {
	ID           uuid.UUID
	UserID       uuid.UUID
	RefreshToken string
	UserAgent    string
	ClientIP     string
	isBlocked    bool
	ExpiresAt    time.Time
	CreatedAt    time.Time
}

type AuthSession struct {
	ID                    uuid.UUID `json:"id"`
	AccessToken           string    `json:"access_token"`
	AccessTokenExpiresAt  time.Time `json:"access_token_expires_at"`
	RefreshToken          string    `json:"refresh_token"`
	RefreshTokenExpiresAt time.Time `json:"refresh_token_expires_at"`
}

type SignInData struct {
	ID          uuid.UUID   `json:"id"`
	DisplayName string      `json:"display_name"`
	Email       string      `json:"email"`
	AvatarURL   *string     `json:"avatar_url"`
	Session     AuthSession `json:"session"`
}

type SignInResponse struct {
	Success bool       `json:"success"`
	Message string     `json:"message"`
	Data    SignInData `json:"data"`
}

type TokenRequest struct {
	GrantType string `json:"grant_type" validate:"required"`
	Token     string `json:"token" validate:"required"`
}

type RefreshTokenResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    AuthSession `json:"data"`
}

type LogoutRequest struct {
	ID string `json:"session_id" validate:"required"`
}
