package kotoba

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"

	"nihon-no-hikari-api/pkg/middleware"
	"nihon-no-hikari-api/pkg/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct {
	service    Operations
	middleware middleware.Middleware
}

func NewHandler(service Operations, m middleware.Middleware) *Handler {
	return &Handler{service: service, middleware: m}
}
func (h *Handler) RegisterRoutes(app *fiber.App) {
	admin := app.Group("/api/v1/admin/kotoba", h.middleware.AuthMiddleware(), h.middleware.RequireRole("Admin"))
	admin.Post("/media", h.upload)
	admin.Get("/topics", h.adminList)
	admin.Post("/topics", h.create)
	admin.Get("/topics/:id", h.adminDetail)
	admin.Put("/topics/:id", h.update)
	admin.Delete("/topics/:id", h.remove)
	student := app.Group("/api/v1/kotoba", h.middleware.AuthMiddleware())
	student.Get("/topics", h.studentList)
	student.Get("/topics/:id", h.studentDetail)
}
func respondError(c fiber.Ctx, err error) error {
	status, message := 500, "gagal memproses Kotoba"
	var fields FieldErrors
	switch {
	case errors.As(err, &fields):
		status, message = 400, fields.Error()
	case errors.Is(err, ErrNotFound):
		status, message = 404, ErrNotFound.Error()
	case errors.Is(err, ErrConflict):
		status, message = 409, ErrConflict.Error()
	case errors.Is(err, ErrTooLarge):
		status, message = 413, ErrTooLarge.Error()
	case errors.Is(err, ErrUnavailable):
		status, message = 503, ErrUnavailable.Error()
	case errors.Is(err, ErrStorage):
		status, message = 502, ErrStorage.Error()
	default:
		slog.Error("kotoba request failed", "error", err)
	}
	return c.Status(status).JSON(ErrorResponse{Success: false, Error: message, FieldErrors: fields})
}
func parseID(c fiber.Ctx) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil || id == uuid.Nil {
		return uuid.Nil, FieldErrors{"id": "UUID topik tidak valid."}
	}
	return id, nil
}
func parseInput(c fiber.Ctx) (TopicInput, error) {
	var in *TopicInput
	if strings.Split(c.Get("Content-Type"), ";")[0] != "application/json" {
		return TopicInput{}, FieldErrors{"body": "Gunakan application/json."}
	}
	decoder := json.NewDecoder(bytes.NewReader(c.Body()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&in); err != nil || in == nil {
		return TopicInput{}, FieldErrors{"body": "Payload JSON tidak valid atau memiliki kolom yang tidak dikenal."}
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return TopicInput{}, FieldErrors{"body": "Kirim satu objek JSON."}
	}
	return *in, nil
}
func (h *Handler) list(c fiber.Ctx, admin bool) error {
	c.Set("Cache-Control", "no-store")
	f := Filter{Level: c.Query("level"), Search: c.Query("search"), Status: c.Query("status"), Sort: c.Query("sort")}
	if err := f.Validate(admin); err != nil {
		return respondError(c, err)
	}
	topics, err := h.service.List(c.Context(), f, admin)
	if err != nil {
		return respondError(c, err)
	}
	return c.JSON(TopicsResponse{Success: true, Data: topics})
}
func (h *Handler) detail(c fiber.Ctx, admin bool) error {
	c.Set("Cache-Control", "no-store")
	id, err := parseID(c)
	if err != nil {
		return respondError(c, err)
	}
	topic, err := h.service.Detail(c.Context(), id, admin)
	if err != nil {
		return respondError(c, err)
	}
	return c.JSON(TopicResponse{Success: true, Data: topic})
}

// adminList godoc
// @Summary Daftar topik Kotoba admin
// @Description Seluruh status; tanpa pagination. Memerlukan role Admin.
// @Tags Kotoba Admin
// @Produce json
// @Security BearerAuth
// @Param level query string false "Level JLPT" Enums(N5,N4,N3,N2,N1)
// @Param search query string false "Cari judul dan deskripsi"
// @Param status query string false "Status aktif" Enums(all,active,inactive)
// @Param sort query string false "Urutan" Enums(order,title) default(order)
// @Success 200 {object} TopicsResponse
// @Failure 400,401,403,500 {object} ErrorResponse
// @Router /admin/kotoba/topics [get]
func (h *Handler) adminList(c fiber.Ctx) error { return h.list(c, true) }

// studentList godoc
// @Summary Daftar topik Kotoba siswa
// @Description Hanya topik aktif dan terlihat; tanpa pagination. Respons siswa tidak menyertakan versi, status, atau ID media.
// @Tags Kotoba
// @Produce json
// @Security BearerAuth
// @Param level query string false "Level JLPT" Enums(N5,N4,N3,N2,N1)
// @Param search query string false "Cari judul dan deskripsi"
// @Param sort query string false "Urutan" Enums(newest,title,vocabulary,order) default(newest)
// @Success 200 {object} TopicsResponse
// @Failure 400,401,500 {object} ErrorResponse
// @Router /kotoba/topics [get]
func (h *Handler) studentList(c fiber.Ctx) error { return h.list(c, false) }

// adminDetail godoc
// @Summary Detail topik untuk editor Kotoba
// @Tags Kotoba Admin
// @Produce json
// @Security BearerAuth
// @Param id path string true "UUID topik" Format(uuid)
// @Success 200 {object} TopicResponse
// @Failure 400,401,403,404,500 {object} ErrorResponse
// @Router /admin/kotoba/topics/{id} [get]
func (h *Handler) adminDetail(c fiber.Ctx) error { return h.detail(c, true) }

// studentDetail godoc
// @Summary Detail topik dan kosakata siswa
// @Description Topik nonaktif menghasilkan 404. Topik aktif yang disembunyikan tetap dapat diakses lewat ID. ID media dan versi hanya tersedia di endpoint admin.
// @Tags Kotoba
// @Produce json
// @Security BearerAuth
// @Param id path string true "UUID topik" Format(uuid)
// @Success 200 {object} TopicResponse
// @Failure 400,401,404,500 {object} ErrorResponse
// @Router /kotoba/topics/{id} [get]
func (h *Handler) studentDetail(c fiber.Ctx) error { return h.detail(c, false) }

// create godoc
// @Summary Buat topik beserta kosakata
// @Description Upload media terlebih dahulu. Kosakata baru tidak mengirim id. Audio opsional.
// @Tags Kotoba Admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body TopicInput true "Form topik lengkap"
// @Success 201 {object} TopicResponse
// @Failure 400,401,403,409,500 {object} ErrorResponse
// @Router /admin/kotoba/topics [post]
func (h *Handler) create(c fiber.Ctx) error { return h.save(c, false) }

// update godoc
// @Summary Ganti seluruh isi topik dan urutan kosakata
// @Description version wajib sesuai respons terakhir. Kosakata dengan id dipertahankan; tanpa id dibuat baru; yang tidak dikirim dihapus. audio_media_id null menghapus audio.
// @Tags Kotoba Admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "UUID topik" Format(uuid)
// @Param body body TopicInput true "Form lengkap beserta version"
// @Success 200 {object} TopicResponse
// @Failure 400,401,403,404,409,500 {object} ErrorResponse
// @Router /admin/kotoba/topics/{id} [put]
func (h *Handler) update(c fiber.Ctx) error { return h.save(c, true) }
func (h *Handler) save(c fiber.Ctx, update bool) error {
	c.Set("Cache-Control", "no-store")
	id := uuid.Nil
	if update {
		var err error
		id, err = parseID(c)
		if err != nil {
			return respondError(c, err)
		}
	}
	in, err := parseInput(c)
	if err != nil {
		return respondError(c, err)
	}
	if err := in.Validate(update); err != nil {
		return respondError(c, err)
	}
	topic, err := h.service.Save(c.Context(), id, in)
	if err != nil {
		return respondError(c, err)
	}
	status := 201
	if update {
		status = 200
	}
	return c.Status(status).JSON(TopicResponse{Success: true, Data: topic})
}

// remove godoc
// @Summary Hapus topik dan kosakatanya
// @Description Media yang tidak lagi digunakan dibersihkan setelah 24 jam.
// @Tags Kotoba Admin
// @Produce json
// @Security BearerAuth
// @Param id path string true "UUID topik" Format(uuid)
// @Success 200 {object} StatusResponse
// @Failure 400,401,403,404,500 {object} ErrorResponse
// @Router /admin/kotoba/topics/{id} [delete]
func (h *Handler) remove(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return respondError(c, err)
	}
	if err := h.service.Delete(c.Context(), id); err != nil {
		return respondError(c, err)
	}
	return c.JSON(StatusResponse{Success: true})
}

// upload godoc
// @Summary Upload cover atau audio ke Supabase private
// @Description Satu file per permintaan. PNG/JPG maksimal 2 MiB; MP3 maksimal 1 MiB. Preview memakai signed URL selama 15 menit. File yang tidak dipasang ke topik dibersihkan setelah 24 jam.
// @Tags Kotoba Admin
// @Accept multipart/form-data
// @Produce json
// @Security BearerAuth
// @Param kind formData string true "Jenis media" Enums(cover,audio)
// @Param file formData file true "Cover PNG/JPG atau audio MP3"
// @Success 201 {object} MediaResponse
// @Failure 400,401,403,413,500,502,503 {object} ErrorResponse
// @Router /admin/kotoba/media [post]
func (h *Handler) upload(c fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	form, err := c.MultipartForm()
	if err != nil {
		return respondError(c, FieldErrors{"file": "Kirim satu file melalui multipart/form-data."})
	}
	if len(form.File) != 1 || len(form.File["file"]) != 1 || len(form.Value["kind"]) != 1 {
		return respondError(c, FieldErrors{"file": "Kirim satu file dan satu jenis media."})
	}
	kind := form.Value["kind"][0]
	if kind != "cover" && kind != "audio" {
		return respondError(c, FieldErrors{"kind": "Pilih cover atau audio."})
	}
	file := form.File["file"][0]
	limit := int64(MaxCoverBytes)
	if kind == "audio" {
		limit = MaxAudioBytes
	}
	if file.Size > limit {
		return respondError(c, ErrTooLarge)
	}
	source, err := file.Open()
	if err != nil {
		return respondError(c, FieldErrors{"file": "File tidak dapat dibaca."})
	}
	defer source.Close()
	user, ok := c.Locals(utils.UserIDKey).(uuid.UUID)
	if !ok || user == uuid.Nil {
		return c.Status(401).JSON(ErrorResponse{Error: "sesi tidak ditemukan"})
	}
	media, err := h.service.Upload(c.Context(), user, kind, file.Filename, file.Header.Get("Content-Type"), source)
	if err != nil {
		return respondError(c, err)
	}
	return c.Status(201).JSON(MediaResponse{Success: true, Data: media})
}
