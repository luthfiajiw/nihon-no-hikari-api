package admin

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"nihon-no-hikari-api/internal/admin/model"
	"nihon-no-hikari-api/pkg/middleware"
	"nihon-no-hikari-api/pkg/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct {
	service    Service
	middleware middleware.Middleware
}

func NewHandler(service Service, m middleware.Middleware) *Handler {
	return &Handler{service: service, middleware: m}
}

func (h *Handler) RegisterRoutes(app *fiber.App) {
	g := app.Group("/api/v1/admin", h.middleware.AuthMiddleware(), h.middleware.RequireRole("Admin"))
	g.Get("/me", h.me)
	g.Get("/catalog", h.catalog)
	g.Get("/levels", h.levels)
	g.Get("/templates", h.templates)
	g.Get("/templates/:id", h.templateDetail)
	g.Post("/templates/preview", h.previewTemplate)
	g.Post("/templates", h.createTemplate)
	g.Patch("/templates/:id", h.updateTemplate)
	g.Post("/lesson-content/preview", h.preview)
	g.Get("/content/:kind", h.list)
	g.Post("/content/:kind", h.create)
	g.Post("/content/:kind/reorder", h.reorder)
	g.Get("/content/:kind/:id", h.detail)
	g.Patch("/content/:kind/:id", h.update)
	g.Delete("/content/:kind/:id", h.remove)
	g.Post("/content/:kind/:id/publish", h.publish)
	g.Post("/content/:kind/:id/restore", h.restore)
}

func bad(c fiber.Ctx, message string) error {
	return c.Status(400).JSON(utils.ErrorRes{Success: false, Error: message})
}

func serverError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrInvalidInput), errors.Is(err, errInvalidPrerequisites):
		return bad(c, decodeError(err).Error())
	case errors.Is(err, ErrPayloadTooLarge):
		return c.Status(413).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
	case errors.Is(err, ErrStaleOrder):
		return c.Status(409).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
	case errors.Is(err, ErrNotFound):
		message := "konten tidak ditemukan"
		var app *applicationError
		if errors.As(err, &app) {
			message = app.message
		}
		return c.Status(404).JSON(utils.ErrorRes{Success: false, Error: message})
	case errors.Is(err, ErrConflict):
		message := "slug atau urutan sudah dipakai"
		var app *applicationError
		if errors.As(err, &app) {
			message = app.message
		}
		return c.Status(409).JSON(utils.ErrorRes{Success: false, Error: message})
	case errors.Is(err, ErrInvalidRelation):
		return bad(c, "relasi atau konfigurasi konten tidak valid")
	default:
		return c.Status(500).JSON(utils.ErrorRes{Success: false, Error: "gagal menyimpan konten"})
	}
}

func templateServerError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrInvalidInput), errors.Is(err, ErrPayloadTooLarge):
		return serverError(c, err)
	case errors.Is(err, ErrNotFound):
		return c.Status(404).JSON(utils.ErrorRes{Success: false, Error: "template kana tidak ditemukan"})
	case errors.Is(err, ErrConflict):
		return c.Status(409).JSON(utils.ErrorRes{Success: false, Error: "template dengan script, karakter, dan versi sumber tersebut sudah ada"})
	default:
		return c.Status(500).JSON(utils.ErrorRes{Success: false, Error: "gagal menyimpan atau mengambil template kana"})
	}
}

func resourceID(c fiber.Ctx) (uuid.UUID, error) { return uuid.Parse(c.Params("id")) }

// me godoc
// @Summary Periksa akses admin
// @Description Memastikan sesi pengguna memiliki role Admin.
// @Tags Admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} model.MeResponse
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Router /admin/me [get]
func (h *Handler) me(c fiber.Ctx) error {
	return c.JSON(MeResponse{Success: true, Role: "Admin"})
}

// catalog godoc
// @Summary Katalog konten admin
// @Description Memerlukan role Admin. Mengambil konten termasuk draft dan arsip. Jika course_id diisi, hanya modules dan lessons kursus tersebut yang dikembalikan; courses dan question_sets berupa array kosong.
// @Tags Admin
// @Produce json
// @Security BearerAuth
// @Param course_id query string false "Filter UUID kursus" Format(uuid)
// @Success 200 {object} model.CatalogResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/catalog [get]
func (h *Handler) catalog(c fiber.Ctx) error {
	var courseID *uuid.UUID
	if raw := c.Query("course_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return bad(c, "ID kursus tidak valid")
		}
		courseID = &id
	}
	response, err := h.service.Catalog(c.Context(), courseID)
	if err != nil {
		return serverError(c, err)
	}
	return c.JSON(response)
}

// levels godoc
// @Summary Daftar level untuk admin
// @Description Memerlukan role Admin. Mengambil level berdasarkan order_index.
// @Tags Admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} model.LevelsResponse
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/levels [get]
func (h *Handler) levels(c fiber.Ctx) error {
	response, err := h.service.Levels(c.Context())
	if err != nil {
		return serverError(c, err)
	}
	return c.JSON(response)
}

// templates godoc
// @Summary Daftar template goresan untuk admin
// @Description Memerlukan role Admin. Mengambil metadata template kana untuk stroke_template_id; tidak menyertakan stroke_data.
// @Tags Admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} model.TemplatesResponse
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/templates [get]
func (h *Handler) templates(c fiber.Ctx) error {
	response, err := h.service.Templates(c.Context())
	if err != nil {
		return serverError(c, err)
	}
	return c.JSON(response)
}

// preview godoc
// @Summary Pratinjau konten lesson
// @Description Memerlukan role Admin. Membersihkan HTML content dan merapikan tips tanpa menyimpan konten. Field tambahan pada objek content dipertahankan. Content kosong/null menjadi objek dengan content dan tips kosong; string HTML lama juga diterima.
// @Tags Admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body model.PreviewRequest true "Konten lesson untuk pratinjau"
// @Success 200 {object} model.PreviewResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Router /admin/lesson-content/preview [post]
func (h *Handler) preview(c fiber.Ctx) error {
	var body model.PreviewRequest
	if err := c.Bind().Body(&body); err != nil {
		return bad(c, "konten tidak valid")
	}
	response, err := h.service.PreviewContent(c.Context(), body.Content)
	if err != nil {
		return serverError(c, err)
	}
	return c.JSON(response)
}

// list godoc
// @Summary Daftar konten admin
// @Description Memerlukan role Admin. Mengambil seluruh resource sesuai kind, termasuk yang belum diterbitkan dan diarsipkan, berdasarkan order_index, created_at, dan id. Content lesson tidak disertakan; course menyertakan module_count dan lesson_count.
// @Tags Admin Content
// @Produce json
// @Security BearerAuth
// @Param kind path string true "Jenis resource" Enums(course,module,lesson,question_set)
// @Success 200 {object} model.ContentListResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/content/{kind} [get]
func (h *Handler) list(c fiber.Ctx) error {
	kind := c.Params("kind")
	if !validKind(kind) {
		return bad(c, "jenis konten tidak valid")
	}
	response, err := h.service.ListContent(c.Context(), kind)
	if err != nil {
		return serverError(c, err)
	}
	return c.JSON(response)
}

// detail godoc
// @Summary Detail konten dan draft admin
// @Description Memerlukan role Admin. Data berisi resource tersimpan dan draft berisi perubahan yang belum diterbitkan, atau null. Content lesson disanitasi. Question set menyertakan seluruh soal dan pilihan beserta jawaban benar.
// @Tags Admin Content
// @Produce json
// @Security BearerAuth
// @Param kind path string true "Jenis resource" Enums(course,module,lesson,question_set)
// @Param id path string true "UUID resource" Format(uuid)
// @Success 200 {object} model.ContentDetailResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/content/{kind}/{id} [get]
func (h *Handler) detail(c fiber.Ctx) error {
	kind := c.Params("kind")
	if !validKind(kind) {
		return bad(c, "jenis konten tidak valid")
	}
	id, err := resourceID(c)
	if err != nil {
		return bad(c, "ID tidak valid")
	}
	response, err := h.service.Detail(c.Context(), kind, id)
	if err != nil {
		return serverError(c, err)
	}
	return c.JSON(response)
}

// create godoc
// @Summary Buat konten admin
// @Description Memerlukan role Admin. Membuat resource yang belum diterbitkan. Kirim field sesuai kind: course memerlukan level_id dan slug; module memerlukan course_id dan slug; lesson memerlukan module_id dan slug. Title wajib untuk semua jenis. Question_set memerlukan kind (practice dengan lesson_id saja, atau final_exam dengan module_id saja) dan skill (reading/writing); questions disimpan sebagai draft dan boleh belum lengkap. Validasi soal lengkap dilakukan saat publish.
// @Tags Admin Content
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param kind path string true "Jenis resource" Enums(course,module,lesson,question_set)
// @Param request body model.ContentRequest true "Payload lengkap sesuai jenis resource"
// @Success 201 {object} model.CreateResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 409 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/content/{kind} [post]
func (h *Handler) create(c fiber.Ctx) error {
	kind := c.Params("kind")
	if !validKind(kind) {
		return bad(c, "jenis konten tidak valid")
	}
	var response CreateResponse
	var err error
	if kind == "question_set" {
		var input setInput
		if err := json.Unmarshal(c.Body(), &input); err != nil {
			return bad(c, decodeError(err).Error())
		}
		response, err = h.service.CreateQuestionSet(c.Context(), input)
	} else {
		var input coreInput
		if err := json.Unmarshal(c.Body(), &input); err != nil {
			return bad(c, decodeError(err).Error())
		}
		response, err = h.service.CreateContent(c.Context(), kind, input)
	}
	if err != nil {
		return serverError(c, err)
	}
	return c.Status(201).JSON(response)
}

// update godoc
// @Summary Perbarui konten admin
// @Description Memerlukan role Admin. Kirim payload lengkap sesuai kind seperti pada create; PATCH tidak menggabungkan field dengan data lama. Course/module/lesson yang sudah terbit disimpan sebagai draft; yang belum terbit diperbarui langsung. Question_set selalu menyimpan payload sebagai draft. Respons draft menunjukkan apakah resource sudah terbit. Kind, skill, dan penempatan question_set terbit tidak dapat diubah; versi belum terbit yang memiliki attempt tidak dapat diedit (409).
// @Tags Admin Content
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param kind path string true "Jenis resource" Enums(course,module,lesson,question_set)
// @Param id path string true "UUID resource" Format(uuid)
// @Param request body model.ContentRequest true "Payload lengkap sesuai jenis resource"
// @Success 200 {object} model.UpdateResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 409 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/content/{kind}/{id} [patch]
func (h *Handler) update(c fiber.Ctx) error {
	kind := c.Params("kind")
	if !validKind(kind) {
		return bad(c, "jenis konten tidak valid")
	}
	id, err := resourceID(c)
	if err != nil {
		return bad(c, "ID tidak valid")
	}
	var response UpdateResponse
	if kind == "question_set" {
		var input setInput
		if err := json.Unmarshal(c.Body(), &input); err != nil {
			return bad(c, decodeError(err).Error())
		}
		response, err = h.service.UpdateQuestionSet(c.Context(), id, input)
	} else {
		var input coreInput
		if err := json.Unmarshal(c.Body(), &input); err != nil {
			return bad(c, decodeError(err).Error())
		}
		response, err = h.service.UpdateContent(c.Context(), kind, id, input)
	}
	if err != nil {
		return serverError(c, err)
	}
	return c.JSON(response)
}

// publish godoc
// @Summary Terbitkan konten admin
// @Description Memerlukan role Admin. Menerapkan draft lalu menerbitkan resource; arsip harus dipulihkan terlebih dahulu. Question_set wajib memiliki soal valid: reading memakai multiple_choice dengan minimal dua pilihan dan tepat satu jawaban benar; writing memakai stroke_writing dengan stroke_template_id dan tanpa pilihan. Setiap soal memerlukan prompt, poin positif, dan stimulus atau template. Menerbitkan perubahan question_set yang sudah terbit membuat versi baru dalam lineage yang sama; respons id adalah UUID versi aktif dan dapat berbeda dari id permintaan. Konflik versi atau konfigurasi unik menghasilkan 409.
// @Tags Admin Content
// @Produce json
// @Security BearerAuth
// @Param kind path string true "Jenis resource" Enums(course,module,lesson,question_set)
// @Param id path string true "UUID resource" Format(uuid)
// @Success 200 {object} model.PublishResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 409 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/content/{kind}/{id}/publish [post]
func (h *Handler) publish(c fiber.Ctx) error {
	kind := c.Params("kind")
	if !validKind(kind) {
		return bad(c, "jenis konten tidak valid")
	}
	id, err := resourceID(c)
	if err != nil {
		return bad(c, "ID tidak valid")
	}
	response, err := h.service.PublishContent(c.Context(), kind, id)
	if err != nil {
		return serverError(c, err)
	}
	return c.JSON(response)
}

// remove godoc
// @Summary Hapus atau arsipkan konten admin
// @Description Memerlukan role Admin. Menghapus resource jika tidak memiliki riwayat atau relasi yang menghalangi penghapusan; jika ada, mengarsipkan dan menonaktifkan publikasi. Draft dihapus. Respons archived menunjukkan apakah resource diarsipkan.
// @Tags Admin Content
// @Produce json
// @Security BearerAuth
// @Param kind path string true "Jenis resource" Enums(course,module,lesson,question_set)
// @Param id path string true "UUID resource" Format(uuid)
// @Success 200 {object} model.RemoveResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/content/{kind}/{id} [delete]
func (h *Handler) remove(c fiber.Ctx) error {
	kind := c.Params("kind")
	if !validKind(kind) {
		return bad(c, "jenis konten tidak valid")
	}
	id, err := resourceID(c)
	if err != nil {
		return bad(c, "ID tidak valid")
	}
	response, err := h.service.RemoveContent(c.Context(), kind, id)
	if err != nil {
		return serverError(c, err)
	}
	return c.JSON(response)
}

// restore godoc
// @Summary Pulihkan konten yang diarsipkan
// @Description Memerlukan role Admin. Menghapus status arsip dan menjadikan resource belum diterbitkan. Resource yang tidak ditemukan atau tidak diarsipkan menghasilkan 404.
// @Tags Admin Content
// @Produce json
// @Security BearerAuth
// @Param kind path string true "Jenis resource" Enums(course,module,lesson,question_set)
// @Param id path string true "UUID resource" Format(uuid)
// @Success 200 {object} model.StatusResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/content/{kind}/{id}/restore [post]
func (h *Handler) restore(c fiber.Ctx) error {
	kind := c.Params("kind")
	if !validKind(kind) {
		return bad(c, "jenis konten tidak valid")
	}
	id, err := resourceID(c)
	if err != nil {
		return bad(c, "ID tidak valid")
	}
	response, err := h.service.RestoreContent(c.Context(), kind, id)
	if err != nil {
		return serverError(c, err)
	}
	return c.JSON(response)
}

// reorder godoc
// @Summary Simpan urutan module atau lesson
// @Description Memerlukan role Admin. ordered_ids wajib memuat seluruh anak parent, termasuk draft dan arsip. Module diurutkan dalam course; lesson dalam module. Urutan langsung berlaku tanpa publish. Hanya order_index pada draft dengan parent yang sama yang disinkronkan; perubahan materi lainnya dipertahankan. Seluruh perubahan disimpan dalam satu transaksi.
// @Tags Admin Content
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param kind path string true "Jenis konten" Enums(module,lesson)
// @Param request body model.ReorderRequest true "Parent dan seluruh ID sesuai urutan baru"
// @Success 200 {object} model.StatusResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 409 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/content/{kind}/reorder [post]
func (h *Handler) reorder(c fiber.Ctx) error {
	kind := c.Params("kind")
	if kind != "module" && kind != "lesson" {
		return bad(c, "jenis konten tidak valid")
	}
	var input ReorderRequest
	if err := json.Unmarshal(c.Body(), &input); err != nil {
		return bad(c, "data urutan tidak valid")
	}
	response, err := h.service.Reorder(c.Context(), kind, input)
	if err != nil {
		return serverError(c, err)
	}
	return c.JSON(response)
}

// templateDetail godoc
// @Summary Detail template kana
// @Description Memerlukan role Admin. Mengambil metadata dan seluruh stroke_data.
// @Tags Admin
// @Produce json
// @Security BearerAuth
// @Param id path string true "UUID template" Format(uuid)
// @Success 200 {object} model.StrokeTemplateResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/templates/{id} [get]
func (h *Handler) templateDetail(c fiber.Ctx) error {
	id, err := resourceID(c)
	if err != nil {
		return bad(c, "ID template tidak valid")
	}
	response, err := h.service.Template(c.Context(), id)
	if err != nil {
		return templateServerError(c, err)
	}
	return c.JSON(response)
}

// previewTemplate godoc
// @Summary Pratinjau SVG template kana
// @Description Memerlukan role Admin. Mengonversi SVG KanjiVG maksimal 256 KiB tanpa menyimpan; menghasilkan 32 titik per goresan.
// @Tags Admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body model.StrokeTemplateInput true "Metadata dan SVG KanjiVG"
// @Success 200 {object} model.StrokeTemplatePreviewResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 413 {object} utils.ErrorRes
// @Router /admin/templates/preview [post]
func (h *Handler) previewTemplate(c fiber.Ctx) error {
	input, err := decodeTemplateInput(c.Body())
	if err != nil {
		return bad(c, decodeError(err).Error())
	}
	response, err := h.service.PreviewTemplate(c.Context(), input)
	if err != nil {
		return templateServerError(c, err)
	}
	return c.JSON(response)
}

// createTemplate godoc
// @Summary Tambah template kana
// @Description Memerlukan role Admin. Mengonversi SVG KanjiVG dan menyimpan template langsung tanpa draft/publish.
// @Tags Admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body model.StrokeTemplateInput true "Metadata dan SVG KanjiVG"
// @Success 201 {object} model.StrokeTemplateResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 409 {object} utils.ErrorRes
// @Failure 413 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/templates [post]
func (h *Handler) createTemplate(c fiber.Ctx) error {
	input, err := decodeTemplateInput(c.Body())
	if err != nil {
		return bad(c, decodeError(err).Error())
	}
	response, err := h.service.CreateTemplate(c.Context(), input)
	if err != nil {
		return templateServerError(c, err)
	}
	return c.Status(201).JSON(response)
}

// updateTemplate godoc
// @Summary Edit template kana secara langsung
// @Description Memerlukan role Admin. Kirim semua metadata. SVG opsional kecuali script/karakter berubah; jika tidak dikirim, stroke_data tersimpan dipertahankan. ID tetap, perubahan langsung berlaku untuk attempt baru; attempt lama memakai snapshot.
// @Tags Admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "UUID template" Format(uuid)
// @Param request body model.StrokeTemplateInput true "Metadata dan SVG pengganti opsional"
// @Success 200 {object} model.StrokeTemplateResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 409 {object} utils.ErrorRes
// @Failure 413 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/templates/{id} [patch]
func (h *Handler) updateTemplate(c fiber.Ctx) error {
	id, err := resourceID(c)
	if err != nil {
		return bad(c, "ID template tidak valid")
	}
	input, err := decodeTemplateInput(c.Body())
	if err != nil {
		return bad(c, decodeError(err).Error())
	}
	response, err := h.service.UpdateTemplate(c.Context(), id, input)
	if err != nil {
		return templateServerError(c, err)
	}
	return c.JSON(response)
}

// decodeTemplateInput keeps the template endpoint's strict JSON decoding policy.
func decodeTemplateInput(raw []byte) (StrokeTemplateInput, error) {
	var input *StrokeTemplateInput
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || input == nil {
		return StrokeTemplateInput{}, errors.New("payload template tidak valid")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return StrokeTemplateInput{}, errors.New("payload template tidak valid")
	}
	return *input, nil
}
