package model

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type UpsertLessonProgressRequest struct {
	Status *Status `json:"status"`
}

func (r UpsertLessonProgressRequest) Validate() error {
	if r.Status == nil {
		return errors.New("status wajib diisi")
	}
	if !IsValidStatus(*r.Status) {
		return errors.New("status harus locked, unlocked, in_progress, atau completed")
	}
	return nil
}

type LessonProgress struct {
	UserID      uuid.UUID  `json:"user_id"`
	LessonID    uuid.UUID  `json:"lesson_id"`
	Status      Status     `json:"status"`
	CompletedAt *time.Time `json:"completed_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type LessonProgressResponse struct {
	Success bool           `json:"success"`
	Message string         `json:"message"`
	Data    LessonProgress `json:"data"`
}
