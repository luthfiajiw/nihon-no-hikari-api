package middleware

import (
	"context"
	"nihon-no-hikari-api/config"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Middleware interface {
	AuthMiddleware() fiber.Handler
	RequireRole(role string) fiber.Handler
	InvalidateSession(sessionID uuid.UUID)
	InvalidateUser(userID uuid.UUID)
}

type sessionStore interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type middleware struct {
	cfg   *config.Config
	store sessionStore
	cache *sessionCache
}

func New(cfg *config.Config, pool *pgxpool.Pool) Middleware {
	return newWithStore(cfg, pool)
}

func newWithStore(cfg *config.Config, store sessionStore) *middleware {
	return &middleware{
		cfg:   cfg,
		store: store,
		cache: newSessionCache(cfg.SessionCacheTTL, defaultSessionCacheMax),
	}
}

func (m *middleware) InvalidateSession(sessionID uuid.UUID) {
	m.cache.deleteSession(sessionID)
}

func (m *middleware) InvalidateUser(userID uuid.UUID) {
	m.cache.deleteUser(userID)
}
