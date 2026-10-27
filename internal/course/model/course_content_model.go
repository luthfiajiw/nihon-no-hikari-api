package model

import (
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strings"

	"nihon-no-hikari-api/pkg/contenthtml"

	"github.com/google/uuid"
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

type ReorderRequest struct {
	ParentID   uuid.UUID   `json:"parent_id" swaggertype:"string" format:"uuid"`
	OrderedIDs []uuid.UUID `json:"ordered_ids" swaggertype:"array,string"`
} // @name internal_course.ReorderRequest

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

// ContentRequest documents course, module, and lesson payloads. Only send fields
// relevant to the resource; PATCH requires the complete payload.
type ContentRequest struct {
	Title string `json:"title" binding:"required" maxLength:"160" example:"Hiragana dasar"`
	// Wajib untuk course, module, dan lesson; huruf kecil, angka, dan tanda hubung.
	Slug string `json:"slug" maxLength:"120" example:"hiragana-dasar"`
	// Wajib untuk course.
	LevelID *uuid.UUID `json:"level_id" format:"uuid"`
	// Wajib untuk module.
	CourseID *uuid.UUID `json:"course_id" format:"uuid"`
	// Wajib untuk lesson.
	ModuleID *uuid.UUID `json:"module_id" format:"uuid"`
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
} // @name internal_course.ContentRequest

type LessonContent struct {
	Content string `json:"content" example:"<p>Belajar huruf hiragana.</p>"`
	Tips    string `json:"tips" example:"Latih setiap hari."`
} // @name internal_course.LessonContent

type PreviewRequest struct {
	Content json.RawMessage `json:"content" swaggertype:"object"`
} // @name internal_course.PreviewRequest

type PreviewResponse struct {
	Success bool            `json:"success" example:"true"`
	Data    json.RawMessage `json:"data" swaggertype:"object"`
} // @name internal_course.PreviewResponse

type StatusResponse struct {
	Success bool `json:"success" example:"true"`
} // @name internal_course.StatusResponse

type CreateResponse struct {
	Success bool      `json:"success" example:"true"`
	ID      uuid.UUID `json:"id" format:"uuid"`
} // @name internal_course.CreateResponse

type UpdateResponse struct {
	Success bool `json:"success" example:"true"`
	// True jika perubahan konten terbit disimpan sebagai draft.
	Draft bool `json:"draft"`
} // @name internal_course.UpdateResponse

type PublishResponse struct {
	Success bool `json:"success" example:"true"`
} // @name internal_course.PublishResponse

type RemoveResponse struct {
	Success bool `json:"success" example:"true"`
	// True jika diarsipkan karena memiliki riwayat atau relasi; false jika dihapus permanen.
	Archived bool `json:"archived"`
} // @name internal_course.RemoveResponse

type ContentListResponse struct {
	Success bool `json:"success" example:"true"`
	// Kolom resource sesuai kind, tanpa content lesson; course menyertakan module_count dan lesson_count.
	Data []json.RawMessage `json:"data" swaggertype:"array,object"`
} // @name internal_course.ContentListResponse

type ContentDetailResponse struct {
	Success bool `json:"success" example:"true"`
	// Kolom course, module, atau lesson sesuai resource; content lesson disanitasi.
	Data json.RawMessage `json:"data" swaggertype:"object"`
	// Payload perubahan yang belum diterbitkan, atau null jika tidak ada draft.
	Draft json.RawMessage `json:"draft" swaggertype:"object"`
} // @name internal_course.ContentDetailResponse

type CatalogData struct {
	Courses []json.RawMessage `json:"courses" swaggertype:"array,object"`
	Modules []json.RawMessage `json:"modules" swaggertype:"array,object"`
	// Lesson hanya memuat id, module_id, title, slug, order_index, is_published, dan archived_at.
	Lessons      []json.RawMessage `json:"lessons" swaggertype:"array,object"`
	QuestionSets []json.RawMessage `json:"question_sets" swaggertype:"array,object"`
} // @name internal_course.CatalogData

type CatalogResponse struct {
	Success bool        `json:"success" example:"true"`
	Data    CatalogData `json:"data"`
} // @name internal_course.CatalogResponse

type Level struct {
	ID          uuid.UUID `json:"id" format:"uuid"`
	Code        string    `json:"code" example:"N5"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	OrderIndex  int16     `json:"order_index"`
	CreatedAt   string    `json:"created_at" format:"date-time"`
	UpdatedAt   string    `json:"updated_at" format:"date-time"`
} // @name internal_course.Level

type LevelsResponse struct {
	Success bool    `json:"success" example:"true"`
	Data    []Level `json:"data"`
} // @name internal_course.LevelsResponse

type ContentState struct {
	Published bool
	Archived  bool
}
