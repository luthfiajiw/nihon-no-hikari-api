package kotoba

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	MaxCoverBytes   = 2 * 1024 * 1024
	MaxAudioBytes   = 1024 * 1024
	MediaRetention  = 24 * time.Hour
	CleanupInterval = 5 * time.Minute
)

var (
	ErrNotFound    = errors.New("topik tidak ditemukan")
	ErrConflict    = errors.New("topik telah berubah; muat ulang sebelum menyimpan")
	ErrUnavailable = errors.New("penyimpanan media belum tersedia")
	ErrStorage     = errors.New("layanan penyimpanan media gagal")
	ErrTooLarge    = errors.New("ukuran file melebihi batas")
)

type FieldErrors map[string]string

func (e FieldErrors) Error() string { return "periksa kembali kolom yang ditandai" }

type Furigana struct {
	Text    string `json:"text"`
	Reading string `json:"reading,omitempty"`
}
type Example struct {
	Japanese   string `json:"japanese"`
	Indonesian string `json:"indonesian"`
}
type WordFields struct {
	Kanji       string     `json:"kanji"`
	Hiragana    string     `json:"hiragana"`
	Furigana    []Furigana `json:"furigana"`
	Romaji      string     `json:"romaji"`
	Translation string     `json:"translation"`
	Example     Example    `json:"example"`
	Type        string     `json:"type" enums:"Kata Benda,Kata Kerja,Kata Sifat,Lainnya"`
}
type VocabularyInput struct {
	ID *uuid.UUID `json:"id,omitempty"`
	WordFields
	AudioMediaID *uuid.UUID `json:"audio_media_id"`
}
type TopicInput struct {
	Title        string            `json:"title" maxLength:"160"`
	Level        string            `json:"level" enums:"N5,N4,N3,N2,N1"`
	Category     string            `json:"category" enums:"Dasar,Populer,Pemula"`
	Description  string            `json:"description" maxLength:"200"`
	OrderIndex   int               `json:"order_index" minimum:"1"`
	IsActive     bool              `json:"is_active"`
	IsVisible    bool              `json:"is_visible"`
	CoverMediaID uuid.UUID         `json:"cover_media_id"`
	Version      int64             `json:"version,omitempty"`
	Vocabulary   []VocabularyInput `json:"vocabulary"`
}
type Vocabulary struct {
	ID uuid.UUID `json:"id"`
	WordFields
	OrderIndex   int        `json:"order_index"`
	AudioMediaID *uuid.UUID `json:"audio_media_id,omitempty"`
	AudioURL     *string    `json:"audio_url"`
	AudioBucket  string     `json:"-"`
	AudioPath    string     `json:"-"`
}
type Topic struct {
	ID                 uuid.UUID    `json:"id"`
	Title              string       `json:"title"`
	Level              string       `json:"level"`
	Category           string       `json:"category"`
	Description        string       `json:"description"`
	OrderIndex         int          `json:"order_index"`
	IsActive           *bool        `json:"is_active,omitempty"`
	IsVisible          *bool        `json:"is_visible,omitempty"`
	CoverMediaID       *uuid.UUID   `json:"cover_media_id,omitempty"`
	CoverURL           string       `json:"cover_url"`
	MediaURLsExpiresAt *time.Time   `json:"media_urls_expires_at,omitempty"`
	Version            *int64       `json:"version,omitempty"`
	VocabularyCount    int          `json:"vocabulary_count"`
	ExerciseCount      int          `json:"exercise_count"`
	CreatedAt          time.Time    `json:"created_at"`
	UpdatedAt          time.Time    `json:"updated_at"`
	Vocabulary         []Vocabulary `json:"vocabulary,omitempty"`
	CoverBucket        string       `json:"-"`
	CoverPath          string       `json:"-"`
}
type Media struct {
	ID           uuid.UUID  `json:"id"`
	Kind         string     `json:"kind"`
	URL          string     `json:"url"`
	URLExpiresAt *time.Time `json:"url_expires_at,omitempty"`
	MIMEType     string     `json:"mime_type"`
	SizeBytes    int64      `json:"size_bytes"`
	Bucket       string     `json:"-"`
	ObjectPath   string     `json:"-"`
	UploadedBy   uuid.UUID  `json:"-"`
	Status       string     `json:"-"`
	Attempts     int        `json:"-"`
}
type Filter struct{ Level, Search, Status, Sort string }
type TopicResponse struct {
	Success bool  `json:"success"`
	Data    Topic `json:"data"`
}
type TopicsResponse struct {
	Success bool    `json:"success"`
	Data    []Topic `json:"data"`
}
type MediaResponse struct {
	Success bool  `json:"success"`
	Data    Media `json:"data"`
}
type StatusResponse struct {
	Success bool `json:"success"`
}
type ErrorResponse struct {
	Success     bool        `json:"success"`
	Error       string      `json:"error"`
	FieldErrors FieldErrors `json:"field_errors,omitempty"`
}

func validLevel(v string) bool { return v == "N5" || v == "N4" || v == "N3" || v == "N2" || v == "N1" }
func (f *Filter) Validate(admin bool) error {
	e := FieldErrors{}
	if f.Level != "" && !validLevel(f.Level) {
		e["level"] = "Level tidak valid."
	}
	if f.Status != "" && (!admin || (f.Status != "all" && f.Status != "active" && f.Status != "inactive")) {
		e["status"] = "Status tidak valid."
	}
	if f.Sort == "" {
		if admin {
			f.Sort = "order"
		} else {
			f.Sort = "newest"
		}
	}
	if f.Sort != "order" && f.Sort != "title" && (admin || (f.Sort != "newest" && f.Sort != "vocabulary")) {
		e["sort"] = "Pengurutan tidak valid."
	}
	f.Search = strings.TrimSpace(f.Search)
	if len(e) > 0 {
		return e
	}
	return nil
}

func (in *TopicInput) Validate(update bool) error {
	e := FieldErrors{}
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)
	if in.Title == "" || utf8.RuneCountInString(in.Title) > 160 {
		e["title"] = "Nama topik wajib diisi, maksimal 160 karakter."
	}
	if in.Description == "" || utf8.RuneCountInString(in.Description) > 200 {
		e["description"] = "Deskripsi wajib diisi, maksimal 200 karakter."
	}
	if !validLevel(in.Level) {
		e["level"] = "Pilih level N5 sampai N1."
	}
	if in.Category != "Dasar" && in.Category != "Populer" && in.Category != "Pemula" {
		e["category"] = "Label topik tidak valid."
	}
	if in.OrderIndex < 1 || in.OrderIndex > math.MaxInt32 {
		e["order_index"] = "Urutan harus antara 1 dan 2147483647."
	}
	if in.CoverMediaID == uuid.Nil {
		e["cover_media_id"] = "Cover wajib diunggah."
	}
	if update && in.Version < 1 {
		e["version"] = "Versi topik wajib dikirim."
	}
	if len(in.Vocabulary) == 0 {
		e["vocabulary"] = "Tambahkan minimal satu kosakata."
	}
	seen := map[uuid.UUID]bool{}
	for i := range in.Vocabulary {
		w := &in.Vocabulary[i]
		prefix := fmt.Sprintf("vocabulary.%d.", i)
		if w.ID != nil {
			if *w.ID == uuid.Nil || seen[*w.ID] || !update {
				e[prefix+"id"] = "ID kosakata tidak valid atau duplikat; kosongkan ID untuk kosakata baru."
			}
			seen[*w.ID] = true
		}
		required := map[string]*string{"kanji": &w.Kanji, "hiragana": &w.Hiragana, "romaji": &w.Romaji, "translation": &w.Translation, "example.japanese": &w.Example.Japanese, "example.indonesian": &w.Example.Indonesian}
		for key, value := range required {
			*value = strings.TrimSpace(*value)
			if *value == "" {
				e[prefix+key] = "Kolom ini wajib diisi."
			}
		}
		if w.Type != "Kata Benda" && w.Type != "Kata Kerja" && w.Type != "Kata Sifat" && w.Type != "Lainnya" {
			e[prefix+"type"] = "Jenis kata tidak valid."
		}
		if w.AudioMediaID != nil && *w.AudioMediaID == uuid.Nil {
			e[prefix+"audio_media_id"] = "ID audio tidak valid."
		}
		if len(w.Furigana) == 0 {
			reading := w.Hiragana
			if w.Kanji == w.Hiragana {
				reading = ""
			}
			w.Furigana = []Furigana{{Text: w.Kanji, Reading: reading}}
		}
		var combined strings.Builder
		for _, part := range w.Furigana {
			combined.WriteString(part.Text)
			if strings.TrimSpace(part.Text) == "" {
				e[prefix+"furigana"] = "Teks segmen tidak boleh kosong."
			}
		}
		if combined.String() != w.Kanji {
			e[prefix+"furigana"] = "Gabungan teks segmen harus sama dengan kata Jepang."
		}
	}
	if len(e) > 0 {
		return e
	}
	return nil
}
