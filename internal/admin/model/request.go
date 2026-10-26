package model

import (
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"nihon-no-hikari-api/pkg/contenthtml"
	"nihon-no-hikari-api/pkg/kanastroke"

	"github.com/google/uuid"
	"golang.org/x/text/unicode/norm"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type CoreInput struct {
	LevelID             uuid.UUID       `json:"level_id"`
	CourseID            uuid.UUID       `json:"course_id"`
	ModuleID            uuid.UUID       `json:"module_id"`
	Slug                string          `json:"slug"`
	Title               string          `json:"title"`
	Description         *string         `json:"description"`
	ThumbnailURL        *string         `json:"thumbnail_url"`
	OrderIndex          int16           `json:"order_index"`
	IsMandatory         bool            `json:"is_mandatory"`
	IsEntry             bool            `json:"is_entry"`
	EstimatedMinutes    *int32          `json:"estimated_minutes"`
	Content             json.RawMessage `json:"content"`
	ModulePrerequisites []uuid.UUID     `json:"module_prerequisites"`
	LessonPrerequisites []uuid.UUID     `json:"lesson_prerequisites"`
}

func (v *CoreInput) Validate(kind string) error {
	if !slugPattern.MatchString(v.Slug) || len(v.Slug) > 120 || strings.TrimSpace(v.Title) == "" || len(v.Title) > 160 || v.OrderIndex < 0 {
		return errors.New("slug, judul, atau urutan tidak valid")
	}
	switch kind {
	case "course":
		if v.LevelID == uuid.Nil {
			return errors.New("level wajib dipilih")
		}
	case "module":
		if v.CourseID == uuid.Nil {
			return errors.New("kursus wajib dipilih")
		}
	case "lesson":
		if v.ModuleID == uuid.Nil {
			return errors.New("module wajib dipilih")
		}
		clean, err := contenthtml.JSON(v.Content)
		if err != nil {
			return err
		}
		v.Content = clean
	}
	if v.ThumbnailURL != nil && *v.ThumbnailURL != "" {
		u, err := url.Parse(*v.ThumbnailURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return errors.New("URL gambar tidak valid")
		}
	}
	if v.EstimatedMinutes != nil && *v.EstimatedMinutes < 0 {
		return errors.New("estimasi waktu tidak valid")
	}
	for _, requiredID := range v.Prerequisites(kind) {
		if requiredID == uuid.Nil {
			return errors.New("ID prasyarat tidak valid")
		}
	}
	return nil
}

func (v CoreInput) Prerequisites(kind string) []uuid.UUID {
	switch kind {
	case "module":
		return v.ModulePrerequisites
	case "lesson":
		return v.LessonPrerequisites
	default:
		return nil
	}
}

type OptionInput struct {
	Label     string  `json:"label" example:"a"`
	MediaURL  *string `json:"media_url" example:"https://example.com/option.png"`
	IsCorrect bool    `json:"is_correct"`
} // @name internal_admin.optionInput

type QuestionInput struct {
	QuestionType     string        `json:"question_type" enums:"multiple_choice,stroke_writing"`
	PromptText       string        `json:"prompt_text" example:"Pilih romaji yang tepat."`
	StimulusText     *string       `json:"stimulus_text"`
	StimulusMediaURL *string       `json:"stimulus_media_url"`
	Explanation      *string       `json:"explanation"`
	Points           int16         `json:"points" example:"1"`
	StrokeTemplateID *uuid.UUID    `json:"stroke_template_id" format:"uuid"`
	ShowWritingGuide bool          `json:"show_writing_guide"`
	Options          []OptionInput `json:"options"`
} // @name internal_admin.questionInput

type SetInput struct {
	Kind             string          `json:"kind"`
	ModuleID         *uuid.UUID      `json:"module_id"`
	LessonID         *uuid.UUID      `json:"lesson_id"`
	Title            string          `json:"title"`
	Skill            string          `json:"skill"`
	PassingScore     int16           `json:"passing_score"`
	TotalQuestions   *int16          `json:"total_questions"`
	TimeLimitSeconds *int32          `json:"time_limit_seconds"`
	MaxAttempts      *int16          `json:"max_attempts"`
	CooldownMinutes  int32           `json:"cooldown_minutes"`
	ShuffleQuestions bool            `json:"shuffle_questions"`
	OrderIndex       int16           `json:"order_index"`
	Questions        []QuestionInput `json:"questions"`
}

func (v SetInput) Validate(publish bool) error {
	if v.Kind != "practice" && v.Kind != "final_exam" {
		return errors.New("jenis set tidak valid")
	}
	if (v.Kind == "practice" && (v.LessonID == nil || v.ModuleID != nil)) || (v.Kind == "final_exam" && (v.ModuleID == nil || v.LessonID != nil)) {
		return errors.New("tempat question set tidak valid")
	}
	if v.Skill != "reading" && v.Skill != "writing" {
		return errors.New("skill harus reading atau writing")
	}
	if strings.TrimSpace(v.Title) == "" || len(v.Title) > 160 || v.PassingScore < 0 || v.PassingScore > 100 || v.OrderIndex < 0 || v.CooldownMinutes < 0 {
		return errors.New("konfigurasi question set tidak valid")
	}
	if v.TimeLimitSeconds != nil && *v.TimeLimitSeconds < 0 {
		return errors.New("batas waktu tidak valid")
	}
	if v.MaxAttempts != nil && *v.MaxAttempts <= 0 {
		return errors.New("batas attempt tidak valid")
	}
	if v.TotalQuestions != nil && (*v.TotalQuestions <= 0 || (publish && int(*v.TotalQuestions) > len(v.Questions))) {
		return errors.New("jumlah soal dipilih melebihi soal tersedia")
	}
	if publish && len(v.Questions) == 0 {
		return errors.New("tambahkan setidaknya satu soal sebelum terbit")
	}
	if !publish {
		return nil
	}
	for _, q := range v.Questions {
		if strings.TrimSpace(q.PromptText) == "" || q.Points <= 0 || !validMediaURL(q.StimulusMediaURL) {
			return errors.New("prompt, poin, atau media soal tidak valid")
		}
		if (q.StimulusText == nil || strings.TrimSpace(*q.StimulusText) == "") && (q.StimulusMediaURL == nil || *q.StimulusMediaURL == "") && q.StrokeTemplateID == nil {
			return errors.New("soal memerlukan stimulus atau template")
		}
		if v.Skill == "reading" {
			if q.QuestionType != "multiple_choice" || q.StrokeTemplateID != nil || len(q.Options) < 2 {
				return errors.New("soal reading memerlukan minimal dua pilihan")
			}
			correct := 0
			for _, o := range q.Options {
				if strings.TrimSpace(o.Label) == "" || !validMediaURL(o.MediaURL) {
					return errors.New("pilihan jawaban tidak valid")
				}
				if o.IsCorrect {
					correct++
				}
			}
			if correct != 1 {
				return errors.New("pilih tepat satu jawaban benar")
			}
		} else if q.QuestionType != "stroke_writing" || q.StrokeTemplateID == nil || len(q.Options) != 0 {
			return errors.New("soal writing memerlukan template goresan")
		}
	}
	return nil
}

func validMediaURL(raw *string) bool {
	if raw == nil || *raw == "" {
		return true
	}
	u, err := url.Parse(*raw)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != ""
}

type ReorderRequest struct {
	ParentID   uuid.UUID   `json:"parent_id" swaggertype:"string" format:"uuid"`
	OrderedIDs []uuid.UUID `json:"ordered_ids" swaggertype:"array,string"`
} // @name internal_admin.ReorderRequest

func (input ReorderRequest) Validate(kind string) error {
	if kind != "module" && kind != "lesson" {
		return errors.New("jenis konten tidak valid")
	}
	if input.ParentID == uuid.Nil || input.OrderedIDs == nil || len(input.OrderedIDs) > 32768 {
		return errors.New("parent atau jumlah konten tidak valid")
	}
	seen := make(map[uuid.UUID]bool, len(input.OrderedIDs))
	for _, id := range input.OrderedIDs {
		if id == uuid.Nil || seen[id] {
			return errors.New("ID konten tidak valid atau berulang")
		}
		seen[id] = true
	}
	return nil
}

type StrokeTemplateInput struct {
	Script        string  `json:"script" enums:"hiragana,katakana"`
	Character     string  `json:"character" example:"あ"`
	Romaji        string  `json:"romaji" example:"a"`
	SourceVersion string  `json:"source_version" example:"r20260714"`
	SVGContent    *string `json:"svg_content,omitempty"`
} // @name internal_admin.StrokeTemplateInput

func (input *StrokeTemplateInput) Validate() error {
	input.Script = strings.TrimSpace(input.Script)
	input.Character = norm.NFC.String(strings.TrimSpace(input.Character))
	input.Romaji = strings.TrimSpace(input.Romaji)
	input.SourceVersion = strings.TrimSpace(input.SourceVersion)
	for _, value := range []string{input.Script, input.Character, input.Romaji, input.SourceVersion} {
		if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
			return errors.New("metadata template berisi karakter tidak valid")
		}
	}
	if !kanastroke.ValidCharacter(input.Script, input.Character) {
		return errors.New("karakter harus berupa satu hiragana atau katakana sesuai script")
	}
	if input.Romaji == "" || !utf8.ValidString(input.Romaji) || utf8.RuneCountInString(input.Romaji) > 8 {
		return errors.New("romaji wajib diisi dan maksimal 8 karakter")
	}
	if input.SourceVersion == "" {
		return errors.New("versi sumber wajib diisi")
	}
	if input.SVGContent != nil && len(*input.SVGContent) > kanastroke.MaxSVGBytes {
		return kanastroke.ErrTooLarge
	}
	return nil
}
