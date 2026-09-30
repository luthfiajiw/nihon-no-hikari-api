package middleware

import (
	"errors"
	"nihon-no-hikari-api/pkg/utils"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
)

func (m *middleware) AuthMiddleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		authorization := c.Get("Authorization")
		if authorization == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"success": false,
				"error":   "authorization header is required",
			})
		}

		parts := strings.Split(authorization, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"success": false,
				"error":   "invalid authorization header format",
			})
		}

		token := parts[1]
		payload, err := utils.VerifyToken(token, m.cfg.JWTSecretKey)
		if err != nil {
			if errors.Is(err, jwt.ErrTokenExpired) {
				return c.Status(fiber.StatusUnauthorized).JSON(utils.ErrorRes{
					Success: false,
					Error:   "akses token kadaluarsa",
				})
			}
			return c.Status(fiber.StatusUnauthorized).JSON(utils.ErrorRes{
				Success: false,
				Error:   err.Error(),
			})
		}

		sessionSql := `
			SELECT is_blocked
			FROM user_sessions
			WHERE id = $1 AND user_id = $2
		`
		var isBlocked bool
		err = m.pool.QueryRow(c.Context(), sessionSql, payload.ID, payload.UserID).Scan(&isBlocked)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"success": false,
				"error":   "sesi tidak ditemukan",
			})
		}
		if isBlocked {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"success": false,
				"error":   "sesi sudah ditutup",
			})
		}

		c.Locals(utils.UserIDKey, payload.UserID)

		return c.Next()
	}
}
