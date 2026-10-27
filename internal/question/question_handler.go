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
	admin := app.Group("/api/v1/admin", h.middleware.AuthMiddleware(), h.middleware.RequireRole("Admin"))
	admin.Get("/question-sets", h.adminList)
	admin.Post("/question-sets", h.adminCreate)
	admin.Get("/question-sets/:id", h.adminDetail)
	admin.Patch("/question-sets/:id", h.adminUpdate)
	admin.Delete("/question-sets/:id", h.adminRemove)
	admin.Post("/question-sets/:id/publish", h.adminPublish)
	admin.Post("/question-sets/:id/restore", h.adminRestore)
	admin.Post("/templates/preview", h.previewTemplate)
	admin.Get("/templates", h.templates)
	admin.Post("/templates", h.createTemplate)
	admin.Get("/templates/:id", h.templateDetail)
	admin.Patch("/templates/:id", h.updateTemplate)

	group := app.Group("/api/v1/question-sets", h.middleware.AuthMiddleware())
	group.Get("", h.listQuestionSets)
	group.Get("/:questionSetId", h.getQuestionSetDetail)
	group.Post("/:questionSetId/attempts", h.startAttempt)
	group.Post("/:questionSetId/attempts/:attemptId/abandon", h.abandonAttempt)
	group.Post("/:questionSetId/attempts/:attemptId/submit", h.submitAttempt)
	app.Get("/api/v1/modules/:moduleId/question-sets", h.middleware.AuthMiddleware(), h.listModuleExams)
}

func (h *Handler) listModuleExams(c fiber.Ctx) error {
	moduleID, err := uuid.Parse(c.Params("moduleId"))
	if err != nil {
		return errorJSON(c, fiber.StatusBadRequest, errors.New("moduleId tidak valid"))
	}
	userID, ok := c.Locals(utils.UserIDKey).(uuid.UUID)
	if !ok {
		return errorJSON(c, fiber.StatusUnauthorized, errors.New("user tidak terautentikasi"))
	}
	res, err := h.service.ListModuleExams(c.Context(), moduleID, userID)
	if err != nil {
		return errorJSON(c, fiber.StatusInternalServerError, err)
	}
	return c.JSON(res)
}

// getQuestionSetDetail godoc
// @Summary Detail question set
// @Description Mengambil detail question set beserta pertanyaannya.
// @Tags Questions
// @Produce json
// @Security BearerAuth
// @Param questionSetId path string true "Question set ID"
// @Success 200 {object} QuestionSetDetailResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /question-sets/{questionSetId} [get]
func (h *Handler) getQuestionSetDetail(c fiber.Ctx) error {
	questionSetID, err := uuid.Parse(c.Params("questionSetId"))
	if err != nil {
		return errorJSON(c, fiber.StatusBadRequest, errors.New("questionSetId tidak valid"))
	}
	userID, ok := c.Locals(utils.UserIDKey).(uuid.UUID)
	if !ok {
		return errorJSON(c, fiber.StatusUnauthorized, errors.New("user tidak terautentikasi"))
	}

	res, err := h.service.GetQuestionSetDetail(c.Context(), questionSetID, userID)
	if err != nil {
		if errors.Is(err, utils.ErrQuestionSetNotFound) {
			return errorJSON(c, fiber.StatusNotFound, err)
		}
		return errorJSON(c, fiber.StatusInternalServerError, err)
	}
	return c.Status(fiber.StatusOK).JSON(res)
}

// listQuestionSets godoc
// @Summary Daftar question set
// @Description Mengambil seluruh question set untuk sebuah lesson.
// @Tags Questions
// @Produce json
// @Security BearerAuth
// @Param lessonId query string true "Lesson ID"
// @Success 200 {object} ListQuestionSetsResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /question-sets [get]
func (h *Handler) listQuestionSets(c fiber.Ctx) error {
	lessonID, err := uuid.Parse(c.Query("lessonId"))
	if err != nil {
		return errorJSON(c, fiber.StatusBadRequest, errors.New("lessonId tidak valid"))
	}
	userID, ok := c.Locals(utils.UserIDKey).(uuid.UUID)
	if !ok {
		return errorJSON(c, fiber.StatusUnauthorized, errors.New("user tidak terautentikasi"))
	}
	res, err := h.service.ListQuestionSets(c.Context(), lessonID, userID)
	if err != nil {
		if errors.Is(err, utils.ErrLessonNotFound) {
			return errorJSON(c, fiber.StatusNotFound, err)
		}
		return errorJSON(c, fiber.StatusInternalServerError, err)
	}
	return c.Status(fiber.StatusOK).JSON(res)
}

// startAttempt godoc
// @Summary Mulai attempt
// @Description Membuat attempt baru untuk sebuah question set.
// @Tags Questions
// @Produce json
// @Security BearerAuth
// @Param questionSetId path string true "Question set ID"
// @Success 201 {object} StartAttemptResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 409 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /question-sets/{questionSetId}/attempts [post]
func (h *Handler) startAttempt(c fiber.Ctx) error {
	questionSetID, err := uuid.Parse(c.Params("questionSetId"))
	if err != nil {
		return errorJSON(c, fiber.StatusBadRequest, errors.New("questionSetId tidak valid"))
	}
	userID, ok := c.Locals(utils.UserIDKey).(uuid.UUID)
	if !ok {
		return errorJSON(c, fiber.StatusUnauthorized, errors.New("user tidak terautentikasi"))
	}

	res, err := h.service.StartAttempt(c.Context(), questionSetID, userID)
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

// submitAttempt godoc
// @Summary Kirim jawaban attempt
// @Description Mengirim jawaban, menilai attempt, dan mengembalikan hasilnya.
// @Tags Questions
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param questionSetId path string true "Question set ID"
// @Param attemptId path string true "Attempt ID"
// @Param request body SubmitAttemptRequest true "Daftar jawaban"
// @Success 200 {object} SubmitAttemptResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 409 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /question-sets/{questionSetId}/attempts/{attemptId}/submit [post]
func (h *Handler) submitAttempt(c fiber.Ctx) error {
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
	res, err := h.service.SubmitAttempt(c.Context(), questionSetID, attemptID, userID, req)
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

// abandonAttempt godoc
// @Summary Batalkan attempt
// @Description Menandai attempt aktif sebagai dibatalkan.
// @Tags Questions
// @Produce json
// @Security BearerAuth
// @Param questionSetId path string true "Question set ID"
// @Param attemptId path string true "Attempt ID"
// @Success 200 {object} AbandonAttemptResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 409 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /question-sets/{questionSetId}/attempts/{attemptId}/abandon [post]
func (h *Handler) abandonAttempt(c fiber.Ctx) error {
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

	res, err := h.service.AbandonAttempt(c.Context(), questionSetID, attemptID, userID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrAttemptNotFound):
			return errorJSON(c, fiber.StatusNotFound, err)
		case errors.Is(err, utils.ErrAttemptAlreadySubmitted):
			return errorJSON(c, fiber.StatusConflict, err)
		default:
			return errorJSON(c, fiber.StatusInternalServerError, err)
		}
	}
	return c.Status(fiber.StatusOK).JSON(res)
}

func errorJSON(c fiber.Ctx, status int, err error) error {
	return c.Status(status).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
}
