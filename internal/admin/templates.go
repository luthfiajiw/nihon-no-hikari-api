package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode/utf8"

	"nihon-no-hikari-api/pkg/kanastroke"
	"nihon-no-hikari-api/pkg/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/text/unicode/norm"
)

type StrokeTemplateInput struct {
	Script        string  `json:"script" enums:"hiragana,katakana"`
	Character     string  `json:"character" example:"あ"`
	Romaji        string  `json:"romaji" example:"a"`
	SourceVersion string  `json:"source_version" example:"r20260714"`
	SVGContent    *string `json:"svg_content,omitempty"`
}

type StrokeTemplateDetail struct {
	StrokeTemplate
	StrokeData kanastroke.Data `json:"stroke_data"`
}

type StrokeTemplateResponse struct {
	Success bool                 `json:"success"`
	Data    StrokeTemplateDetail `json:"data"`
}

type StrokeTemplatePreview struct {
	Script        string          `json:"script"`
	Character     string          `json:"character"`
	Romaji        string          `json:"romaji"`
	SourceVersion string          `json:"source_version"`
	StrokeCount   int             `json:"stroke_count"`
	StrokeData    kanastroke.Data `json:"stroke_data"`
}

type StrokeTemplatePreviewResponse struct {
	Success bool                  `json:"success"`
	Data    StrokeTemplatePreview `json:"data"`
}

func parseTemplateInput(raw []byte) (StrokeTemplateInput, error) {
	var input *StrokeTemplateInput
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || input == nil {
		return StrokeTemplateInput{}, errors.New("payload template tidak valid")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return StrokeTemplateInput{}, errors.New("payload template tidak valid")
	}
	input.Script = strings.TrimSpace(input.Script)
	input.Character = norm.NFC.String(strings.TrimSpace(input.Character))
	input.Romaji = strings.TrimSpace(input.Romaji)
	input.SourceVersion = strings.TrimSpace(input.SourceVersion)
	for _, value := range []string{input.Script, input.Character, input.Romaji, input.SourceVersion} {
		if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
			return *input, errors.New("metadata template berisi karakter tidak valid")
		}
	}
	if !kanastroke.ValidCharacter(input.Script, input.Character) {
		return *input, errors.New("karakter harus berupa satu hiragana atau katakana sesuai script")
	}
	if input.Romaji == "" || !utf8.ValidString(input.Romaji) || utf8.RuneCountInString(input.Romaji) > 8 {
		return *input, errors.New("romaji wajib diisi dan maksimal 8 karakter")
	}
	if input.SourceVersion == "" {
		return *input, errors.New("versi sumber wajib diisi")
	}
	if input.SVGContent != nil && len(*input.SVGContent) > kanastroke.MaxSVGBytes {
		return *input, kanastroke.ErrTooLarge
	}
	return *input, nil
}

func templateInputError(c fiber.Ctx, err error) error {
	if errors.Is(err, kanastroke.ErrTooLarge) {
		return c.Status(413).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
	}
	return bad(c, err.Error())
}

func templateServerError(c fiber.Ctx, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return c.Status(404).JSON(utils.ErrorRes{Success: false, Error: "template kana tidak ditemukan"})
	}
	var pgerr *pgconn.PgError
	if errors.As(err, &pgerr) && pgerr.Code == "23505" {
		return c.Status(409).JSON(utils.ErrorRes{Success: false, Error: "template dengan script, karakter, dan versi sumber tersebut sudah ada"})
	}
	return c.Status(500).JSON(utils.ErrorRes{Success: false, Error: "gagal menyimpan atau mengambil template kana"})
}

type templateQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func loadTemplate(ctx context.Context, querier templateQuerier, id uuid.UUID, lock bool) (StrokeTemplateDetail, error) {
	query := `SELECT id,script,character,romaji,source_version,stroke_data FROM public.kana_stroke_templates WHERE id=$1`
	if lock {
		query += ` FOR UPDATE`
	}
	var item StrokeTemplateDetail
	err := querier.QueryRow(ctx, query, id).Scan(&item.ID, &item.Script, &item.Character, &item.Romaji, &item.SourceVersion, &item.StrokeData)
	item.StrokeCount = len(item.StrokeData.Strokes)
	return item, err
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
	item, err := loadTemplate(c.Context(), h.pool, id, false)
	if err != nil {
		return templateServerError(c, err)
	}
	return c.JSON(StrokeTemplateResponse{Success: true, Data: item})
}

func convertTemplateInput(input StrokeTemplateInput) (StrokeTemplatePreview, error) {
	if input.SVGContent == nil || strings.TrimSpace(*input.SVGContent) == "" {
		return StrokeTemplatePreview{}, errors.New("file SVG KanjiVG wajib diunggah")
	}
	data, err := kanastroke.Convert(*input.SVGContent, input.Script, input.Character)
	return StrokeTemplatePreview{Script: input.Script, Character: input.Character, Romaji: input.Romaji,
		SourceVersion: input.SourceVersion, StrokeCount: len(data.Strokes), StrokeData: data}, err
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
	input, err := parseTemplateInput(c.Body())
	if err != nil {
		return templateInputError(c, err)
	}
	preview, err := convertTemplateInput(input)
	if err != nil {
		return templateInputError(c, err)
	}
	return c.JSON(StrokeTemplatePreviewResponse{Success: true, Data: preview})
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
	input, err := parseTemplateInput(c.Body())
	if err != nil {
		return templateInputError(c, err)
	}
	preview, err := convertTemplateInput(input)
	if err != nil {
		return templateInputError(c, err)
	}
	item := StrokeTemplateDetail{StrokeTemplate: StrokeTemplate{ID: uuid.New(), Script: input.Script, Character: input.Character,
		Romaji: input.Romaji, SourceVersion: input.SourceVersion, StrokeCount: preview.StrokeCount}, StrokeData: preview.StrokeData}
	strokeJSON, err := json.Marshal(item.StrokeData)
	if err != nil {
		return templateServerError(c, err)
	}
	_, err = h.pool.Exec(c.Context(), `INSERT INTO public.kana_stroke_templates(id,script,character,romaji,source_version,stroke_data) VALUES($1,$2,$3,$4,$5,$6::jsonb)`,
		item.ID, item.Script, item.Character, item.Romaji, item.SourceVersion, string(strokeJSON))
	if err != nil {
		return templateServerError(c, err)
	}
	return c.Status(201).JSON(StrokeTemplateResponse{Success: true, Data: item})
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
	input, err := parseTemplateInput(c.Body())
	if err != nil {
		return templateInputError(c, err)
	}
	// Parse uploaded geometry before taking a row lock.
	var converted *kanastroke.Data
	if input.SVGContent != nil {
		preview, err := convertTemplateInput(input)
		if err != nil {
			return templateInputError(c, err)
		}
		converted = &preview.StrokeData
	}
	tx, err := h.pool.Begin(c.Context())
	if err != nil {
		return templateServerError(c, err)
	}
	defer func() { _ = tx.Rollback(c.Context()) }()
	item, err := loadTemplate(c.Context(), tx, id, true)
	if err != nil {
		return templateServerError(c, err)
	}
	if converted == nil && (input.Script != item.Script || input.Character != item.Character) {
		return bad(c, "unggah SVG pengganti saat script atau karakter berubah")
	}
	if converted != nil {
		item.StrokeData = *converted
	}
	item.Script, item.Character, item.Romaji, item.SourceVersion = input.Script, input.Character, input.Romaji, input.SourceVersion
	item.StrokeCount = len(item.StrokeData.Strokes)
	strokeJSON, err := json.Marshal(item.StrokeData)
	if err != nil {
		return templateServerError(c, err)
	}
	_, err = tx.Exec(c.Context(), `UPDATE public.kana_stroke_templates SET script=$2,character=$3,romaji=$4,source_version=$5,stroke_data=$6::jsonb WHERE id=$1`,
		id, item.Script, item.Character, item.Romaji, item.SourceVersion, string(strokeJSON))
	if err != nil {
		return templateServerError(c, err)
	}
	if err := tx.Commit(c.Context()); err != nil {
		return templateServerError(c, err)
	}
	return c.JSON(StrokeTemplateResponse{Success: true, Data: item})
}
