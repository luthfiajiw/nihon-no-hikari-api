package middleware

import (
	"errors"
	"nihon-no-hikari-api/pkg/utils"
	"strings"
	"time"

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

		role, cached := m.cache.get(payload.ID, payload.UserID)
		if !cached {
			sessionSql := `
				SELECT s.is_blocked, u.role, s.expires_at
				FROM user_sessions s
				JOIN users u ON u.id = s.user_id
				WHERE s.id = $1 AND s.user_id = $2 AND s.expires_at > NOW()
			`
			var isBlocked bool
			var expiresAt time.Time
			err = m.store.QueryRow(c.Context(), sessionSql, payload.ID, payload.UserID).Scan(&isBlocked, &role, &expiresAt)
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
			m.cache.put(payload.ID, payload.UserID, role, expiresAt)
		}

		c.Locals(utils.UserIDKey, payload.UserID)
		c.Locals(utils.UserRoleKey, role)
		c.Locals(utils.SessionIDKey, payload.ID)

		return c.Next()
	}
}

func (m *middleware) RequireRole(role string) fiber.Handler {
	return func(c fiber.Ctx) error {
		if c.Locals(utils.UserRoleKey) != role {
			return c.Status(fiber.StatusForbidden).JSON(utils.ErrorRes{Success: false, Error: "akses tidak diizinkan"})
		}
		return c.Next()
	}
}
