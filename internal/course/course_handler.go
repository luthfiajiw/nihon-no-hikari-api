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
	courseGroup.Get("/:courseId/modules", h.listLessons)
	courseGroup.Put("/:courseId/modules/:moduleId/progress", h.upsertModuleProgress)
	courseGroup.Get("/:courseId/lessons/:lessonId", h.getLessonDetail)
	courseGroup.Put("/:courseId/lessons/:lessonId/progress", h.upsertLessonProgress)
}

func (h *Handler) upsertLessonProgress(c fiber.Ctx) error {
	courseID, err := uuid.Parse(c.Params("courseId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{Success: false, Error: "courseId tidak valid"})
	}

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

	res, err := h.service.UpsertLessonProgress(c.Context(), courseID, lessonID, userID, req)
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
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
		}
	}

	return c.Status(fiber.StatusOK).JSON(res)
}

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

func (h *Handler) getLessonDetail(c fiber.Ctx) error {
	courseID, err := uuid.Parse(c.Params("courseId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{
			Success: false,
			Error:   "courseId tidak valid",
		})
	}

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

	res, err := h.service.GetLessonDetail(c.Context(), courseID, lessonID, userID)
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

func (h *Handler) listLessons(c fiber.Ctx) error {
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
