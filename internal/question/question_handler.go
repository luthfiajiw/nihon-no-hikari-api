package question

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
	group := app.Group("/api/v1/courses", h.middleware.AuthMiddleware())
	group.Get("/:courseId/lessons/:lessonId/question-sets", h.listQuestionSets)
	group.Get("/:courseId/lessons/:lessonId/question-sets/:questionSetId", h.getQuestionSetDetail)
	group.Post("/:courseId/lessons/:lessonId/question-sets/:questionSetId/attempts", h.startAttempt)
	group.Post("/:courseId/lessons/:lessonId/question-sets/:questionSetId/attempts/:attemptId/submit", h.submitAttempt)
}

func (h *Handler) getQuestionSetDetail(c fiber.Ctx) error {
	courseID, lessonID, err := parseCourseAndLessonIDs(c)
	if err != nil {
		return errorJSON(c, fiber.StatusBadRequest, err)
	}
	questionSetID, err := uuid.Parse(c.Params("questionSetId"))
	if err != nil {
		return errorJSON(c, fiber.StatusBadRequest, errors.New("questionSetId tidak valid"))
	}
	userID, ok := c.Locals(utils.UserIDKey).(uuid.UUID)
	if !ok {
		return errorJSON(c, fiber.StatusUnauthorized, errors.New("user tidak terautentikasi"))
	}

	res, err := h.service.GetQuestionSetDetail(c.Context(), courseID, lessonID, questionSetID, userID)
	if err != nil {
		if errors.Is(err, utils.ErrQuestionSetNotFound) {
			return errorJSON(c, fiber.StatusNotFound, err)
		}
		return errorJSON(c, fiber.StatusInternalServerError, err)
	}
	return c.Status(fiber.StatusOK).JSON(res)
}

func (h *Handler) listQuestionSets(c fiber.Ctx) error {
	courseID, lessonID, err := parseCourseAndLessonIDs(c)
	if err != nil {
		return errorJSON(c, fiber.StatusBadRequest, err)
	}
	userID, ok := c.Locals(utils.UserIDKey).(uuid.UUID)
	if !ok {
		return errorJSON(c, fiber.StatusUnauthorized, errors.New("user tidak terautentikasi"))
	}
	res, err := h.service.ListQuestionSets(c.Context(), courseID, lessonID, userID)
	if err != nil {
		if errors.Is(err, utils.ErrLessonNotFound) {
			return errorJSON(c, fiber.StatusNotFound, err)
		}
		return errorJSON(c, fiber.StatusInternalServerError, err)
	}
	return c.Status(fiber.StatusOK).JSON(res)
}

func (h *Handler) startAttempt(c fiber.Ctx) error {
	courseID, lessonID, err := parseCourseAndLessonIDs(c)
	if err != nil {
		return errorJSON(c, fiber.StatusBadRequest, err)
	}
	questionSetID, err := uuid.Parse(c.Params("questionSetId"))
	if err != nil {
		return errorJSON(c, fiber.StatusBadRequest, errors.New("questionSetId tidak valid"))
	}
	userID, ok := c.Locals(utils.UserIDKey).(uuid.UUID)
	if !ok {
		return errorJSON(c, fiber.StatusUnauthorized, errors.New("user tidak terautentikasi"))
	}

	res, err := h.service.StartAttempt(c.Context(), courseID, lessonID, questionSetID, userID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrQuestionSetNotFound):
			return errorJSON(c, fiber.StatusNotFound, err)
		case errors.Is(err, utils.ErrLessonPrerequisiteNotCompleted),
			errors.Is(err, utils.ErrQuestionSetEmpty),
			errors.Is(err, utils.ErrAttemptLimitReached),
			errors.Is(err, utils.ErrAttemptCooldown):
			return errorJSON(c, fiber.StatusConflict, err)
		default:
			return errorJSON(c, fiber.StatusInternalServerError, err)
		}
	}
	return c.Status(fiber.StatusCreated).JSON(res)
}

func (h *Handler) submitAttempt(c fiber.Ctx) error {
	courseID, lessonID, err := parseCourseAndLessonIDs(c)
	if err != nil {
		return errorJSON(c, fiber.StatusBadRequest, err)
	}
	questionSetID, err := uuid.Parse(c.Params("questionSetId"))
	if err != nil {
		return errorJSON(c, fiber.StatusBadRequest, errors.New("questionSetId tidak valid"))
	}
	attemptID, err := uuid.Parse(c.Params("attemptId"))
	if err != nil {
		return errorJSON(c, fiber.StatusBadRequest, errors.New("attemptId tidak valid"))
	}
	userID, ok := c.Locals(utils.UserIDKey).(uuid.UUID)
	if !ok {
		return errorJSON(c, fiber.StatusUnauthorized, errors.New("user tidak terautentikasi"))
	}

	var req SubmitAttemptRequest
	if err := c.Bind().Body(&req); err != nil {
		return errorJSON(c, fiber.StatusBadRequest, err)
	}
	res, err := h.service.SubmitAttempt(c.Context(), courseID, lessonID, questionSetID, attemptID, userID, req)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrInvalidAttempt):
			return errorJSON(c, fiber.StatusBadRequest, err)
		case errors.Is(err, utils.ErrAttemptNotFound):
			return errorJSON(c, fiber.StatusNotFound, err)
		case errors.Is(err, utils.ErrAttemptAlreadySubmitted), errors.Is(err, utils.ErrAttemptExpired):
			return errorJSON(c, fiber.StatusConflict, err)
		default:
			return errorJSON(c, fiber.StatusInternalServerError, err)
		}
	}
	return c.Status(fiber.StatusOK).JSON(res)
}

func parseCourseAndLessonIDs(c fiber.Ctx) (uuid.UUID, uuid.UUID, error) {
	courseID, err := uuid.Parse(c.Params("courseId"))
	if err != nil {
		return uuid.Nil, uuid.Nil, errors.New("courseId tidak valid")
	}
	lessonID, err := uuid.Parse(c.Params("lessonId"))
	if err != nil {
		return uuid.Nil, uuid.Nil, errors.New("lessonId tidak valid")
	}
	return courseID, lessonID, nil
}

func errorJSON(c fiber.Ctx, status int, err error) error {
	return c.Status(status).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
}
