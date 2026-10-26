package model

import (
	"encoding/json"

	"nihon-no-hikari-api/pkg/kanastroke"

	"github.com/google/uuid"
)

// ContentRequest describes the union accepted by the polymorphic content endpoint.
// ContentRequest combines the fields accepted by each content kind. Only send
// fields relevant to the selected kind; PATCH requires the complete payload.
type ContentRequest struct {
	Title string `json:"title" binding:"required" maxLength:"160" example:"Hiragana dasar"`
	// Wajib untuk course, module, dan lesson; huruf kecil, angka, dan tanda hubung.
	Slug string `json:"slug" maxLength:"120" example:"hiragana-dasar"`
	// Wajib untuk course.
	LevelID *uuid.UUID `json:"level_id" format:"uuid"`
	// Wajib untuk module.
	CourseID *uuid.UUID `json:"course_id" format:"uuid"`
	// Wajib untuk lesson atau question_set final_exam; kosong untuk practice.
	ModuleID *uuid.UUID `json:"module_id" format:"uuid"`
	// Wajib untuk question_set practice; kosong untuk final_exam.
	LessonID *uuid.UUID `json:"lesson_id" format:"uuid"`
	// Untuk course dan module.
	Description *string `json:"description"`
	// Untuk course; URL HTTP/HTTPS.
	ThumbnailURL *string `json:"thumbnail_url" example:"https://example.com/thumbnail.png"`
	OrderIndex   int16   `json:"order_index" minimum:"0" example:"0"`
	// Untuk module.
	IsMandatory bool `json:"is_mandatory"`
	// Untuk module dan lesson.
	IsEntry bool `json:"is_entry"`
	// Daftar UUID module prasyarat. [] menghapus semua prasyarat; null/tidak dikirim mempertahankan relasi.
	ModulePrerequisites []uuid.UUID `json:"module_prerequisites"`
	// Daftar UUID lesson prasyarat. [] menghapus semua prasyarat; null/tidak dikirim mempertahankan relasi.
	LessonPrerequisites []uuid.UUID `json:"lesson_prerequisites"`
	EstimatedMinutes    *int32      `json:"estimated_minutes" minimum:"0"`
	// Untuk lesson; HTML disanitasi. Jika kosong, content dan tips menjadi string kosong.
	Content *LessonContent `json:"content"`
	// Field berikut hanya untuk question_set. Kind dan skill wajib diisi.
	Kind         string `json:"kind" enums:"practice,final_exam"`
	Skill        string `json:"skill" enums:"reading,writing"`
	PassingScore int16  `json:"passing_score" minimum:"0" maximum:"100" example:"70"`
	// Null berarti semua soal; saat publish tidak boleh melebihi jumlah soal tersedia.
	TotalQuestions *int16 `json:"total_questions" minimum:"1"`
	// Null atau 0 berarti tanpa batas waktu.
	TimeLimitSeconds *int32 `json:"time_limit_seconds" minimum:"0"`
	// Null berarti tanpa batas attempt.
	MaxAttempts      *int16 `json:"max_attempts" minimum:"1"`
	CooldownMinutes  int32  `json:"cooldown_minutes" minimum:"0"`
	ShuffleQuestions bool   `json:"shuffle_questions"`
	// Boleh kosong saat menyimpan draft. Saat publish wajib ada soal valid.
	Questions []QuestionInput `json:"questions"`
} // @name internal_admin.ContentRequest

type LessonContent struct {
	Content string `json:"content" example:"<p>Belajar huruf hiragana.</p>"`
	Tips    string `json:"tips" example:"Latih setiap hari."`
} // @name internal_admin.LessonContent

type PreviewRequest struct {
	Content json.RawMessage `json:"content" swaggertype:"object"`
} // @name internal_admin.PreviewRequest

type PreviewResponse struct {
	Success bool            `json:"success" example:"true"`
	Data    json.RawMessage `json:"data" swaggertype:"object"`
} // @name internal_admin.PreviewResponse

type MeResponse struct {
	Success bool   `json:"success" example:"true"`
	Role    string `json:"role" example:"Admin" enums:"Admin"`
} // @name internal_admin.MeResponse

type StatusResponse struct {
	Success bool `json:"success" example:"true"`
} // @name internal_admin.StatusResponse

type CreateResponse struct {
	Success bool      `json:"success" example:"true"`
	ID      uuid.UUID `json:"id" format:"uuid"`
} // @name internal_admin.CreateResponse

type UpdateResponse struct {
	Success bool `json:"success" example:"true"`
	// True jika konten sudah diterbitkan. Perubahan question_set selalu disimpan sebagai draft.
	Draft bool `json:"draft"`
} // @name internal_admin.UpdateResponse

type PublishResponse struct {
	Success bool `json:"success" example:"true"`
	// Hanya untuk question_set; dapat berubah saat versi baru diterbitkan.
	ID *uuid.UUID `json:"id,omitempty" format:"uuid"`
} // @name internal_admin.PublishResponse

type RemoveResponse struct {
	Success bool `json:"success" example:"true"`
	// True jika diarsipkan karena memiliki riwayat atau relasi; false jika dihapus permanen.
	Archived bool `json:"archived"`
} // @name internal_admin.RemoveResponse

type ContentListResponse struct {
	Success bool `json:"success" example:"true"`
	// Kolom resource sesuai kind, tanpa content lesson; course menyertakan module_count dan lesson_count.
	Data []json.RawMessage `json:"data" swaggertype:"array,object"`
} // @name internal_admin.ContentListResponse

type ContentDetailResponse struct {
	Success bool `json:"success" example:"true"`
	// Kolom resource sesuai kind; question_set menyertakan questions, options, dan is_correct.
	Data json.RawMessage `json:"data" swaggertype:"object"`
	// Payload perubahan yang belum diterbitkan, atau null jika tidak ada draft.
	Draft json.RawMessage `json:"draft" swaggertype:"object"`
} // @name internal_admin.ContentDetailResponse

type CatalogData struct {
	Courses []json.RawMessage `json:"courses" swaggertype:"array,object"`
	Modules []json.RawMessage `json:"modules" swaggertype:"array,object"`
	// Lesson hanya memuat id, module_id, title, slug, order_index, is_published, dan archived_at.
	Lessons      []json.RawMessage `json:"lessons" swaggertype:"array,object"`
	QuestionSets []json.RawMessage `json:"question_sets" swaggertype:"array,object"`
} // @name internal_admin.CatalogData

type CatalogResponse struct {
	Success bool        `json:"success" example:"true"`
	Data    CatalogData `json:"data"`
} // @name internal_admin.CatalogResponse

type Level struct {
	ID          uuid.UUID `json:"id" format:"uuid"`
	Code        string    `json:"code" example:"N5"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	OrderIndex  int16     `json:"order_index"`
	CreatedAt   string    `json:"created_at" format:"date-time"`
	UpdatedAt   string    `json:"updated_at" format:"date-time"`
} // @name internal_admin.Level

type LevelsResponse struct {
	Success bool    `json:"success" example:"true"`
	Data    []Level `json:"data"`
} // @name internal_admin.LevelsResponse

type StrokeTemplate struct {
	ID            uuid.UUID `json:"id" format:"uuid"`
	Character     string    `json:"character" example:"あ"`
	Romaji        string    `json:"romaji" example:"a"`
	Script        string    `json:"script" enums:"hiragana,katakana"`
	SourceVersion string    `json:"source_version"`
	StrokeCount   int       `json:"stroke_count"`
} // @name internal_admin.StrokeTemplate

type TemplatesResponse struct {
	Success bool             `json:"success" example:"true"`
	Data    []StrokeTemplate `json:"data"`
} // @name internal_admin.TemplatesResponse

type StrokeTemplateDetail struct {
	StrokeTemplate
	StrokeData kanastroke.Data `json:"stroke_data"`
} // @name internal_admin.StrokeTemplateDetail

type StrokeTemplateResponse struct {
	Success bool                 `json:"success"`
	Data    StrokeTemplateDetail `json:"data"`
} // @name internal_admin.StrokeTemplateResponse

type StrokeTemplatePreview struct {
	Script        string          `json:"script"`
	Character     string          `json:"character"`
	Romaji        string          `json:"romaji"`
	SourceVersion string          `json:"source_version"`
	StrokeCount   int             `json:"stroke_count"`
	StrokeData    kanastroke.Data `json:"stroke_data"`
} // @name internal_admin.StrokeTemplatePreview

type StrokeTemplatePreviewResponse struct {
	Success bool                  `json:"success"`
	Data    StrokeTemplatePreview `json:"data"`
} // @name internal_admin.StrokeTemplatePreviewResponse
