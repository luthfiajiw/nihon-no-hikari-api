package user

import (
	"errors"
	"nihon-no-hikari-api/pkg/middleware"
	"nihon-no-hikari-api/pkg/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	pool       *pgxpool.Pool
	middleware middleware.Middleware
}

type Summary struct {
	ID          uuid.UUID `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	Role        Role      `json:"role"`
}

func NewHandler(pool *pgxpool.Pool, authMiddleware middleware.Middleware) *Handler {
	return &Handler{pool: pool, middleware: authMiddleware}
}

func (h *Handler) RegisterRoutes(app *fiber.App) {
	group := app.Group("/api/v1/admin/users", h.middleware.AuthMiddleware(), h.middleware.RequireRole(string(RoleAdmin)))
	group.Get("", h.list)
	group.Patch("/:userId/role", h.changeRole)
}

func (h *Handler) list(c fiber.Ctx) error {
	rows, err := h.pool.Query(c.Context(), `SELECT id, email, display_name, role FROM users ORDER BY created_at, id`)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.ErrorRes{Success: false, Error: "gagal mengambil daftar pengguna"})
	}
	defer rows.Close()

	users := make([]Summary, 0)
	for rows.Next() {
		var item Summary
		if err := rows.Scan(&item.ID, &item.Email, &item.DisplayName, &item.Role); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(utils.ErrorRes{Success: false, Error: "gagal membaca pengguna"})
		}
		users = append(users, item)
	}
	if rows.Err() != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.ErrorRes{Success: false, Error: "gagal membaca pengguna"})
	}
	return c.JSON(fiber.Map{"success": true, "data": users})
}

func (h *Handler) changeRole(c fiber.Ctx) error {
	userID, err := uuid.Parse(c.Params("userId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{Success: false, Error: "ID pengguna tidak valid"})
	}
	var request struct {
		Role Role `json:"role"`
	}
	if err := c.Bind().Body(&request); err != nil || (request.Role != RoleUser && request.Role != RoleAdmin) {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{Success: false, Error: "role harus User atau Admin"})
	}
	if userID == c.Locals(utils.UserIDKey) {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{Success: false, Error: "role akun sendiri tidak dapat diubah"})
	}

	var user Summary
	err = h.pool.QueryRow(c.Context(), `UPDATE users SET role = $1, updated_at = NOW() WHERE id = $2 RETURNING id, email, display_name, role`, request.Role, userID).
		Scan(&user.ID, &user.Email, &user.DisplayName, &user.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return c.Status(fiber.StatusNotFound).JSON(utils.ErrorRes{Success: false, Error: "pengguna tidak ditemukan"})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.ErrorRes{Success: false, Error: "gagal mengubah role"})
	}
	return c.JSON(fiber.Map{"success": true, "data": user})
}
