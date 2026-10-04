package model

import (
	"nihon-no-hikari-api/internal/question"

	"github.com/google/uuid"
)

type Lesson struct {
	ID     uuid.UUID `json:"id"`
	Slug   string    `json:"slug"`
	Title  string    `json:"title"`
	Status Status    `json:"status"`
}

type LessonDetail struct {
	ID           uuid.UUID              `json:"id"`
	Slug         string                 `json:"slug"`
	Title        string                 `json:"title"`
	Content      string                 `json:"content"`
	QuestionSets []question.QuestionSet `json:"question_sets"`
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
