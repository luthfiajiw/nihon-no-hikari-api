package user

import (
	"time"

	"github.com/google/uuid"
)

type Role string

const (
	RoleUser  Role = "User"
	RoleAdmin Role = "Admin"
)

type User struct {
	ID          uuid.UUID `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	AvatarURL   *string   `json:"avatar_url"`
	Password    string    `json:"password_hash"`
	Role        Role      `json:"role"`
}

type UserParams struct {
	DisplayName     string `json:"display_name"`
	Email           string `json:"email"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirm_password"`
}

type OAuthUserParams struct {
	DisplayName string
	Email       string
	AvatarURL   *string
}

type FindUserParam struct {
	ID    uuid.UUID
	Email string
}

type ChangeRoleRequest struct {
	Role Role `json:"role" enums:"User,Admin" binding:"required" example:"Admin"`
}

type ListUsersResponse struct {
	Success bool      `json:"success" example:"true"`
	Data    []Summary `json:"data"`
}

type ChangeRoleResponse struct {
	Success bool    `json:"success" example:"true"`
	Data    Summary `json:"data"`
}

type ProfileSessionSummary struct {
	ID        uuid.UUID `json:"id"`
	ClientIP  string    `json:"client_ip"`
	UserAgent string    `json:"user_agent"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	IsCurrent bool      `json:"is_current"`
}

type ProfileData struct {
	ID           uuid.UUID               `json:"id"`
	Email        string                  `json:"email"`
	DisplayName  string                  `json:"display_name"`
	AvatarURL    *string                 `json:"avatar_url"`
	Role         Role                    `json:"role"`
	HasPassword  bool                    `json:"has_password"`
	GoogleLinked bool                    `json:"google_linked"`
	Sessions     []ProfileSessionSummary `json:"sessions"`
}

type ProfileResponse struct {
	Success bool        `json:"success" example:"true"`
	Data    ProfileData `json:"data"`
}

type UpdateProfileRequest struct {
	DisplayName string  `json:"display_name" validate:"required,min=1,max=120"`
	AvatarURL   *string `json:"avatar_url"`
}

type UpdateProfileResponse struct {
	Success bool `json:"success" example:"true"`
	Data    User `json:"data"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password" validate:"required,min=8"`
	ConfirmPassword string `json:"confirm_password" validate:"required"`
}

type LinkGoogleRequest struct {
	IDToken           string `json:"id_token" validate:"required"`
	VerificationToken string `json:"verification_token" validate:"required"`
}

type UnlinkGoogleRequest struct {
	CurrentPassword string `json:"current_password" validate:"required"`
}

type EmailVerification struct {
	ID             uuid.UUID  `json:"id"`
	UserID         uuid.UUID  `json:"user_id"`
	Purpose        string     `json:"purpose"`
	CodeHash       string     `json:"-"`
	Attempts       int        `json:"attempts"`
	ExpiresAt      time.Time  `json:"expires_at"`
	VerifiedAt     *time.Time `json:"verified_at,omitempty"`
	GrantHash      *string    `json:"-"`
	GrantExpiresAt *time.Time `json:"grant_expires_at,omitempty"`
	ConsumedAt     *time.Time `json:"consumed_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

type VerifyLinkGoogleCodeRequest struct {
	Code string `json:"code" validate:"required,len=6"`
}

type VerifyLinkGoogleCodeData struct {
	VerificationToken string `json:"verification_token"`
}

type VerifyLinkGoogleCodeResponse struct {
	Success bool                     `json:"success" example:"true"`
	Data    VerifyLinkGoogleCodeData `json:"data"`
}
