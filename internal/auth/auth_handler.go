package auth

import (
	"errors"

	"nihon-no-hikari-api/pkg/middleware"
	"nihon-no-hikari-api/pkg/utils"

	"github.com/gofiber/fiber/v3"
)

type Handler struct {
	service    Service
	validator  utils.BodyValidator
	middleware middleware.Middleware
}

func NewHandler(s Service, v utils.BodyValidator, m middleware.Middleware) *Handler {
	return &Handler{
		service:    s,
		validator:  v,
		middleware: m,
	}
}

func (h *Handler) RegisterRoutes(app *fiber.App) {
	app.Get("/api/v1/admin/me", h.middleware.AuthMiddleware(), h.middleware.RequireRole("Admin"), h.me)
	authGroup := app.Group("/api/v1/auth")
	authGroup.Post("/register", h.register)
	authGroup.Post("/signin", h.signIn)
	authGroup.Post("/token", h.token)
	authGroup.Post("/signout", h.signout)
}

// register godoc
// @Summary Registrasi akun
// @Description Membuat akun pengguna baru.
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body RegisterRequest true "Data registrasi"
// @Success 201 {object} utils.SuccessRes
// @Failure 400 {object} utils.ErrorRes
// @Router /auth/register [post]
func (h *Handler) register(c fiber.Ctx) error {
	var req RegisterRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{
			Success: false,
			Error:   err.Error(),
		})
	}

	if req.Password != req.ConfirmPassword {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{
			Success: false,
			Error:   utils.ErrPasswordNotMatch.Error(),
		})
	}

	if err := utils.ValidatePassword(req.Password); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{
			Success: false,
			Error:   err.Error(),
		})
	}

	if !utils.ValidateEmail(req.Email) {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{
			Success: false,
			Error:   utils.ErrInvalidEmail.Error(),
		})
	}

	err := h.service.Register(c.Context(), req)
	if err != nil {
		message := err.Error()
		if errors.Is(err, utils.ErrEmailAlreadyTaken) {
			message = err.Error()
		}
		return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{
			Success: false,
			Error:   message,
		})
	}

	return c.Status(fiber.StatusCreated).JSON(utils.SuccessRes{
		Success: true,
		Message: "akun baru berhasil dibuat",
	})
}

// signIn godoc
// @Summary Masuk
// @Description Mengautentikasi pengguna dan membuat sesi baru.
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body SignInRequest true "Kredensial pengguna"
// @Success 200 {object} SignInResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /auth/signin [post]
func (h *Handler) signIn(c fiber.Ctx) error {
	var req SignInRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{
			Success: false,
			Error:   err.Error(),
		})
	}

	res, err := h.service.SignIn(c.Context(), req, c.Get("User-Agent"), c.IP())
	if err != nil {
		if errors.Is(err, utils.ErrUserNotFound) || errors.Is(err, utils.ErrUserNotRegistered) {
			return c.Status(fiber.StatusNotFound).JSON(utils.ErrorRes{
				Success: false,
				Error:   utils.ErrUserNotRegistered.Error(),
			})
		} else if errors.Is(err, utils.ErrInvalidCredentials) {
			return c.Status(fiber.StatusUnauthorized).JSON(utils.ErrorRes{
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

// token godoc
// @Summary Perbarui access token
// @Description Membuat access token baru menggunakan refresh token.
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body TokenRequest true "Refresh token"
// @Success 200 {object} RefreshTokenResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /auth/token [post]
func (h *Handler) token(c fiber.Ctx) error {
	var req TokenRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{
			Success: false,
			Error:   err.Error(),
		})
	}

	if validationErr := h.validator.Validate(req); validationErr != nil {
		return c.Status(fiber.StatusBadRequest).JSON(validationErr)
	}

	if req.GrantType != "refresh" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{
			Success: false,
			Error:   utils.ErrUnsupportedGrantType.Error(),
		})
	}

	res, err := h.service.RefreshAccessToken(c.Context(), req)
	if err != nil {
		if errors.Is(err, utils.ErrInvalidRefreshToken) ||
			errors.Is(err, utils.ErrSessionBlocked) ||
			errors.Is(err, utils.ErrSessionExpired) {
			return c.Status(fiber.StatusUnauthorized).JSON(utils.ErrorRes{
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

// signout godoc
// @Summary Keluar
// @Description Menutup sesi pengguna berdasarkan session ID.
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body LogoutRequest true "Sesi yang akan ditutup"
// @Success 200 {object} utils.SuccessRes
// @Failure 400 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /auth/signout [post]
func (h *Handler) signout(c fiber.Ctx) error {
	var req LogoutRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{
			Success: false,
			Error:   err.Error(),
		})
	}

	if validationErr := h.validator.Validate(req); validationErr != nil {
		return c.Status(fiber.StatusBadRequest).JSON(validationErr)
	}

	if err := h.service.Logout(c.Context(), req); err != nil {
		if errors.Is(err, utils.ErrSessionNotFound) {
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

	return c.Status(fiber.StatusOK).JSON(utils.SuccessRes{
		Success: true,
		Message: "berhasil keluar",
	})
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
	return c.JSON(MeResponse{Success: true, Role: "Admin"})
}
