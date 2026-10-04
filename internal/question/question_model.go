package question

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

type Skill string

const (
	SkillReading Skill = "reading"
	SkillWriting Skill = "writing"
)

func (s Skill) IsSupported() bool {
	return s == SkillReading || s == SkillWriting
}

type QuestionOption struct {
	ID         uuid.UUID `json:"id"`
	Label      string    `json:"label"`
	MediaURL   *string   `json:"media_url"`
	OrderIndex int16     `json:"order_index"`
}

type Question struct {
	ID             uuid.UUID        `json:"id"`
	QuestionType   string           `json:"question_type"`
	Skill          Skill            `json:"skill"`
	PromptText     *string          `json:"prompt_text"`
	PromptMediaURL *string          `json:"prompt_media_url"`
	Points         int16            `json:"points"`
	OrderIndex     int16            `json:"order_index"`
	Options        []QuestionOption `json:"options"`
}

type QuestionSet struct {
	ID               uuid.UUID `json:"id"`
	LessonID         uuid.UUID `json:"lesson_id"`
	Title            string    `json:"title"`
	Skill            *Skill    `json:"skill"`
	PassingScore     int16     `json:"passing_score"`
	QuestionCount    int32     `json:"question_count"`
	TimeLimitSeconds *int32    `json:"time_limit_seconds"`
	MaxAttempts      *int16    `json:"max_attempts"`
	CooldownMinutes  int32     `json:"cooldown_minutes"`
	ShuffleQuestions bool      `json:"shuffle_questions"`
	IsPassed         bool      `json:"is_passed"`
	AttemptsUsed     int32     `json:"attempts_used"`
}

type ListQuestionSetsResponse struct {
	Success bool          `json:"success"`
	Message string        `json:"message"`
	Data    []QuestionSet `json:"data"`
}

type QuestionSetDetail struct {
	QuestionSet
	Questions []Question `json:"questions"`
}

type QuestionSetDetailResponse struct {
	Success bool              `json:"success"`
	Message string            `json:"message"`
	Data    QuestionSetDetail `json:"data"`
}

type Attempt struct {
	ID          uuid.UUID   `json:"id"`
	Number      int16       `json:"attempt_number"`
	Status      string      `json:"status"`
	StartedAt   time.Time   `json:"started_at"`
	ExpiresAt   *time.Time  `json:"expires_at"`
	QuestionSet QuestionSet `json:"question_set"`
	Questions   []Question  `json:"questions"`
}

type StartAttemptResponse struct {
	Success bool    `json:"success"`
	Message string  `json:"message"`
	Data    Attempt `json:"data"`
}

type SubmitAnswerRequest struct {
	QuestionID       uuid.UUID  `json:"question_id"`
	SelectedOptionID *uuid.UUID `json:"selected_option_id"`
	AnswerText       *string    `json:"answer_text"`
}

type SubmitAttemptRequest struct {
	Answers []SubmitAnswerRequest `json:"answers"`
}

func (r SubmitAttemptRequest) Validate() error {
	if len(r.Answers) == 0 {
		return errors.New("answers wajib diisi")
	}

	seen := make(map[uuid.UUID]struct{}, len(r.Answers))
	for _, answer := range r.Answers {
		if answer.QuestionID == uuid.Nil {
			return errors.New("question_id tidak valid")
		}
		if _, exists := seen[answer.QuestionID]; exists {
			return errors.New("question_id tidak boleh duplikat")
		}
		seen[answer.QuestionID] = struct{}{}

		hasOption := answer.SelectedOptionID != nil && *answer.SelectedOptionID != uuid.Nil
		hasText := answer.AnswerText != nil && strings.TrimSpace(*answer.AnswerText) != ""
		if hasOption == hasText {
			return errors.New("setiap jawaban harus memiliki tepat satu selected_option_id atau answer_text")
		}
		if answer.AnswerText != nil && utf8.RuneCountInString(*answer.AnswerText) > 255 {
			return errors.New("answer_text maksimal 255 karakter")
		}
	}

	return nil
}

type AnswerResult struct {
	QuestionID   uuid.UUID `json:"question_id"`
	Skill        Skill     `json:"skill"`
	IsCorrect    bool      `json:"is_correct"`
	EarnedPoints int16     `json:"earned_points"`
	MaxPoints    int16     `json:"max_points"`
	Explanation  *string   `json:"explanation"`
}

type SkillResult struct {
	Skill        Skill `json:"skill"`
	Score        int16 `json:"score"`
	EarnedPoints int16 `json:"earned_points"`
	TotalPoints  int16 `json:"total_points"`
	IsPassed     bool  `json:"is_passed"`
}

type AttemptResult struct {
	AttemptID    uuid.UUID      `json:"attempt_id"`
	Score        int16          `json:"score"`
	EarnedPoints int16          `json:"earned_points"`
	TotalPoints  int16          `json:"total_points"`
	PassingScore int16          `json:"passing_score"`
	IsPassed     bool           `json:"is_passed"`
	LessonStatus string         `json:"lesson_status"`
	SubmittedAt  time.Time      `json:"submitted_at"`
	Skills       []SkillResult  `json:"skills"`
	Answers      []AnswerResult `json:"answers"`
}

type SubmitAttemptResponse struct {
	Success bool          `json:"success"`
	Message string        `json:"message"`
	Data    AttemptResult `json:"data"`
}
