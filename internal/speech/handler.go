package speech

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"nihon-no-hikari-api/config"
	"nihon-no-hikari-api/internal/course/model"
	"nihon-no-hikari-api/pkg/middleware"
	"nihon-no-hikari-api/pkg/utils"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type lessonReader interface {
	GetLessonDetail(context.Context, uuid.UUID) (*model.LessonDetail, error)
}

type TokenData struct {
	Token     string    `json:"token"`
	Region    string    `json:"region"`
	ExpiresAt time.Time `json:"expires_at"`
}

type TokenResponse struct {
	Success bool      `json:"success"`
	Message string    `json:"message"`
	Data    TokenData `json:"data"`
}

type requestWindow struct {
	start time.Time
	count int
}

type Handler struct {
	key        string
	region     string
	lessons    lessonReader
	middleware middleware.Middleware
	client     *http.Client
	now        func() time.Time
	mu         sync.Mutex
	windows    map[uuid.UUID]requestWindow
	nextPrune  time.Time
}

var regionPattern = regexp.MustCompile(`^[a-z][a-z0-9]{1,40}$`)

func NewHandler(cfg *config.Config, lessons lessonReader, auth middleware.Middleware) *Handler {
	return &Handler{
		key: strings.TrimSpace(cfg.AzureSpeechKey), region: strings.TrimSpace(cfg.AzureSpeechRegion),
		lessons: lessons, middleware: auth, now: time.Now,
		client: &http.Client{
			Timeout:       10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		windows: make(map[uuid.UUID]requestWindow),
	}
}

func (h *Handler) RegisterRoutes(app *fiber.App) {
	app.Post("/api/v1/lessons/:lessonId/speech-token", h.middleware.AuthMiddleware(), h.issueToken)
}

func (h *Handler) allow(userID uuid.UUID, now time.Time) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !now.Before(h.nextPrune) {
		for id, window := range h.windows {
			if now.Sub(window.start) >= time.Minute {
				delete(h.windows, id)
			}
		}
		h.nextPrune = now.Add(time.Minute)
	}
	window := h.windows[userID]
	if window.start.IsZero() || now.Sub(window.start) >= time.Minute {
		window = requestWindow{start: now}
	}
	if window.count >= 10 {
		return false
	}
	window.count++
	h.windows[userID] = window
	return true
}

// issueToken godoc
// @Summary Token sementara Azure untuk pembacaan lesson
// @Description Memerlukan lesson terpublikasi. Token disimpan di memori klien maksimal sembilan menit; dibatasi 10 permintaan per pengguna per menit.
// @Tags Lessons
// @Produce json
// @Security BearerAuth
// @Param lessonId path string true "Lesson ID"
// @Success 200 {object} TokenResponse
// @Failure 400 {object} utils.ErrorRes
// @Failure 401 {object} utils.ErrorRes
// @Failure 404 {object} utils.ErrorRes
// @Failure 429 {object} utils.ErrorRes
// @Failure 500 {object} utils.ErrorRes
// @Failure 502 {object} utils.ErrorRes
// @Failure 503 {object} utils.ErrorRes
// @Failure 504 {object} utils.ErrorRes
// @Router /lessons/{lessonId}/speech-token [post]
func (h *Handler) issueToken(c fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	fail := func(status int, message string) error {
		return c.Status(status).JSON(utils.ErrorRes{Success: false, Error: message})
	}
	userID, ok := c.Locals(utils.UserIDKey).(uuid.UUID)
	if !ok {
		return fail(401, "user tidak terautentikasi")
	}
	lessonID, err := uuid.Parse(c.Params("lessonId"))
	if err != nil {
		return fail(400, "lessonId tidak valid")
	}
	if !h.allow(userID, h.now()) {
		c.Set("Retry-After", "60")
		return fail(429, "Terlalu banyak permintaan pembacaan. Coba lagi sebentar.")
	}
	if _, err := h.lessons.GetLessonDetail(c.Context(), lessonID); err != nil {
		if errors.Is(err, utils.ErrLessonNotFound) {
			return fail(404, "lesson tidak ditemukan")
		}
		return fail(500, "Gagal memeriksa lesson.")
	}
	if h.key == "" || !regionPattern.MatchString(h.region) {
		return fail(503, "Fitur baca lesson belum dikonfigurasi.")
	}
	// Start the conservative lifetime before exchanging the key, not after network latency.
	issuedAt := h.now()
	req, err := http.NewRequestWithContext(c.Context(), http.MethodPost,
		"https://"+h.region+".api.cognitive.microsoft.com/sts/v1.0/issueToken", nil)
	if err != nil {
		return fail(503, "Konfigurasi pembacaan lesson tidak valid.")
	}
	req.Header.Set("Ocp-Apim-Subscription-Key", h.key)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := h.client.Do(req)
	if err != nil {
		var timeout net.Error
		if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &timeout) && timeout.Timeout() {
			return fail(504, "Layanan suara terlalu lama merespons. Silakan coba lagi.")
		}
		return fail(502, "Gagal terhubung ke layanan suara.")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fail(502, "Layanan suara gagal membuat token. Silakan coba lagi.")
	}
	const maxTokenSize = 16 * 1024
	body, err := io.ReadAll(io.LimitReader(res.Body, maxTokenSize+1))
	if err != nil {
		var timeout net.Error
		if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &timeout) && timeout.Timeout() {
			return fail(504, "Layanan suara terlalu lama merespons. Silakan coba lagi.")
		}
		return fail(502, "Respons layanan suara tidak valid.")
	}
	token := strings.TrimSpace(string(body))
	if len(body) > maxTokenSize || token == "" || strings.ContainsAny(token, "\r\n") {
		return fail(502, "Respons layanan suara tidak valid.")
	}
	return c.JSON(TokenResponse{Success: true, Message: "token pembacaan berhasil dibuat", Data: TokenData{
		Token: token, Region: h.region, ExpiresAt: issuedAt.Add(9 * time.Minute),
	}})
}
