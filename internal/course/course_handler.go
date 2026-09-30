package course

import (
	"errors"
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
	courseGroup.Get("/:courseId", h.listLessons)
	courseGroup.Get("/:courseId/lessons/:lessonId", h.getLessonDetail)
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

	res, err := h.service.GetLessonDetail(c.Context(), courseID, lessonID)
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

	res, err := h.service.GetLessons(c.Context(), courseID)
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
