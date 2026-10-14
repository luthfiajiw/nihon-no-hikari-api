package user

import (
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
