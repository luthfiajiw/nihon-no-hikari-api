package question

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"nihon-no-hikari-api/pkg/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

func bad(c fiber.Ctx, message string) error {
	return c.Status(400).JSON(utils.ErrorRes{Success: false, Error: message})
}

func serverError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrInvalidInput):
		return bad(c, decodeError(err).Error())
	case errors.Is(err, ErrPayloadTooLarge):
		return c.Status(413).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
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

// templates godoc
// @Summary Daftar template goresan untuk admin
// @Description Memerlukan role Admin. Mengambil metadata template kana untuk stroke_template_id; tidak menyertakan stroke_data.
// @Tags Admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} TemplatesResponse
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

// templateDetail godoc
// @Summary Detail template kana
// @Description Memerlukan role Admin. Mengambil metadata dan seluruh stroke_data.
// @Tags Admin
// @Produce json
// @Security BearerAuth
// @Param id path string true "UUID template" Format(uuid)
// @Success 200 {object} StrokeTemplateResponse
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
// @Param request body StrokeTemplateInput true "Metadata dan SVG KanjiVG"
// @Success 200 {object} StrokeTemplatePreviewResponse
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
// @Param request body StrokeTemplateInput true "Metadata dan SVG KanjiVG"
// @Success 201 {object} StrokeTemplateResponse
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
// @Param request body StrokeTemplateInput true "Metadata dan SVG pengganti opsional"
// @Success 200 {object} StrokeTemplateResponse
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

// list godoc
// @Summary Daftar konten admin
// @Description Memerlukan role Admin. Mengambil seluruh resource domain ini termasuk draft dan arsip, berdasarkan order_index, created_at, dan id.
// @Tags Question Admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} ContentListResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/question-sets [get]
func (h *Handler) adminList(c fiber.Ctx) error {
	response, err := h.service.ListAdminQuestionSets(c.Context())
	if err != nil {
		return serverError(c, err)
	}
	return c.JSON(response)
}

// detail godoc
// @Summary Detail konten dan draft admin
// @Description Memerlukan role Admin. Data berisi resource tersimpan; draft berisi perubahan yang belum diterbitkan, atau null.
// @Tags Question Admin
// @Produce json
// @Security BearerAuth
// @Param id path string true "UUID resource" Format(uuid)
// @Success 200 {object} ContentDetailResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/question-sets/{id} [get]
func (h *Handler) adminDetail(c fiber.Ctx) error {
	id, err := resourceID(c)
	if err != nil {
		return bad(c, "ID tidak valid")
	}
	response, err := h.service.AdminQuestionSetDetail(c.Context(), id)
	if err != nil {
		return serverError(c, err)
	}
	return c.JSON(response)
}

// create godoc
// @Summary Buat konten admin
// @Description Memerlukan role Admin. Membuat question set practice dengan lesson_id atau final_exam dengan module_id. Skill reading/writing wajib; questions disimpan sebagai draft dan divalidasi lengkap saat publish.
// @Tags Question Admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body SetInput true "Payload lengkap sesuai jenis resource"
// @Success 201 {object} CreateResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 409 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/question-sets [post]
func (h *Handler) adminCreate(c fiber.Ctx) error {
	var input SetInput
	if err := json.Unmarshal(c.Body(), &input); err != nil {
		return bad(c, decodeError(err).Error())
	}
	response, err := h.service.CreateQuestionSet(c.Context(), input)
	if err != nil {
		return serverError(c, err)
	}
	return c.Status(201).JSON(response)
}

// update godoc
// @Summary Perbarui konten admin
// @Description Memerlukan role Admin. PATCH menerima payload lengkap dan menyimpan perubahan sebagai draft. Kind, skill, dan penempatan versi terbit tidak dapat diubah; versi lama dengan attempt tidak dapat diedit.
// @Tags Question Admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "UUID resource" Format(uuid)
// @Param request body SetInput true "Payload lengkap sesuai jenis resource"
// @Success 200 {object} UpdateResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 409 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/question-sets/{id} [patch]
func (h *Handler) adminUpdate(c fiber.Ctx) error {
	id, err := resourceID(c)
	if err != nil {
		return bad(c, "ID tidak valid")
	}
	var input SetInput
	if err := json.Unmarshal(c.Body(), &input); err != nil {
		return bad(c, decodeError(err).Error())
	}
	response, err := h.service.UpdateQuestionSet(c.Context(), id, input)
	if err != nil {
		return serverError(c, err)
	}
	return c.JSON(response)
}

// publish godoc
// @Summary Terbitkan konten admin
// @Description Memerlukan role Admin. Memvalidasi seluruh soal dan menerbitkan draft secara atomik. Perubahan versi terbit membuat versi baru dalam lineage yang sama; respons id dapat berbeda dari id permintaan. Riwayat attempt tetap dipertahankan.
// @Tags Question Admin
// @Produce json
// @Security BearerAuth
// @Param id path string true "UUID resource" Format(uuid)
// @Success 200 {object} PublishResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 409 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/question-sets/{id}/publish [post]
func (h *Handler) adminPublish(c fiber.Ctx) error {
	id, err := resourceID(c)
	if err != nil {
		return bad(c, "ID tidak valid")
	}
	response, err := h.service.PublishQuestionSet(c.Context(), id)
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
// @Success 200 {object} RemoveResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/question-sets/{id} [delete]
func (h *Handler) adminRemove(c fiber.Ctx) error {
	id, err := resourceID(c)
	if err != nil {
		return bad(c, "ID tidak valid")
	}
	response, err := h.service.RemoveQuestionSet(c.Context(), id)
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
// @Success 200 {object} StatusResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/question-sets/{id}/restore [post]
func (h *Handler) adminRestore(c fiber.Ctx) error {
	id, err := resourceID(c)
	if err != nil {
		return bad(c, "ID tidak valid")
	}
	response, err := h.service.RestoreQuestionSet(c.Context(), id)
	if err != nil {
		return serverError(c, err)
	}
	return c.JSON(response)
}
