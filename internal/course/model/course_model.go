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
	TotalHours   int32       `json:"total_hours"`
	TotalLessons int32       `json:"total_lessons"`
	Level        CourseLevel `json:"level"`
}

type ListCourseResponse struct {
	Success bool     `json:"success"`
	Message string   `json:"message"`
	Data    []Course `json:"data"`
}
