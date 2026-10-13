package course

import (
	"errors"
	"nihon-no-hikari-api/internal/course/model"
	"nihon-no-hikari-api/pkg/middleware"
	"nihon-no-hikari-api/pkg/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct {
	service    Service
	middleware middleware.Middleware
}

func NewHandler(service Service, middleware middleware.Middleware) *Handler {
	return &Handler{service: service, middleware: middleware}
}

func (h *Handler) RegisterRoutes(app *fiber.App) {
	courseGroup := app.Group("/api/v1/courses", h.middleware.AuthMiddleware())
	courseGroup.Get("", h.list)
	courseGroup.Get("/:courseId", h.getDetail)
	courseGroup.Put("/:courseId/modules/:moduleId/progress", h.upsertModuleProgress)
	lessonGroup := app.Group("/api/v1/lessons", h.middleware.AuthMiddleware())
	lessonGroup.Get("", h.listLessons)
	lessonGroup.Get("/:lessonId", h.getLessonDetail)
	lessonGroup.Put("/:lessonId/progress", h.upsertLessonProgress)
}

// upsertLessonProgress godoc
// @Summary Perbarui progres lesson
// @Description Membuat atau memperbarui progres lesson milik pengguna.
// @Tags Lessons
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param lessonId path string true "Lesson ID"
// @Param request body model.UpsertLessonProgressRequest true "Progres lesson"
// @Success 200 {object} model.LessonProgressResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 409 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /lessons/{lessonId}/progress [put]
func (h *Handler) upsertLessonProgress(c fiber.Ctx) error {
	lessonID, err := uuid.Parse(c.Params("lessonId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{Success: false, Error: "lessonId tidak valid"})
	}

	userID, ok := c.Locals(utils.UserIDKey).(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(utils.ErrorRes{Success: false, Error: "user tidak terautentikasi"})
	}

	var req model.UpsertLessonProgressRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
	}

	res, err := h.service.UpsertLessonProgress(c.Context(), lessonID, userID, req)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrInvalidLessonProgress):
			return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
		case errors.Is(err, utils.ErrLessonNotFound):
			return c.Status(fiber.StatusNotFound).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
		case errors.Is(err, utils.ErrLessonPrerequisiteNotCompleted):
			return c.Status(fiber.StatusConflict).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
		case errors.Is(err, utils.ErrLessonCompletionRequiresQuiz):
			return c.Status(fiber.StatusConflict).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
		}
	}

	return c.Status(fiber.StatusOK).JSON(res)
}

// upsertModuleProgress godoc
// @Summary Perbarui progres modul
// @Description Membuat atau memperbarui progres modul milik pengguna.
// @Tags Courses
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param courseId path string true "Course ID"
// @Param moduleId path string true "Module ID"
// @Param request body model.UpsertModuleProgressRequest true "Progres modul"
// @Success 200 {object} model.ModuleProgressResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 409 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /courses/{courseId}/modules/{moduleId}/progress [put]
func (h *Handler) upsertModuleProgress(c fiber.Ctx) error {
	courseID, err := uuid.Parse(c.Params("courseId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{Success: false, Error: "courseId tidak valid"})
	}

	moduleID, err := uuid.Parse(c.Params("moduleId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{Success: false, Error: "moduleId tidak valid"})
	}

	userID, ok := c.Locals(utils.UserIDKey).(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(utils.ErrorRes{Success: false, Error: "user tidak terautentikasi"})
	}

	var req model.UpsertModuleProgressRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
	}

	res, err := h.service.UpsertModuleProgress(c.Context(), courseID, moduleID, userID, req)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrInvalidModuleProgress):
			return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
		case errors.Is(err, utils.ErrModuleNotFound):
			return c.Status(fiber.StatusNotFound).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
		case errors.Is(err, utils.ErrModulePrerequisiteNotCompleted):
			return c.Status(fiber.StatusConflict).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
		case errors.Is(err, utils.ErrModuleCompletionRequiresExam):
			return c.Status(fiber.StatusConflict).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
		}
	}

	return c.Status(fiber.StatusOK).JSON(res)
}

// getDetail godoc
// @Summary Detail course
// @Description Mengambil detail course beserta daftar modul dan status progres.
// @Tags Courses
// @Produce json
// @Security BearerAuth
// @Param courseId path string true "Course ID"
// @Success 200 {object} model.CourseDetailResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /courses/{courseId} [get]
func (h *Handler) getDetail(c fiber.Ctx) error {
	courseID, err := uuid.Parse(c.Params("courseId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{
			Success: false,
			Error:   "courseId tidak valid",
		})
	}

	userID, ok := c.Locals(utils.UserIDKey).(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(utils.ErrorRes{
			Success: false,
			Error:   "user tidak terautentikasi",
		})
	}

	res, err := h.service.GetDetail(c.Context(), courseID, userID)
	if err != nil {
		if errors.Is(err, utils.ErrCourseNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(utils.ErrorRes{
				Success: false,
				Error:   err.Error(),
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(utils.ErrorRes{
			Success: false,
			Error:   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(res)
}

// getLessonDetail godoc
// @Summary Detail lesson
// @Description Mengambil konten lesson dan question set yang terkait.
// @Tags Lessons
// @Produce json
// @Security BearerAuth
// @Param lessonId path string true "Lesson ID"
// @Success 200 {object} model.LessonDetailResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /lessons/{lessonId} [get]
func (h *Handler) getLessonDetail(c fiber.Ctx) error {
	lessonID, err := uuid.Parse(c.Params("lessonId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{
			Success: false,
			Error:   "lessonId tidak valid",
		})
	}

	userID, ok := c.Locals(utils.UserIDKey).(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(utils.ErrorRes{
			Success: false,
			Error:   "user tidak terautentikasi",
		})
	}

	res, err := h.service.GetLessonDetail(c.Context(), lessonID, userID)
	if err != nil {
		if errors.Is(err, utils.ErrLessonNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(utils.ErrorRes{
				Success: false,
				Error:   err.Error(),
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(utils.ErrorRes{
			Success: false,
			Error:   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(res)
}

// listLessons godoc
// @Summary Daftar lesson course
// @Description Mengambil lesson yang dikelompokkan berdasarkan modul beserta status progres.
// @Tags Lessons
// @Produce json
// @Security BearerAuth
// @Param courseId query string true "Course ID"
// @Success 200 {object} model.ListLessonResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /lessons [get]
func (h *Handler) listLessons(c fiber.Ctx) error {
	courseID, err := uuid.Parse(c.Query("courseId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{
			Success: false,
			Error:   "courseId tidak valid",
		})
	}

	userID, ok := c.Locals(utils.UserIDKey).(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(utils.ErrorRes{
			Success: false,
			Error:   "user tidak terautentikasi",
		})
	}

	res, err := h.service.GetLessons(c.Context(), courseID, userID)
	if err != nil {
		if errors.Is(err, utils.ErrCourseNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(utils.ErrorRes{
				Success: false,
				Error:   err.Error(),
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(utils.ErrorRes{
			Success: false,
			Error:   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(res)
}

// list godoc
// @Summary Daftar course
// @Description Mengambil seluruh course yang tersedia.
// @Tags Courses
// @Produce json
// @Security BearerAuth
// @Success 200 {object} model.ListCourseResponse
// @Failure 401 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /courses [get]
func (h *Handler) list(c fiber.Ctx) error {
	res, err := h.service.GetList(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.ErrorRes{
			Success: false,
			Error:   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(res)
}
