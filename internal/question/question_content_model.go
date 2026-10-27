package question

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"unicode/utf8"

	"nihon-no-hikari-api/pkg/kanastroke"

	"github.com/google/uuid"
	"golang.org/x/text/unicode/norm"
)

type OptionInput struct {
	Label     string  `json:"label" example:"a"`
	MediaURL  *string `json:"media_url" example:"https://example.com/option.png"`
	IsCorrect bool    `json:"is_correct"`
} // @name internal_question.OptionInput

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
} // @name internal_question.QuestionInput

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

type StrokeTemplateInput struct {
	Script        string  `json:"script" enums:"hiragana,katakana"`
	Character     string  `json:"character" example:"あ"`
	Romaji        string  `json:"romaji" example:"a"`
	SourceVersion string  `json:"source_version" example:"r20260714"`
	SVGContent    *string `json:"svg_content,omitempty"`
} // @name internal_question.StrokeTemplateInput

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

type StatusResponse struct {
	Success bool `json:"success" example:"true"`
} // @name internal_question.StatusResponse

type CreateResponse struct {
	Success bool      `json:"success" example:"true"`
	ID      uuid.UUID `json:"id" format:"uuid"`
} // @name internal_question.CreateResponse

type UpdateResponse struct {
	Success bool `json:"success" example:"true"`
	// True jika konten sudah diterbitkan. Perubahan question_set selalu disimpan sebagai draft.
	Draft bool `json:"draft"`
} // @name internal_question.UpdateResponse

type PublishResponse struct {
	Success bool `json:"success" example:"true"`
	// Hanya untuk question_set; dapat berubah saat versi baru diterbitkan.
	ID *uuid.UUID `json:"id,omitempty" format:"uuid"`
} // @name internal_question.PublishResponse

type RemoveResponse struct {
	Success bool `json:"success" example:"true"`
	// True jika diarsipkan karena memiliki riwayat atau relasi; false jika dihapus permanen.
	Archived bool `json:"archived"`
} // @name internal_question.RemoveResponse

type ContentListResponse struct {
	Success bool `json:"success" example:"true"`
	// Kolom resource sesuai kind, tanpa content lesson; course menyertakan module_count dan lesson_count.
	Data []json.RawMessage `json:"data" swaggertype:"array,object"`
} // @name internal_question.ContentListResponse

type ContentDetailResponse struct {
	Success bool `json:"success" example:"true"`
	// Kolom resource sesuai kind; question_set menyertakan questions, options, dan is_correct.
	Data json.RawMessage `json:"data" swaggertype:"object"`
	// Payload perubahan yang belum diterbitkan, atau null jika tidak ada draft.
	Draft json.RawMessage `json:"draft" swaggertype:"object"`
} // @name internal_question.ContentDetailResponse

type StrokeTemplate struct {
	ID            uuid.UUID `json:"id" format:"uuid"`
	Character     string    `json:"character" example:"あ"`
	Romaji        string    `json:"romaji" example:"a"`
	Script        string    `json:"script" enums:"hiragana,katakana"`
	SourceVersion string    `json:"source_version"`
	StrokeCount   int       `json:"stroke_count"`
} // @name internal_question.StrokeTemplate

type TemplatesResponse struct {
	Success bool             `json:"success" example:"true"`
	Data    []StrokeTemplate `json:"data"`
} // @name internal_question.TemplatesResponse

type StrokeTemplateDetail struct {
	StrokeTemplate
	StrokeData kanastroke.Data `json:"stroke_data"`
} // @name internal_question.StrokeTemplateDetail

type StrokeTemplateResponse struct {
	Success bool                 `json:"success"`
	Data    StrokeTemplateDetail `json:"data"`
} // @name internal_question.StrokeTemplateResponse

type StrokeTemplatePreview struct {
	Script        string          `json:"script"`
	Character     string          `json:"character"`
	Romaji        string          `json:"romaji"`
	SourceVersion string          `json:"source_version"`
	StrokeCount   int             `json:"stroke_count"`
	StrokeData    kanastroke.Data `json:"stroke_data"`
} // @name internal_question.StrokeTemplatePreview

type StrokeTemplatePreviewResponse struct {
	Success bool                  `json:"success"`
	Data    StrokeTemplatePreview `json:"data"`
} // @name internal_question.StrokeTemplatePreviewResponse

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
