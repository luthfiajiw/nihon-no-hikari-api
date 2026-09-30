package model

import "github.com/google/uuid"

type Module struct {
	ID               uuid.UUID `json:"id"`
	Slug             string    `json:"slug"`
	Title            string    `json:"title"`
	Description      *string   `json:"description"`
	IsMandatory      bool      `json:"is_mandatory"`
	IsEntry          bool      `json:"is_entry"`
	EstimatedMinutes int32     `json:"estimated_minutes"`
}

type Lesson struct {
	ID    uuid.UUID `json:"id"`
	Slug  string    `json:"slug"`
	Title string    `json:"title"`
}

type LessonDetail struct {
	ID      uuid.UUID `json:"id"`
	Slug    string    `json:"slug"`
	Title   string    `json:"title"`
	Content string    `json:"content"`
}

type ModuleLesson struct {
	Module  Module   `json:"module"`
	Lessons []Lesson `json:"lessons"`
}

type ListLessonResponse struct {
	Success bool           `json:"success"`
	Message string         `json:"message"`
	Data    []ModuleLesson `json:"data"`
}

type LessonDetailResponse struct {
	Success bool         `json:"success"`
	Message string       `json:"message"`
	Data    LessonDetail `json:"data"`
}
