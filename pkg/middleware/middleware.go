package middleware

import (
	"nihon-no-hikari-api/config"

	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Middleware interface {
	AuthMiddleware() fiber.Handler
}

type middleware struct {
	cfg  *config.Config
	pool *pgxpool.Pool
}

func New(cfg *config.Config, pool *pgxpool.Pool) Middleware {
	return &middleware{cfg, pool}
}
