package course

import (
	"encoding/json"
	"errors"

	"nihon-no-hikari-api/internal/course/model"
	"nihon-no-hikari-api/pkg/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

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

func resourceID(c fiber.Ctx) (uuid.UUID, error) { return uuid.Parse(c.Params("id")) }

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
	response, err := h.service.AdminCatalog(c.Context(), courseID)
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
	response, err := h.service.AdminLevels(c.Context())
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
// @Router /admin/lessons/preview [post]
func (h *Handler) preview(c fiber.Ctx) error {
	var body model.PreviewRequest
	if err := c.Bind().Body(&body); err != nil {
		return bad(c, "konten tidak valid")
	}
	response, err := h.service.PreviewLessonContent(c.Context(), body.Content)
	if err != nil {
		return serverError(c, err)
	}
	return c.JSON(response)
}

// list godoc
// @Summary Daftar konten admin
// @Description Memerlukan role Admin. Mengambil seluruh resource domain ini termasuk draft dan arsip, berdasarkan order_index, created_at, dan id.
// @Tags Course Admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} model.ContentListResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/courses [get]
// @Router /admin/modules [get]
// @Router /admin/lessons [get]
func (h *Handler) adminList(c fiber.Ctx, kind string) error {
	response, err := h.service.ListAdminContent(c.Context(), kind)
	if err != nil {
		return serverError(c, err)
	}
	return c.JSON(response)
}

// detail godoc
// @Summary Detail konten dan draft admin
// @Description Memerlukan role Admin. Data berisi resource tersimpan; draft berisi perubahan yang belum diterbitkan, atau null.
// @Tags Course Admin
// @Produce json
// @Security BearerAuth
// @Param id path string true "UUID resource" Format(uuid)
// @Success 200 {object} model.ContentDetailResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/courses/{id} [get]
// @Router /admin/modules/{id} [get]
// @Router /admin/lessons/{id} [get]
func (h *Handler) adminDetail(c fiber.Ctx, kind string) error {
	id, err := resourceID(c)
	if err != nil {
		return bad(c, "ID tidak valid")
	}
	response, err := h.service.AdminContentDetail(c.Context(), kind, id)
	if err != nil {
		return serverError(c, err)
	}
	return c.JSON(response)
}

// create godoc
// @Summary Buat konten admin
// @Description Memerlukan role Admin. Membuat course, module, atau lesson yang belum terbit. Course memerlukan level_id; module memerlukan course_id; lesson memerlukan module_id. Title dan slug wajib.
// @Tags Course Admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body model.ContentRequest true "Payload lengkap sesuai jenis resource"
// @Success 201 {object} model.CreateResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 409 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/courses [post]
// @Router /admin/modules [post]
// @Router /admin/lessons [post]
func (h *Handler) adminCreate(c fiber.Ctx, kind string) error {
	var input model.CoreInput
	if err := json.Unmarshal(c.Body(), &input); err != nil {
		return bad(c, decodeError(err).Error())
	}
	response, err := h.service.CreateAdminContent(c.Context(), kind, input)
	if err != nil {
		return serverError(c, err)
	}
	return c.Status(201).JSON(response)
}

// update godoc
// @Summary Perbarui konten admin
// @Description Memerlukan role Admin. PATCH menerima payload lengkap. Resource terbit menyimpan perubahan sebagai draft; resource belum terbit diperbarui langsung.
// @Tags Course Admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "UUID resource" Format(uuid)
// @Param request body model.ContentRequest true "Payload lengkap sesuai jenis resource"
// @Success 200 {object} model.UpdateResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 409 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/courses/{id} [patch]
// @Router /admin/modules/{id} [patch]
// @Router /admin/lessons/{id} [patch]
func (h *Handler) adminUpdate(c fiber.Ctx, kind string) error {
	id, err := resourceID(c)
	if err != nil {
		return bad(c, "ID tidak valid")
	}
	var input model.CoreInput
	if err := json.Unmarshal(c.Body(), &input); err != nil {
		return bad(c, decodeError(err).Error())
	}
	response, err := h.service.UpdateAdminContent(c.Context(), kind, id, input)
	if err != nil {
		return serverError(c, err)
	}
	return c.JSON(response)
}

// publish godoc
// @Summary Terbitkan konten admin
// @Description Memerlukan role Admin. Menerapkan draft dan menerbitkan course, module, atau lesson secara atomik. Arsip harus dipulihkan terlebih dahulu.
// @Tags Course Admin
// @Produce json
// @Security BearerAuth
// @Param id path string true "UUID resource" Format(uuid)
// @Success 200 {object} model.PublishResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 409 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/courses/{id}/publish [post]
// @Router /admin/modules/{id}/publish [post]
// @Router /admin/lessons/{id}/publish [post]
func (h *Handler) adminPublish(c fiber.Ctx, kind string) error {
	id, err := resourceID(c)
	if err != nil {
		return bad(c, "ID tidak valid")
	}
	response, err := h.service.PublishAdminContent(c.Context(), kind, id)
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
// @Param id path string true "UUID resource" Format(uuid)
// @Success 200 {object} model.RemoveResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/courses/{id} [delete]
// @Router /admin/modules/{id} [delete]
// @Router /admin/lessons/{id} [delete]
func (h *Handler) adminRemove(c fiber.Ctx, kind string) error {
	id, err := resourceID(c)
	if err != nil {
		return bad(c, "ID tidak valid")
	}
	response, err := h.service.RemoveAdminContent(c.Context(), kind, id)
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
// @Param id path string true "UUID resource" Format(uuid)
// @Success 200 {object} model.StatusResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/courses/{id}/restore [post]
// @Router /admin/modules/{id}/restore [post]
// @Router /admin/lessons/{id}/restore [post]
func (h *Handler) adminRestore(c fiber.Ctx, kind string) error {
	id, err := resourceID(c)
	if err != nil {
		return bad(c, "ID tidak valid")
	}
	response, err := h.service.RestoreAdminContent(c.Context(), kind, id)
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
// @Param request body model.ReorderRequest true "Parent dan seluruh ID sesuai urutan baru"
// @Success 200 {object} model.StatusResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 409 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/modules/reorder [post]
// @Router /admin/lessons/reorder [post]
func (h *Handler) adminReorder(c fiber.Ctx, kind string) error {
	if kind != "module" && kind != "lesson" {
		return bad(c, "jenis konten tidak valid")
	}
	var input model.ReorderRequest
	if err := json.Unmarshal(c.Body(), &input); err != nil {
		return bad(c, "data urutan tidak valid")
	}
	response, err := h.service.ReorderAdminContent(c.Context(), kind, input)
	if err != nil {
		return serverError(c, err)
	}
	return c.JSON(response)
}

func (h *Handler) registerAdminContent(g fiber.Router, path, kind string) {
	g.Get(path, func(c fiber.Ctx) error { return h.adminList(c, kind) })
	g.Post(path, func(c fiber.Ctx) error { return h.adminCreate(c, kind) })
	g.Get(path+"/:id", func(c fiber.Ctx) error { return h.adminDetail(c, kind) })
	g.Patch(path+"/:id", func(c fiber.Ctx) error { return h.adminUpdate(c, kind) })
	g.Delete(path+"/:id", func(c fiber.Ctx) error { return h.adminRemove(c, kind) })
	g.Post(path+"/:id/publish", func(c fiber.Ctx) error { return h.adminPublish(c, kind) })
	g.Post(path+"/:id/restore", func(c fiber.Ctx) error { return h.adminRestore(c, kind) })
}
