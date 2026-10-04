package main

import (
	"log"
	"nihon-no-hikari-api/config"
	"nihon-no-hikari-api/internal/auth"
	"nihon-no-hikari-api/internal/course"
	"nihon-no-hikari-api/internal/question"
	"nihon-no-hikari-api/internal/user"
	"nihon-no-hikari-api/pkg/db"
	"nihon-no-hikari-api/pkg/middleware"
	"nihon-no-hikari-api/pkg/utils"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/logger"
)

var validate = validator.New()
var bodyValidator = utils.BodyValidator{
	Validator: validate,
}

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

	tx := db.NewTx(pool)

	// REPOSITORIES
	userRepo := user.NewRepository(pool)
	authRepo := auth.NewRepository(pool)
	courseRepo := course.NewRepository(pool)
	questionRepo := question.NewRepository(pool)

	// SERVICES
	authService := auth.NewService(tx, cfg, authRepo, userRepo)
	courseService := course.NewService(courseRepo, questionRepo)
	questionService := question.NewService(questionRepo)

	// MIDDLEWARE
	authMiddleware := middleware.New(cfg, pool)

	// HANDLERS
	authHandler := auth.NewHandler(authService, bodyValidator)
	courseHandler := course.NewHandler(courseService, authMiddleware)
	questionHandler := question.NewHandler(questionService, authMiddleware)

	// FIBER APP
	app := fiber.New()
	app.Use(logger.New())

	authHandler.RegisterRoutes(app)
	courseHandler.RegisterRoutes(app)
	questionHandler.RegisterRoutes(app)

	if err := app.Listen(cfg.Port); err != nil {
		log.Fatalf("server failed to start: %v", err)
	}
	log.Printf("Server starting on port %s", cfg.Port)
}
