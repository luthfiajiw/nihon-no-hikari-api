---
name: backend-module-scaffolding
description: Use when creating a new domain module, layer, route, or refactoring backend code in nihon-no-hikari-api according to backend module conventions.
---

# Backend Module Scaffolding

Standard operating procedure for scaffolding and implementing domain modules in the `nihon-no-hikari-api` Go REST API according to the official codebase architecture and conventions.

---

## 1. Core Architectural Principles

1. **Strict Domain Ownership (No Central Admin Module)**:
   - **THERE IS NO** `internal/admin` module.
   - Administrative capabilities are handled directly within their owning domain's handler (`internal/<module>/<module>_handler.go`).
   - Each domain registers its own public, student, and administrative routes via `RegisterRoutes(app *fiber.App)`.

2. **Domain Boundaries & Cross-Domain Communication**:
   - Circular imports between domains are strictly prohibited (e.g., `course` may read from `question` via a read-only port such as `CatalogReader`, but `question` must never import `course`).
   - Never dispatch write operations to another domain directly.

3. **Administrative Pagination Standards**:
   - All administrative listings support pagination query parameters (`?page=<int>&limit=<int>&search=<string>`) parsed via `utils.ParsePaginationParams(c)`.
   - Handlers construct filter structs embedding `utils.PaginationParams` and pass them to the service and repository.
   - Repositories execute a count query (`COUNT(*)`) alongside the data query with `LIMIT ... OFFSET ...` using `params.Offset()`.
   - Repositories and services return `utils.BuildPaginationMeta(filter.Pagination, total)`.
   - Backward compatibility: When `page` and `limit` are not provided, queries return all matching items with a default `PaginationMeta`.

4. **Database & Transactions**:
   - Use `pgx/v5` (`*pgxpool.Pool` at construction time in `NewRepository(pool *pgxpool.Pool)`).
   - **Shared `db.Querier` & `dbRepository` naming**: Implement repositories with an unexported `type dbRepository struct { q db.Querier }` backed by the centralized `db.Querier` interface from `nihon-no-hikari-api/pkg/db`. **Do not** declare duplicate module-local `querier` or `dbQuerier` interfaces.
   - **Single query handle**: Route every repository query through `r.q` (`Exec`, `Query`, `QueryRow`). Do **not** store a separate `pool *pgxpool.Pool` field on `dbRepository` alongside `q db.Querier` so transaction-bound calls can never accidentally escape onto the connection pool (the sole legacy exception is `question.dbRepository`, which keeps `pool` strictly for existing attempt transactions).
   - **Transactional composability (`db.WithTx` + `UseTx`)**: Repositories must expose `UseTx(pgx.Tx) Repository` returning `&dbRepository{q: tx}` (no per-method `*Tx(ctx, tx, ...)` variants such as `CreateTx`). Services own transaction boundaries via `db.WithTx` (`s.tx.Exec`) and bind participating repositories with `s.repo.UseTx(tx)`.

---

## 2. Mandatory File Naming Conventions

Every Go file under `internal/<module>` **MUST** start with the module's name:

| Category | File Naming Pattern | Examples |
| :--- | :--- | :--- |
| **Primary Layer** | `<module>_<layer>.go` | `kotoba_model.go`<br>`kotoba_repository.go`<br>`kotoba_service.go`<br>`kotoba_handler.go` |
| **Feature Layer** | `<module>_<feature>_<layer>.go` | `course_content_repository.go`<br>`question_content_service.go` |
| **Helpers / Sub-logic** | `<module>_<helper>.go` | `kotoba_media.go`<br>`question_stroke_evaluator.go` |
| **Nested Models** | `<module>/model/<module>_<entity>_model.go` | `course/model/course_lesson_model.go` |
| **Unit & Contract Tests** | `<source_name>_test.go` | `kotoba_handler_test.go`<br>`course_content_handler_contract_test.go` |
| **Database Suite Tests** | `<module>_[feature]_database_test.go` | `kotoba_database_test.go`<br>`question_content_database_test.go` |

---

## 3. Step-by-Step Module Scaffolding

When creating a new domain module (e.g., `bookmark`), follow this sequence:

### Step 1: Model, DTOs & Sentinel Errors (`<module>_model.go`)
Define database entity structs, request inputs, response DTOs, and domain-specific sentinel errors:

```go
package bookmark

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound = errors.New("bookmark not found")
	ErrConflict = errors.New("bookmark already exists")
)

type FieldErrors map[string]string

func (e FieldErrors) Error() string { return "please review the highlighted fields" }

type Bookmark struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	TargetID  uuid.UUID `json:"target_id"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateBookmarkInput struct {
	TargetID uuid.UUID `json:"target_id" validate:"required"`
}

type ErrorResponse struct {
	Success     bool              `json:"success"`
	Error       string            `json:"error"`
	FieldErrors map[string]string `json:"field_errors,omitempty"`
}
```

---

### Step 2: Repository (`<module>_repository.go`)
Implement the domain `Repository` interface with `dbRepository` backed by the shared `db.Querier` from `pkg/db` and expose `UseTx(pgx.Tx) Repository`:

> [!IMPORTANT]
> Never redeclare a private `querier` / `dbQuerier` interface in the module, never keep a raw `*pgxpool.Pool` field on `dbRepository`, and never add `*Tx(ctx, tx, ...)` method duplicates—always bind via `UseTx(tx)`.

```go
package bookmark

import (
	"context"
	"errors"

	"nihon-no-hikari-api/pkg/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	UseTx(pgx.Tx) Repository
	ListByUser(ctx context.Context, userID uuid.UUID) ([]Bookmark, error)
	Create(ctx context.Context, userID, targetID uuid.UUID) (Bookmark, error)
	Delete(ctx context.Context, userID, id uuid.UUID) error
}

type dbRepository struct {
	q db.Querier
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &dbRepository{q: pool}
}

func (r *dbRepository) UseTx(tx pgx.Tx) Repository {
	return &dbRepository{q: tx}
}

func (r *dbRepository) ListByUser(ctx context.Context, userID uuid.UUID) ([]Bookmark, error) {
	query := `SELECT id, user_id, target_id, created_at FROM bookmarks WHERE user_id = $1`
	rows, err := r.q.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Bookmark
	for rows.Next() {
		var b Bookmark
		if err := rows.Scan(&b.ID, &b.UserID, &b.TargetID, &b.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, b)
	}
	return items, rows.Err()
}

// ... implement Create and Delete with ErrNotFound mapped when pgx.ErrNoRows occurs
```

---

### Step 3: Service & Business Operations (`<module>_service.go`)
Define the operations interface, hold `db.WithTx` on the service struct, and bind repositories via `UseTx(tx)` inside multi-step or cross-repository transactions:

```go
package bookmark

import (
	"context"

	"nihon-no-hikari-api/pkg/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Operations interface {
	GetBookmarks(ctx context.Context, userID uuid.UUID) ([]Bookmark, error)
	AddBookmark(ctx context.Context, userID uuid.UUID, input CreateBookmarkInput) (Bookmark, error)
	RemoveBookmark(ctx context.Context, userID, bookmarkID uuid.UUID) error
}

type Service struct {
	tx   db.WithTx
	repo Repository
}

func NewService(tx db.WithTx, repo Repository) *Service {
	return &Service{tx: tx, repo: repo}
}

func (s *Service) GetBookmarks(ctx context.Context, userID uuid.UUID) ([]Bookmark, error) {
	return s.repo.ListByUser(ctx, userID)
}

func (s *Service) AddBookmark(ctx context.Context, userID uuid.UUID, input CreateBookmarkInput) (Bookmark, error) {
	var created Bookmark
	err := s.tx.Exec(ctx, func(tx pgx.Tx) error {
		repo := s.repo.UseTx(tx)
		var err error
		created, err = repo.Create(ctx, userID, input.TargetID)
		return err
	})
	return created, err
}
```

---

### Step 4: Handler & Routing (`<module>_handler.go`)
Use Fiber v3, integrate authentication/authorization middleware, and map domain errors to HTTP status codes:

> [!IMPORTANT]
> **Static vs. Parameterized Route Order:**
> Always register static routes (such as `/reorder` or `/preview`) **BEFORE** parameterized routes (`/:id`) to prevent Fiber routing collisions.

```go
package bookmark

import (
	"errors"
	"log/slog"

	"nihon-no-hikari-api/pkg/middleware"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct {
	service    Operations
	middleware middleware.Middleware
}

func NewHandler(service Operations, m middleware.Middleware) *Handler {
	return &Handler{service: service, middleware: m}
}

func (h *Handler) RegisterRoutes(app *fiber.App) {
	// 1. User / Student Endpoints
	userGroup := app.Group("/api/v1/bookmarks", h.middleware.AuthMiddleware())
	userGroup.Get("/", h.list)
	userGroup.Post("/", h.create)
	userGroup.Delete("/:id", h.delete)

	// 2. Administrative Endpoints (if applicable)
	adminGroup := app.Group("/api/v1/admin/bookmarks", h.middleware.AuthMiddleware(), h.middleware.RequireRole("Admin"))
	adminGroup.Get("/stats", h.adminStats) // Static route registered first
	adminGroup.Get("/:id", h.adminDetail)  // Parameterized route
}

func respondError(c fiber.Ctx, err error) error {
	status, message := 500, "failed to process request"
	var fields FieldErrors
	switch {
	case errors.As(err, &fields):
		status, message = 400, fields.Error()
	case errors.Is(err, ErrNotFound):
		status, message = 404, ErrNotFound.Error()
	case errors.Is(err, ErrConflict):
		status, message = 409, ErrConflict.Error()
	default:
		slog.Error("bookmark request error", "error", err)
	}
	return c.Status(status).JSON(ErrorResponse{Success: false, Error: message, FieldErrors: fields})
}
```

---

### Step 5: Application Bootstrap in `cmd/main.go`
Wire the new repository, service, and handler during application startup:

```go
// 1. Initialize Repository
bookmarkRepo := bookmark.NewRepository(pool)

// 2. Initialize Service
bookmarkService := bookmark.NewService(tx, bookmarkRepo)

// 3. Initialize Handler
bookmarkHandler := bookmark.NewHandler(bookmarkService, authMiddleware)

// 4. Register Routes
bookmarkHandler.RegisterRoutes(app)
```

---

## 4. Testing & Verification

1. **Unit Testing**:
   - Follow the TDD cycle (Red-Green-Refactor).
   - Name test files with `<module>_<layer>_test.go`.
   - Mock repository interfaces to test services and handlers in isolation.

2. **Build & Quality Verification**:
   Run the mandatory verification suite:
   ```bash
   go test ./...
   go vet ./...
   go build ./...
   ```

3. **Knowledge Graph Synchronization**:
   After creating or editing Go files:
   ```bash
   graphify update .
   ```
   Confirm that graphify recognizes the new handlers and dependencies without creating orphan nodes.
