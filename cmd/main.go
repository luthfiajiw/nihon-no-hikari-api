package main

import (
	"log"
	"nihon-no-hikari-api/config"
	"nihon-no-hikari-api/pkg/db"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/logger"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Can't load config: %v", err)
	}

	pool, err := db.NewPostgresPool(cfg.DBSource)
	if err != nil {
		log.Fatalf("Can't connect to database %v", err)
	}

	defer pool.Close()

	// FIBER APP
	app := fiber.New()
	app.Use(logger.New())

	if err := app.Listen(cfg.Port); err != nil {
		log.Fatalf("server failed to start: %v", err)
	}
	log.Printf("Server starting on port %s", cfg.Port)
}
