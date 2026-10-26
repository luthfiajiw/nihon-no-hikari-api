package model

import "github.com/google/uuid"

type ContentState struct {
	Published bool
	Archived  bool
}
type QuestionSetState struct {
	ContentState
	LineageID   uuid.UUID
	Version     int
	HasAttempts bool
	Kind        string
	Skill       *string
	LessonID    *uuid.UUID
	ModuleID    *uuid.UUID
}
