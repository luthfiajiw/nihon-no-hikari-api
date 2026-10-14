package admin

import (
	"time"

	"github.com/google/uuid"
)

// These models describe the dynamic admin payloads for Swagger.
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
	Questions []questionInput `json:"questions"`
}

type LessonContent struct {
	Content string `json:"content" example:"<p>Belajar huruf hiragana.</p>"`
	Tips    string `json:"tips" example:"Latih setiap hari."`
}

type PreviewRequest struct {
	Content *LessonContent `json:"content"`
}

type PreviewResponse struct {
	Success bool          `json:"success" example:"true"`
	Data    LessonContent `json:"data"`
}

type MeResponse struct {
	Success bool   `json:"success" example:"true"`
	Role    string `json:"role" example:"Admin" enums:"Admin"`
}

type StatusResponse struct {
	Success bool `json:"success" example:"true"`
}

type CreateResponse struct {
	Success bool      `json:"success" example:"true"`
	ID      uuid.UUID `json:"id" format:"uuid"`
}

type UpdateResponse struct {
	Success bool `json:"success" example:"true"`
	// True jika konten sudah diterbitkan. Perubahan question_set selalu disimpan sebagai draft.
	Draft bool `json:"draft"`
}

type PublishResponse struct {
	Success bool `json:"success" example:"true"`
	// Hanya untuk question_set; dapat berubah saat versi baru diterbitkan.
	ID *uuid.UUID `json:"id,omitempty" format:"uuid"`
}

type RemoveResponse struct {
	Success bool `json:"success" example:"true"`
	// True jika diarsipkan karena memiliki riwayat atau relasi; false jika dihapus permanen.
	Archived bool `json:"archived"`
}

type ContentListResponse struct {
	Success bool `json:"success" example:"true"`
	// Kolom resource sesuai kind, tanpa content lesson; course menyertakan module_count dan lesson_count.
	Data []map[string]interface{} `json:"data"`
}

type ContentDetailResponse struct {
	Success bool `json:"success" example:"true"`
	// Kolom resource sesuai kind; question_set menyertakan questions, options, dan is_correct.
	Data map[string]interface{} `json:"data"`
	// Payload perubahan yang belum diterbitkan, atau null jika tidak ada draft.
	Draft *ContentRequest `json:"draft"`
}

type CatalogData struct {
	Courses []map[string]interface{} `json:"courses"`
	Modules []map[string]interface{} `json:"modules"`
	// Lesson hanya memuat id, module_id, title, slug, order_index, is_published, dan archived_at.
	Lessons      []map[string]interface{} `json:"lessons"`
	QuestionSets []map[string]interface{} `json:"question_sets"`
}

type CatalogResponse struct {
	Success bool        `json:"success" example:"true"`
	Data    CatalogData `json:"data"`
}

type Level struct {
	ID          uuid.UUID `json:"id" format:"uuid"`
	Code        string    `json:"code" example:"N5"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	OrderIndex  int16     `json:"order_index"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type LevelsResponse struct {
	Success bool    `json:"success" example:"true"`
	Data    []Level `json:"data"`
}

type StrokeTemplate struct {
	ID            uuid.UUID `json:"id" format:"uuid"`
	Character     string    `json:"character" example:"あ"`
	Romaji        string    `json:"romaji" example:"a"`
	Script        string    `json:"script" enums:"hiragana,katakana"`
	SourceVersion string    `json:"source_version"`
}

type TemplatesResponse struct {
	Success bool             `json:"success" example:"true"`
	Data    []StrokeTemplate `json:"data"`
}
