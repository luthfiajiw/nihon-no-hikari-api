package auth

import (
	"errors"
	"nihon-no-hikari-api/pkg/utils"

	"github.com/gofiber/fiber/v3"
)

type Handler struct {
	service   Service
	validator utils.BodyValidator
}

func NewHandler(s Service, v utils.BodyValidator) *Handler {
	return &Handler{
		service:   s,
		validator: v,
	}
}

func (h *Handler) RegisterRoutes(app *fiber.App) {
	authGroup := app.Group("/api/v1/auth")
	authGroup.Post("/register", h.register)
	authGroup.Post("/signin", h.signIn)
	authGroup.Post("/token", h.token)
	authGroup.Post("/signout", h.signout)
}

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
