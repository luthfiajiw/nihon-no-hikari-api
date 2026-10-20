package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"nihon-no-hikari-api/pkg/contenthtml"
	"nihon-no-hikari-api/pkg/middleware"
	"nihon-no-hikari-api/pkg/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	pool       *pgxpool.Pool
	middleware middleware.Middleware
}

func NewHandler(pool *pgxpool.Pool, m middleware.Middleware) *Handler {
	return &Handler{pool: pool, middleware: m}
}

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

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

func table(kind string) (string, bool) {
	switch kind {
	case "course":
		return "courses", true
	case "module":
		return "modules", true
	case "lesson":
		return "lessons", true
	case "question_set":
		return "question_sets", true
	default:
		return "", false
	}
}

func bad(c fiber.Ctx, message string) error {
	return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{Success: false, Error: message})
}

func serverError(c fiber.Ctx, err error) error {
	if errors.Is(err, errInvalidPrerequisites) {
		return bad(c, err.Error())
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return c.Status(404).JSON(utils.ErrorRes{Success: false, Error: "konten tidak ditemukan"})
	}
	if strings.Contains(err.Error(), "SQLSTATE 23505") {
		return c.Status(409).JSON(utils.ErrorRes{Success: false, Error: "slug atau urutan sudah dipakai"})
	}
	if strings.Contains(err.Error(), "SQLSTATE 23503") || strings.Contains(err.Error(), "SQLSTATE 23514") {
		return bad(c, "relasi atau konfigurasi konten tidak valid")
	}
	return c.Status(500).JSON(utils.ErrorRes{Success: false, Error: "gagal menyimpan konten"})
}

func resourceID(c fiber.Ctx) (uuid.UUID, error) { return uuid.Parse(c.Params("id")) }

func (h *Handler) rows(ctx context.Context, query string, args ...any) ([]json.RawMessage, error) {
	rows, err := h.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]json.RawMessage, 0)
	for rows.Next() {
		var item json.RawMessage
		if err := rows.Scan(&item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// me godoc
// @Summary Periksa akses admin
// @Description Memastikan sesi pengguna memiliki role Admin.
// @Tags Admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} MeResponse
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Router /admin/me [get]
func (h *Handler) me(c fiber.Ctx) error {
	return c.JSON(fiber.Map{"success": true, "role": "Admin"})
}

// catalog godoc
// @Summary Katalog konten admin
// @Description Memerlukan role Admin. Mengambil konten termasuk draft dan arsip. Jika course_id diisi, hanya modules dan lessons kursus tersebut yang dikembalikan; courses dan question_sets berupa array kosong.
// @Tags Admin
// @Produce json
// @Security BearerAuth
// @Param course_id query string false "Filter UUID kursus" Format(uuid)
// @Success 200 {object} CatalogResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/catalog [get]
func (h *Handler) catalog(c fiber.Ctx) error {
	data := map[string][]json.RawMessage{}
	var courseID *uuid.UUID
	if raw := c.Query("course_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return bad(c, "ID kursus tidak valid")
		}
		courseID = &id
	}
	for kind, name := range map[string]string{"courses": "courses", "modules": "modules", "lessons": "lessons", "question_sets": "question_sets"} {
		data[kind] = []json.RawMessage{}
		if courseID != nil && name != "modules" && name != "lessons" {
			continue
		}
		projection := "to_jsonb(t)"
		if name == "lessons" {
			projection = "jsonb_build_object('id',t.id,'module_id',t.module_id,'title',t.title,'slug',t.slug,'order_index',t.order_index,'is_published',t.is_published,'archived_at',t.archived_at)"
		}
		where := ""
		args := []any{}
		if courseID != nil {
			args = append(args, *courseID)
			if name == "modules" {
				where = " WHERE t.course_id=$1"
			} else {
				where = " WHERE t.module_id IN (SELECT id FROM modules WHERE course_id=$1)"
			}
		}
		items, err := h.rows(c.Context(), fmt.Sprintf(`SELECT %s FROM public.%s t%s ORDER BY t.order_index, t.created_at, t.id`, projection, name, where), args...)
		if err != nil {
			return serverError(c, err)
		}
		data[kind] = items
	}
	return c.JSON(fiber.Map{"success": true, "data": data})
}

// levels godoc
// @Summary Daftar level untuk admin
// @Description Memerlukan role Admin. Mengambil level berdasarkan order_index.
// @Tags Admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} LevelsResponse
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/levels [get]
func (h *Handler) levels(c fiber.Ctx) error {
	items, err := h.rows(c.Context(), `SELECT to_jsonb(t) FROM public.levels t ORDER BY order_index`)
	if err != nil {
		return serverError(c, err)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

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
	items, err := h.rows(c.Context(), `SELECT jsonb_build_object('id',id,'character',character,'romaji',romaji,'script',script,'source_version',source_version,'stroke_count',jsonb_array_length(stroke_data->'strokes')) FROM public.kana_stroke_templates ORDER BY script,character,source_version DESC`)
	if err != nil {
		return serverError(c, err)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// preview godoc
// @Summary Pratinjau konten lesson
// @Description Memerlukan role Admin. Membersihkan HTML content dan merapikan tips tanpa menyimpan konten. Field tambahan pada objek content dipertahankan. Content kosong/null menjadi objek dengan content dan tips kosong; string HTML lama juga diterima.
// @Tags Admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body PreviewRequest true "Konten lesson untuk pratinjau"
// @Success 200 {object} PreviewResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Router /admin/lesson-content/preview [post]
func (h *Handler) preview(c fiber.Ctx) error {
	var body struct {
		Content json.RawMessage `json:"content"`
	}
	if err := c.Bind().Body(&body); err != nil {
		return bad(c, "konten tidak valid")
	}
	clean, err := contenthtml.JSON(body.Content)
	if err != nil {
		return bad(c, "konten lesson harus berupa objek dengan content dan tips")
	}
	return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(clean)})
}

// list godoc
// @Summary Daftar konten admin
// @Description Memerlukan role Admin. Mengambil seluruh resource sesuai kind, termasuk yang belum diterbitkan dan diarsipkan, berdasarkan order_index, created_at, dan id. Content lesson tidak disertakan; course menyertakan module_count dan lesson_count.
// @Tags Admin Content
// @Produce json
// @Security BearerAuth
// @Param kind path string true "Jenis resource" Enums(course,module,lesson,question_set)
// @Success 200 {object} ContentListResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/content/{kind} [get]
func (h *Handler) list(c fiber.Ctx) error {
	name, ok := table(c.Params("kind"))
	if !ok {
		return bad(c, "jenis konten tidak valid")
	}
	projection := "to_jsonb(t) - 'content'"
	if name == "courses" {
		projection = `to_jsonb(t) || jsonb_build_object('module_count',(SELECT count(*) FROM modules m WHERE m.course_id=t.id),'lesson_count',(SELECT count(*) FROM lessons l JOIN modules m ON m.id=l.module_id WHERE m.course_id=t.id))`
	}
	items, err := h.rows(c.Context(), fmt.Sprintf(`SELECT %s FROM public.%s t ORDER BY t.order_index,t.created_at,t.id`, projection, name))
	if err != nil {
		return serverError(c, err)
	}
	return c.JSON(fiber.Map{"success": true, "data": items})
}

// detail godoc
// @Summary Detail konten dan draft admin
// @Description Memerlukan role Admin. Data berisi resource tersimpan dan draft berisi perubahan yang belum diterbitkan, atau null. Content lesson disanitasi. Question set menyertakan seluruh soal dan pilihan beserta jawaban benar.
// @Tags Admin Content
// @Produce json
// @Security BearerAuth
// @Param kind path string true "Jenis resource" Enums(course,module,lesson,question_set)
// @Param id path string true "UUID resource" Format(uuid)
// @Success 200 {object} ContentDetailResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/content/{kind}/{id} [get]
func (h *Handler) detail(c fiber.Ctx) error {
	name, ok := table(c.Params("kind"))
	if !ok {
		return bad(c, "jenis konten tidak valid")
	}
	id, err := resourceID(c)
	if err != nil {
		return bad(c, "ID tidak valid")
	}
	var live json.RawMessage
	projection := "to_jsonb(t)"
	if kind := c.Params("kind"); kind == "module" || kind == "lesson" {
		projection += fmt.Sprintf(` || jsonb_build_object('%s_prerequisites', COALESCE((SELECT jsonb_agg(p.required_%s_id ORDER BY p.required_%s_id) FROM %s_prerequisites p WHERE p.%s_id=t.id), '[]'::jsonb))`, kind, kind, kind, kind, kind)
	}
	err = h.pool.QueryRow(c.Context(), fmt.Sprintf(`SELECT %s FROM public.%s t WHERE id=$1`, projection, name), id).Scan(&live)
	if err != nil {
		return serverError(c, err)
	}
	if name == "lessons" {
		var fields map[string]json.RawMessage
		if json.Unmarshal(live, &fields) == nil {
			clean, err := contenthtml.JSON(fields["content"])
			if err == nil {
				fields["content"] = clean
				live, _ = json.Marshal(fields)
			}
		}
	}
	if name == "question_sets" {
		live, err = h.withQuestions(c.Context(), live, id)
		if err != nil {
			return serverError(c, err)
		}
	}
	var draft json.RawMessage
	err = h.pool.QueryRow(c.Context(), `SELECT payload FROM public.admin_drafts WHERE resource_type=$1 AND resource_id=$2`, c.Params("kind"), id).Scan(&draft)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return serverError(c, err)
	}
	return c.JSON(fiber.Map{"success": true, "data": live, "draft": draft})
}

type coreInput struct {
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

func parseCore(kind string, raw []byte) (coreInput, error) {
	var v coreInput
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, err
	}
	if !slugPattern.MatchString(v.Slug) || len(v.Slug) > 120 || strings.TrimSpace(v.Title) == "" || len(v.Title) > 160 || v.OrderIndex < 0 {
		return v, errors.New("slug, judul, atau urutan tidak valid")
	}
	switch kind {
	case "course":
		if v.LevelID == uuid.Nil {
			return v, errors.New("level wajib dipilih")
		}
	case "module":
		if v.CourseID == uuid.Nil {
			return v, errors.New("kursus wajib dipilih")
		}
	case "lesson":
		if v.ModuleID == uuid.Nil {
			return v, errors.New("module wajib dipilih")
		}
		clean, err := contenthtml.JSON(v.Content)
		if err != nil {
			return v, err
		}
		v.Content = clean
	}
	if v.ThumbnailURL != nil && *v.ThumbnailURL != "" {
		u, err := url.Parse(*v.ThumbnailURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return v, errors.New("URL gambar tidak valid")
		}
	}
	if v.EstimatedMinutes != nil && *v.EstimatedMinutes < 0 {
		return v, errors.New("estimasi waktu tidak valid")
	}
	for _, requiredID := range v.prerequisites(kind) {
		if requiredID == uuid.Nil {
			return v, errors.New("ID prasyarat tidak valid")
		}
	}
	return v, nil
}

func saveCore(ctx context.Context, tx pgx.Tx, kind string, id uuid.UUID, v coreInput, create bool) error {
	if err := validatePrerequisites(ctx, tx, kind, id, v.prerequisites(kind)); err != nil {
		return err
	}
	switch kind {
	case "course":
		if create {
			_, err := tx.Exec(ctx, `INSERT INTO courses(id,level_id,slug,title,description,thumbnail_url,order_index) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, v.LevelID, v.Slug, v.Title, v.Description, v.ThumbnailURL, v.OrderIndex)
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE courses SET level_id=$2,slug=$3,title=$4,description=$5,thumbnail_url=$6,order_index=$7,updated_at=now() WHERE id=$1`, id, v.LevelID, v.Slug, v.Title, v.Description, v.ThumbnailURL, v.OrderIndex)
		return err
	case "module":
		if create {
			if _, err := tx.Exec(ctx, `UPDATE modules SET order_index=order_index+1 WHERE course_id=$1 AND order_index >= $2`, v.CourseID, v.OrderIndex); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO modules(id,course_id,slug,title,description,order_index,is_mandatory,is_entry,estimated_minutes) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, v.CourseID, v.Slug, v.Title, v.Description, v.OrderIndex, v.IsMandatory, v.IsEntry, v.EstimatedMinutes)
			if err != nil {
				return err
			}
			return savePrerequisites(ctx, tx, kind, id, v.ModulePrerequisites)
		}
		var oldCourse uuid.UUID
		var oldOrder int16
		if err := tx.QueryRow(ctx, `SELECT course_id,order_index FROM modules WHERE id=$1`, id).Scan(&oldCourse, &oldOrder); err != nil {
			return err
		}
		if oldCourse == v.CourseID {
			if v.OrderIndex < oldOrder {
				if _, err := tx.Exec(ctx, `UPDATE modules SET order_index=order_index+1 WHERE course_id=$1 AND id<>$2 AND order_index >= $3 AND order_index < $4`, oldCourse, id, v.OrderIndex, oldOrder); err != nil {
					return err
				}
			}
			if v.OrderIndex > oldOrder {
				if _, err := tx.Exec(ctx, `UPDATE modules SET order_index=order_index-1 WHERE course_id=$1 AND id<>$2 AND order_index > $3 AND order_index <= $4`, oldCourse, id, oldOrder, v.OrderIndex); err != nil {
					return err
				}
			}
		} else {
			if _, err := tx.Exec(ctx, `UPDATE modules SET order_index=order_index-1 WHERE course_id=$1 AND order_index > $2`, oldCourse, oldOrder); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE modules SET order_index=order_index+1 WHERE course_id=$1 AND order_index >= $2`, v.CourseID, v.OrderIndex); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `UPDATE modules SET course_id=$2,slug=$3,title=$4,description=$5,order_index=$6,is_mandatory=$7,is_entry=$8,estimated_minutes=$9,updated_at=now() WHERE id=$1`, id, v.CourseID, v.Slug, v.Title, v.Description, v.OrderIndex, v.IsMandatory, v.IsEntry, v.EstimatedMinutes)
		if err != nil {
			return err
		}
		return savePrerequisites(ctx, tx, kind, id, v.ModulePrerequisites)
	case "lesson":
		if create {
			_, err := tx.Exec(ctx, `INSERT INTO lessons(id,module_id,slug,title,content,order_index,is_entry) VALUES($1,$2,$3,$4,$5::jsonb,$6,$7)`, id, v.ModuleID, v.Slug, v.Title, string(v.Content), v.OrderIndex, v.IsEntry)
			if err != nil {
				return err
			}
			return savePrerequisites(ctx, tx, kind, id, v.LessonPrerequisites)
		}
		_, err := tx.Exec(ctx, `UPDATE lessons SET module_id=$2,slug=$3,title=$4,content=$5::jsonb,order_index=$6,is_entry=$7,updated_at=now() WHERE id=$1`, id, v.ModuleID, v.Slug, v.Title, string(v.Content), v.OrderIndex, v.IsEntry)
		if err != nil {
			return err
		}
		return savePrerequisites(ctx, tx, kind, id, v.LessonPrerequisites)
	}
	return errors.New("jenis konten tidak valid")
}

// create godoc
// @Summary Buat konten admin
// @Description Memerlukan role Admin. Membuat resource yang belum diterbitkan. Kirim field sesuai kind: course memerlukan level_id dan slug; module memerlukan course_id dan slug; lesson memerlukan module_id dan slug. Title wajib untuk semua jenis. Question_set memerlukan kind (practice dengan lesson_id saja, atau final_exam dengan module_id saja) dan skill (reading/writing); questions disimpan sebagai draft dan boleh belum lengkap. Validasi soal lengkap dilakukan saat publish.
// @Tags Admin Content
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param kind path string true "Jenis resource" Enums(course,module,lesson,question_set)
// @Param request body ContentRequest true "Payload lengkap sesuai jenis resource"
// @Success 201 {object} CreateResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 409 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/content/{kind} [post]
func (h *Handler) create(c fiber.Ctx) error {
	kind := c.Params("kind")
	if _, ok := table(kind); !ok {
		return bad(c, "jenis konten tidak valid")
	}
	if kind == "question_set" {
		return h.createQuestionSet(c)
	}
	v, err := parseCore(kind, c.Body())
	if err != nil {
		return bad(c, err.Error())
	}
	id := uuid.New()
	tx, err := h.pool.Begin(c.Context())
	if err != nil {
		return serverError(c, err)
	}
	defer tx.Rollback(c.Context())
	if err := saveCore(c.Context(), tx, kind, id, v, true); err != nil {
		return serverError(c, err)
	}
	if err := tx.Commit(c.Context()); err != nil {
		return serverError(c, err)
	}
	return c.Status(201).JSON(fiber.Map{"success": true, "id": id})
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
// @Param request body ContentRequest true "Payload lengkap sesuai jenis resource"
// @Success 200 {object} UpdateResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 409 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/content/{kind}/{id} [patch]
func (h *Handler) update(c fiber.Ctx) error {
	kind := c.Params("kind")
	if _, ok := table(kind); !ok {
		return bad(c, "jenis konten tidak valid")
	}
	if kind == "question_set" {
		return h.updateQuestionSet(c)
	}
	id, err := resourceID(c)
	if err != nil {
		return bad(c, "ID tidak valid")
	}
	v, err := parseCore(kind, c.Body())
	if err != nil {
		return bad(c, err.Error())
	}
	name, _ := table(kind)
	tx, err := h.pool.Begin(c.Context())
	if err != nil {
		return serverError(c, err)
	}
	defer tx.Rollback(c.Context())
	var published bool
	if err := tx.QueryRow(c.Context(), fmt.Sprintf(`SELECT is_published FROM public.%s WHERE id=$1 FOR UPDATE`, name), id).Scan(&published); err != nil {
		return serverError(c, err)
	}
	if published {
		if err := validatePrerequisites(c.Context(), tx, kind, id, v.prerequisites(kind)); err != nil {
			return serverError(c, err)
		}
		payload, _ := json.Marshal(v)
		_, err = tx.Exec(c.Context(), `INSERT INTO admin_drafts(resource_type,resource_id,payload) VALUES($1,$2,$3::jsonb) ON CONFLICT(resource_type,resource_id) DO UPDATE SET payload=EXCLUDED.payload,updated_at=now()`, kind, id, string(payload))
	} else {
		err = saveCore(c.Context(), tx, kind, id, v, false)
	}
	if err != nil {
		return serverError(c, err)
	}
	if err := tx.Commit(c.Context()); err != nil {
		return serverError(c, err)
	}
	return c.JSON(fiber.Map{"success": true, "draft": published})
}

// publish godoc
// @Summary Terbitkan konten admin
// @Description Memerlukan role Admin. Menerapkan draft lalu menerbitkan resource; arsip harus dipulihkan terlebih dahulu. Question_set wajib memiliki soal valid: reading memakai multiple_choice dengan minimal dua pilihan dan tepat satu jawaban benar; writing memakai stroke_writing dengan stroke_template_id dan tanpa pilihan. Setiap soal memerlukan prompt, poin positif, dan stimulus atau template. Menerbitkan perubahan question_set yang sudah terbit membuat versi baru dalam lineage yang sama; respons id adalah UUID versi aktif dan dapat berbeda dari id permintaan. Konflik versi atau konfigurasi unik menghasilkan 409.
// @Tags Admin Content
// @Produce json
// @Security BearerAuth
// @Param kind path string true "Jenis resource" Enums(course,module,lesson,question_set)
// @Param id path string true "UUID resource" Format(uuid)
// @Success 200 {object} PublishResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 409 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/content/{kind}/{id}/publish [post]
func (h *Handler) publish(c fiber.Ctx) error {
	kind := c.Params("kind")
	if _, ok := table(kind); !ok {
		return bad(c, "jenis konten tidak valid")
	}
	if kind == "question_set" {
		return h.publishQuestionSet(c)
	}
	id, err := resourceID(c)
	if err != nil {
		return bad(c, "ID tidak valid")
	}
	name, _ := table(kind)
	tx, err := h.pool.Begin(c.Context())
	if err != nil {
		return serverError(c, err)
	}
	defer tx.Rollback(c.Context())
	var archived bool
	if err := tx.QueryRow(c.Context(), fmt.Sprintf(`SELECT archived_at IS NOT NULL FROM public.%s WHERE id=$1 FOR UPDATE`, name), id).Scan(&archived); err != nil {
		return serverError(c, err)
	}
	if archived {
		return bad(c, "pulihkan konten sebelum diterbitkan")
	}
	var raw []byte
	err = tx.QueryRow(c.Context(), `SELECT payload FROM admin_drafts WHERE resource_type=$1 AND resource_id=$2`, kind, id).Scan(&raw)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return serverError(c, err)
	}
	if err == nil {
		v, err := parseCore(kind, raw)
		if err != nil {
			return bad(c, err.Error())
		}
		if err := saveCore(c.Context(), tx, kind, id, v, false); err != nil {
			return serverError(c, err)
		}
	}
	if _, err := tx.Exec(c.Context(), fmt.Sprintf(`UPDATE public.%s SET is_published=true,updated_at=now() WHERE id=$1`, name), id); err != nil {
		return serverError(c, err)
	}
	if _, err := tx.Exec(c.Context(), `DELETE FROM admin_drafts WHERE resource_type=$1 AND resource_id=$2`, kind, id); err != nil {
		return serverError(c, err)
	}
	if err := tx.Commit(c.Context()); err != nil {
		return serverError(c, err)
	}
	return c.JSON(fiber.Map{"success": true})
}

// remove godoc
// @Summary Hapus atau arsipkan konten admin
// @Description Memerlukan role Admin. Menghapus resource jika tidak memiliki riwayat atau relasi yang menghalangi penghapusan; jika ada, mengarsipkan dan menonaktifkan publikasi. Draft dihapus. Respons archived menunjukkan apakah resource diarsipkan.
// @Tags Admin Content
// @Produce json
// @Security BearerAuth
// @Param kind path string true "Jenis resource" Enums(course,module,lesson,question_set)
// @Param id path string true "UUID resource" Format(uuid)
// @Success 200 {object} RemoveResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/content/{kind}/{id} [delete]
func (h *Handler) remove(c fiber.Ctx) error {
	kind := c.Params("kind")
	name, ok := table(kind)
	if !ok {
		return bad(c, "jenis konten tidak valid")
	}
	id, err := resourceID(c)
	if err != nil {
		return bad(c, "ID tidak valid")
	}
	tx, err := h.pool.Begin(c.Context())
	if err != nil {
		return serverError(c, err)
	}
	defer tx.Rollback(c.Context())
	var exists bool
	if err := tx.QueryRow(c.Context(), fmt.Sprintf(`SELECT true FROM public.%s WHERE id=$1 FOR UPDATE`, name), id).Scan(&exists); err != nil {
		return serverError(c, err)
	}
	historySQL := map[string]string{
		"course":       `SELECT EXISTS(SELECT 1 FROM enrollments WHERE course_id=$1) OR EXISTS(SELECT 1 FROM user_module_progress p JOIN modules m ON m.id=p.module_id WHERE m.course_id=$1) OR EXISTS(SELECT 1 FROM user_lesson_progress p JOIN lessons l ON l.id=p.lesson_id JOIN modules m ON m.id=l.module_id WHERE m.course_id=$1) OR EXISTS(SELECT 1 FROM attempts a JOIN question_sets s ON s.id=a.question_set_id LEFT JOIN lessons l ON l.id=s.lesson_id JOIN modules m ON m.id=COALESCE(s.module_id,l.module_id) WHERE m.course_id=$1)`,
		"module":       `SELECT EXISTS(SELECT 1 FROM user_module_progress WHERE module_id=$1) OR EXISTS(SELECT 1 FROM user_lesson_progress p JOIN lessons l ON l.id=p.lesson_id WHERE l.module_id=$1) OR EXISTS(SELECT 1 FROM attempts a JOIN question_sets s ON s.id=a.question_set_id LEFT JOIN lessons l ON l.id=s.lesson_id WHERE COALESCE(s.module_id,l.module_id)=$1)`,
		"lesson":       `SELECT EXISTS(SELECT 1 FROM user_lesson_progress WHERE lesson_id=$1) OR EXISTS(SELECT 1 FROM attempts a JOIN question_sets s ON s.id=a.question_set_id WHERE s.lesson_id=$1)`,
		"question_set": `SELECT EXISTS(SELECT 1 FROM attempts a JOIN question_sets attempted_set ON attempted_set.id=a.question_set_id JOIN question_sets target_set ON target_set.lineage_id=attempted_set.lineage_id WHERE target_set.id=$1)`,
	}
	var used bool
	if err := tx.QueryRow(c.Context(), historySQL[kind], id).Scan(&used); err != nil {
		return serverError(c, err)
	}
	if used {
		_, err = tx.Exec(c.Context(), fmt.Sprintf(`UPDATE public.%s SET is_published=false,archived_at=now() WHERE id=$1`, name), id)
	} else {
		if _, err = tx.Exec(c.Context(), `SAVEPOINT admin_delete`); err != nil {
			return serverError(c, err)
		}
		_, err = tx.Exec(c.Context(), fmt.Sprintf(`DELETE FROM public.%s WHERE id=$1`, name), id)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			if _, rollbackErr := tx.Exec(c.Context(), `ROLLBACK TO SAVEPOINT admin_delete`); rollbackErr != nil {
				return serverError(c, rollbackErr)
			}
			used = true
			_, err = tx.Exec(c.Context(), fmt.Sprintf(`UPDATE public.%s SET is_published=false,archived_at=now() WHERE id=$1`, name), id)
		}
	}
	if err != nil {
		return serverError(c, err)
	}
	if _, err := tx.Exec(c.Context(), `DELETE FROM admin_drafts WHERE resource_type=$1 AND resource_id=$2`, kind, id); err != nil {
		return serverError(c, err)
	}
	if err := tx.Commit(c.Context()); err != nil {
		return serverError(c, err)
	}
	return c.JSON(fiber.Map{"success": true, "archived": used})
}

// restore godoc
// @Summary Pulihkan konten yang diarsipkan
// @Description Memerlukan role Admin. Menghapus status arsip dan menjadikan resource belum diterbitkan. Resource yang tidak ditemukan atau tidak diarsipkan menghasilkan 404.
// @Tags Admin Content
// @Produce json
// @Security BearerAuth
// @Param kind path string true "Jenis resource" Enums(course,module,lesson,question_set)
// @Param id path string true "UUID resource" Format(uuid)
// @Success 200 {object} StatusResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/content/{kind}/{id}/restore [post]
func (h *Handler) restore(c fiber.Ctx) error {
	name, ok := table(c.Params("kind"))
	if !ok {
		return bad(c, "jenis konten tidak valid")
	}
	id, err := resourceID(c)
	if err != nil {
		return bad(c, "ID tidak valid")
	}
	result, err := h.pool.Exec(c.Context(), fmt.Sprintf(`UPDATE public.%s SET archived_at=NULL,is_published=false WHERE id=$1 AND archived_at IS NOT NULL`, name), id)
	if err != nil {
		return serverError(c, err)
	}
	if result.RowsAffected() == 0 {
		return c.Status(404).JSON(utils.ErrorRes{Success: false, Error: "arsip tidak ditemukan"})
	}
	return c.JSON(fiber.Map{"success": true})
}
