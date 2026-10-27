package main

import (
	"context"
	"log"
	"nihon-no-hikari-api/config"
	_ "nihon-no-hikari-api/docs"
	"nihon-no-hikari-api/internal/auth"
	"nihon-no-hikari-api/internal/course"
	"nihon-no-hikari-api/internal/kotoba"
	"nihon-no-hikari-api/internal/question"
	"nihon-no-hikari-api/internal/speech"
	"nihon-no-hikari-api/internal/user"
	"nihon-no-hikari-api/pkg/db"
	"nihon-no-hikari-api/pkg/middleware"
	"nihon-no-hikari-api/pkg/supabasestorage"
	"nihon-no-hikari-api/pkg/utils"

	"github.com/go-playground/validator/v10"
	swaggo "github.com/gofiber/contrib/v3/swaggo"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/logger"
)

var validate = validator.New()
var bodyValidator = utils.BodyValidator{
	Validator: validate,
}

//go:generate go run github.com/swaggo/swag/cmd/swag@v1.16.6 init -g cmd/main.go -d .. -o ../docs --parseDependency --parseInternal

// @title Nihon no Hikari API
// @version 1.0
// @description REST API untuk platform pembelajaran Nihon no Hikari.
// @BasePath /api/v1
// @schemes http https
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Masukkan access token dengan format: Bearer {token}
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
	kotobaRepo := kotoba.NewRepository(pool)

	// SERVICES
	authService := auth.NewService(tx, cfg, authRepo, userRepo)
	courseService := course.NewService(tx, courseRepo, questionRepo)
	questionService := question.NewService(tx, questionRepo)
	storage := supabasestorage.New(cfg.SupabaseURL, cfg.SupabaseServiceKey, nil)
	kotobaService := kotoba.NewService(tx, kotobaRepo, storage, cfg.KotobaCoverBucket, cfg.KotobaAudioBucket)
	cleanupCtx, stopCleanup := context.WithCancel(context.Background())
	defer stopCleanup()
	go kotobaService.RunCleanup(cleanupCtx)

	// MIDDLEWARE
	authMiddleware := middleware.New(cfg, pool)

	// HANDLERS
	authHandler := auth.NewHandler(authService, bodyValidator, authMiddleware)
	courseHandler := course.NewHandler(courseService, authMiddleware)
	questionHandler := question.NewHandler(questionService, authMiddleware)
	userHandler := user.NewHandler(pool, authMiddleware)
	speechHandler := speech.NewHandler(cfg, courseRepo, authMiddleware)
	kotobaHandler := kotoba.NewHandler(kotobaService, authMiddleware)

	// FIBER APP
	app := fiber.New()
	app.Use(logger.New())
	app.Get("/swagger/*", swaggo.HandlerDefault)

	authHandler.RegisterRoutes(app)
	courseHandler.RegisterRoutes(app)
	questionHandler.RegisterRoutes(app)
	userHandler.RegisterRoutes(app)
	speechHandler.RegisterRoutes(app)
	kotobaHandler.RegisterRoutes(app)

	if err := app.Listen(cfg.Port); err != nil {
		log.Fatalf("server failed to start: %v", err)
	}
	log.Printf("Server starting on port %s", cfg.Port)
}
