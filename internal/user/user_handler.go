package user

import (
	"errors"
	"fmt"
	"strings"

	"nihon-no-hikari-api/pkg/middleware"
	"nihon-no-hikari-api/pkg/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	pool       *pgxpool.Pool
	service    Service
	middleware middleware.Middleware
}

type Summary struct {
	ID          uuid.UUID `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	Role        Role      `json:"role"`
}

func NewHandler(pool *pgxpool.Pool, service Service, authMiddleware middleware.Middleware) *Handler {
	return &Handler{
		pool:       pool,
		service:    service,
		middleware: authMiddleware,
	}
}

func (h *Handler) RegisterRoutes(app *fiber.App) {
	// Regular user profile endpoints
	userGroup := app.Group("/api/v1/user", h.middleware.AuthMiddleware())
	userGroup.Get("/profile", h.getProfile)
	userGroup.Patch("/profile", h.updateProfile)
	userGroup.Post("/password", h.changePassword)
	userGroup.Post("/link-google/request-code", h.requestLinkGoogleCode)
	userGroup.Post("/link-google/verify-code", h.verifyLinkGoogleCode)
	userGroup.Post("/link-google", h.linkGoogle)
	userGroup.Post("/unlink-google", h.unlinkGoogle)
	userGroup.Delete("/sessions/other", h.revokeOtherSessions)

	// Admin user endpoints
	adminGroup := app.Group("/api/v1/admin/users", h.middleware.AuthMiddleware(), h.middleware.RequireRole(string(RoleAdmin)))
	adminGroup.Get("", h.list)
	adminGroup.Patch("/:userId/role", h.changeRole)
}

func getUserID(c fiber.Ctx) (uuid.UUID, error) {
	val := c.Locals(utils.UserIDKey)
	id, ok := val.(uuid.UUID)
	if !ok || id == uuid.Nil {
		return uuid.Nil, errors.New("pengguna tidak terautentikasi")
	}
	return id, nil
}

func getSessionID(c fiber.Ctx) uuid.UUID {
	val := c.Locals(utils.SessionIDKey)
	id, ok := val.(uuid.UUID)
	if !ok {
		return uuid.Nil
	}
	return id
}

// getProfile godoc
// @Summary Profil pengguna saat ini
// @Description Mengambil data profil diri, status kata sandi, akun Google tertaut, dan daftar sesi aktif.
// @Tags User Profile
// @Produce json
// @Security BearerAuth
// @Success 200 {object} ProfileResponse
// @Failure 401 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /user/profile [get]
func (h *Handler) getProfile(c fiber.Ctx) error {
	userID, err := getUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
	}

	sessionID := getSessionID(c)
	data, err := h.service.GetProfile(c.Context(), userID, sessionID)
	if err != nil {
		if errors.Is(err, utils.ErrUserNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(utils.ErrorRes{Success: false, Error: "gagal mengambil data profil"})
	}

	return c.JSON(fiber.Map{"success": true, "data": data})
}

// updateProfile godoc
// @Summary Perbarui profil diri
// @Description Mengubah nama tampilan dan/atau avatar profil.
// @Tags User Profile
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body UpdateProfileRequest true "Data pembaruan profil"
// @Success 200 {object} UpdateProfileResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /user/profile [patch]
func (h *Handler) updateProfile(c fiber.Ctx) error {
	userID, err := getUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
	}

	var req UpdateProfileRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{Success: false, Error: "format data tidak valid"})
	}

	updated, err := h.service.UpdateProfile(c.Context(), userID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "data": updated})
}

// changePassword godoc
// @Summary Ubah atau atur kata sandi
// @Description Memperbarui kata sandi pengguna atau membuat kata sandi pertama kali bagi pengguna OAuth.
// @Tags User Profile
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body ChangePasswordRequest true "Data perubahan kata sandi"
// @Success 200 {object} utils.SuccessRes
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /user/password [post]
func (h *Handler) changePassword(c fiber.Ctx) error {
	userID, err := getUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
	}

	var req ChangePasswordRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{Success: false, Error: "format data tidak valid"})
	}

	if err := h.service.ChangePassword(c.Context(), userID, req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
	}

	return c.JSON(utils.SuccessRes{Success: true, Message: "kata sandi berhasil diperbarui"})
}

// requestLinkGoogleCode godoc
// @Summary Minta kode verifikasi tautan Google
// @Description Mengirimkan kode 6-digit OTP ke email akun untuk verifikasi tautan akun Google.
// @Tags User Profile
// @Produce json
// @Security BearerAuth
// @Success 200 {object} utils.SuccessRes
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 429 {object} utils.ErrorRes
// @Failure 503 {object} utils.ErrorRes
// @Router /user/link-google/request-code [post]
func (h *Handler) requestLinkGoogleCode(c fiber.Ctx) error {
	userID, err := getUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
	}

	if err := h.service.RequestLinkGoogleCode(c.Context(), userID); err != nil {
		switch {
		case errors.Is(err, utils.ErrVerificationRateLimited):
			return c.Status(fiber.StatusTooManyRequests).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
		case errors.Is(err, utils.ErrMailerDisabled):
			return c.Status(fiber.StatusServiceUnavailable).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
		case errors.Is(err, utils.ErrGoogleAlreadyLinked):
			return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
		}
	}

	return c.JSON(utils.SuccessRes{Success: true, Message: "kode verifikasi berhasil dikirim ke email Anda"})
}

// verifyLinkGoogleCode godoc
// @Summary Verifikasi kode OTP tautan Google
// @Description Memverifikasi kode 6-digit OTP dan menghasilkan verification_token untuk penautan Google.
// @Tags User Profile
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body VerifyLinkGoogleCodeRequest true "Kode OTP 6-digit"
// @Success 200 {object} VerifyLinkGoogleCodeResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 429 {object} utils.ErrorRes
// @Router /user/link-google/verify-code [post]
func (h *Handler) verifyLinkGoogleCode(c fiber.Ctx) error {
	userID, err := getUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
	}

	var req VerifyLinkGoogleCodeRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{Success: false, Error: "format data tidak valid"})
	}

	token, err := h.service.VerifyLinkGoogleCode(c.Context(), userID, req)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrVerificationAttemptsExceeded):
			return c.Status(fiber.StatusTooManyRequests).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
		case errors.Is(err, utils.ErrInvalidVerificationCode):
			return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
		default:
			return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
		}
	}

	return c.JSON(VerifyLinkGoogleCodeResponse{
		Success: true,
		Data: VerifyLinkGoogleCodeData{
			VerificationToken: token,
		},
	})
}

// linkGoogle godoc
// @Summary Hubungkan akun Google
// @Description Menautkan akun Google ke akun pengguna saat ini menggunakan Google ID token dan token verifikasi email.
// @Tags User Profile
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body LinkGoogleRequest true "Google ID Token dan Verification Token"
// @Success 200 {object} utils.SuccessRes
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /user/link-google [post]
func (h *Handler) linkGoogle(c fiber.Ctx) error {
	userID, err := getUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
	}

	var req LinkGoogleRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{Success: false, Error: "format data tidak valid"})
	}

	if err := h.service.LinkGoogle(c.Context(), userID, req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
	}

	return c.JSON(utils.SuccessRes{Success: true, Message: "akun Google berhasil ditautkan"})
}

// unlinkGoogle godoc
// @Summary Putuskan tautan akun Google
// @Description Memutuskan tautan akun Google dari akun pengguna saat ini menggunakan kata sandi saat ini.
// @Tags User Profile
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body UnlinkGoogleRequest true "Kata sandi saat ini"
// @Success 200 {object} utils.SuccessRes
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /user/unlink-google [post]
func (h *Handler) unlinkGoogle(c fiber.Ctx) error {
	userID, err := getUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
	}

	var req UnlinkGoogleRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{Success: false, Error: "format data tidak valid"})
	}

	if err := h.service.UnlinkGoogle(c.Context(), userID, req); err != nil {
		switch {
		case errors.Is(err, utils.ErrUserNotFound):
			return c.Status(fiber.StatusNotFound).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
		case errors.Is(err, utils.ErrCannotUnlinkWithoutPassword),
			errors.Is(err, utils.ErrCurrentPasswordRequired),
			errors.Is(err, utils.ErrCurrentPasswordIncorrect),
			errors.Is(err, utils.ErrGoogleNotLinked):
			return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(utils.ErrorRes{Success: false, Error: "gagal memutuskan tautan akun Google"})
		}
	}

	return c.JSON(utils.SuccessRes{Success: true, Message: "tautan akun Google berhasil diputuskan"})
}

// revokeOtherSessions godoc
// @Summary Cabut seluruh sesi lain
// @Description Menutup seluruh sesi masuk aktif pada perangkat lain milik pengguna selain sesi saat ini.
// @Tags User Profile
// @Produce json
// @Security BearerAuth
// @Success 200 {object} utils.SuccessRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /user/sessions/other [delete]
func (h *Handler) revokeOtherSessions(c fiber.Ctx) error {
	userID, err := getUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(utils.ErrorRes{Success: false, Error: err.Error()})
	}

	sessionID := getSessionID(c)
	if err := h.service.RevokeOtherSessions(c.Context(), userID, sessionID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.ErrorRes{Success: false, Error: "gagal mencabut sesi lain"})
	}
	h.middleware.InvalidateUser(userID)

	return c.JSON(utils.SuccessRes{Success: true, Message: "seluruh sesi lain berhasil dicabut"})
}

// list godoc
// @Summary Daftar pengguna untuk admin
// @Description Memerlukan role Admin. Mengambil ID, email, nama tampilan, dan role seluruh pengguna berdasarkan created_at dan id.
// @Tags Admin Users
// @Produce json
// @Security BearerAuth
// @Success 200 {object} ListUsersResponse
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/users [get]
func (h *Handler) list(c fiber.Ctx) error {
	p := utils.ParsePaginationParams(c)
	roleFilter := strings.TrimSpace(c.Query("role"))

	var whereClauses []string
	var args []any
	argIndex := 1

	if p.Search != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("(strpos(lower(display_name || ' ' || email), lower($%d)) > 0)", argIndex))
		args = append(args, p.Search)
		argIndex++
	}
	if roleFilter != "" && (roleFilter == string(RoleUser) || roleFilter == string(RoleAdmin)) {
		whereClauses = append(whereClauses, fmt.Sprintf("role = $%d", argIndex))
		args = append(args, roleFilter)
		argIndex++
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = " WHERE " + strings.Join(whereClauses, " AND ")
	}

	query := "SELECT id, email, display_name, role, count(*) OVER() FROM users" + whereSQL + " ORDER BY created_at, id"
	if p.Paginated {
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", p.Limit, p.Offset())
	}

	rows, err := h.pool.Query(c.Context(), query, args...)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.ErrorRes{Success: false, Error: "gagal mengambil daftar pengguna"})
	}
	defer rows.Close()

	users := make([]Summary, 0)
	var total int
	for rows.Next() {
		var item Summary
		if err := rows.Scan(&item.ID, &item.Email, &item.DisplayName, &item.Role, &total); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(utils.ErrorRes{Success: false, Error: "gagal membaca pengguna"})
		}
		users = append(users, item)
	}
	if rows.Err() != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.ErrorRes{Success: false, Error: "gagal membaca pengguna"})
	}
	if len(users) == 0 && p.Paginated && p.Offset() > 0 {
		countQuery := "SELECT count(*) FROM users" + whereSQL
		if err := h.pool.QueryRow(c.Context(), countQuery, args...).Scan(&total); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(utils.ErrorRes{Success: false, Error: "gagal mengambil daftar pengguna"})
		}
	}
	meta := utils.BuildPaginationMeta(p, total)
	return c.JSON(ListUsersResponse{Success: true, Data: users, Meta: &meta})
}

// changeRole godoc
// @Summary Ubah role pengguna
// @Description Memerlukan role Admin. Mengubah role menjadi User atau Admin. Role akun sendiri tidak dapat diubah. Perubahan hak akses berlaku pada permintaan berikutnya.
// @Tags Admin Users
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param userId path string true "UUID pengguna" Format(uuid)
// @Param request body ChangeRoleRequest true "Role baru pengguna"
// @Success 200 {object} ChangeRoleResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 403 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Router /admin/users/{userId}/role [patch]
func (h *Handler) changeRole(c fiber.Ctx) error {
	userID, err := uuid.Parse(c.Params("userId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ErrorRes{Success: false, Error: "ID pengguna tidak valid"})
	}
	var request ChangeRoleRequest
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
	h.middleware.InvalidateUser(userID)
	return c.JSON(fiber.Map{"success": true, "data": user})
}
