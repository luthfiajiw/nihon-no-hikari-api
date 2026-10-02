package model

import (
	"github.com/google/uuid"
)

type CourseLevel struct {
	ID   uuid.UUID `json:"id"`
	Code string    `json:"code"`
	Name string    `json:"name"`
}

type Course struct {
	ID           uuid.UUID   `json:"id"`
	Slug         string      `json:"slug"`
	Title        string      `json:"title"`
	Description  *string     `json:"description"`
	ThumbnailUrl *string     `json:"thumbnail_url"`
	TotalMinutes int32       `json:"total_minutes"`
	TotalLessons int32       `json:"total_lessons"`
	Level        CourseLevel `json:"level"`
}

type CourseDetail struct {
	ID           uuid.UUID   `json:"id"`
	Slug         string      `json:"slug"`
	Title        string      `json:"title"`
	Description  *string     `json:"description"`
	ThumbnailUrl *string     `json:"thumbnail_url"`
	TotalMinutes int32       `json:"total_minutes"`
	TotalLessons int32       `json:"total_lessons"`
	Level        CourseLevel `json:"level"`
	Modules      []Module    `json:"modules"`
}

type Status string

const (
	StatusLocked     Status = "locked"
	StatusUnlocked   Status = "unlocked"
	StatusInProgress Status = "in_progress"
	StatusCompleted  Status = "completed"
)

func IsValidStatus(status Status) bool {
	switch status {
	case StatusLocked, StatusUnlocked, StatusInProgress, StatusCompleted:
		return true
	default:
		return false
	}
}

type Module struct {
	ID               uuid.UUID `json:"id"`
	Slug             string    `json:"slug"`
	Title            string    `json:"title"`
	Description      *string   `json:"description"`
	IsMandatory      bool      `json:"is_mandatory"`
	IsEntry          bool      `json:"is_entry"`
	Status           Status    `json:"status"`
	EstimatedMinutes int32     `json:"estimated_minutes"`
}

type ListCourseResponse struct {
	Success bool     `json:"success"`
	Message string   `json:"message"`
	Data    []Course `json:"data"`
}

type CourseDetailResponse struct {
	Success bool         `json:"success"`
	Message string       `json:"message"`
	Data    CourseDetail `json:"data"`
}
