package middleware

import (
	"context"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"nihon-no-hikari-api/config"
	"nihon-no-hikari-api/pkg/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type fakeRow struct {
	blocked   bool
	role      string
	expiresAt time.Time
	err       error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	*dest[0].(*bool) = r.blocked
	*dest[1].(*string) = r.role
	*dest[2].(*time.Time) = r.expiresAt
	return nil
}

type fakeSessionStore struct {
	mu      sync.Mutex
	queries atomic.Int64
	row     fakeRow
}

func (s *fakeSessionStore) QueryRow(context.Context, string, ...any) pgx.Row {
	s.queries.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.row
}

func (s *fakeSessionStore) setRow(row fakeRow) {
	s.mu.Lock()
	s.row = row
	s.mu.Unlock()
}

func newTestApp(m Middleware) *fiber.App {
	app := fiber.New()
	app.Get("/me", m.AuthMiddleware(), m.RequireRole("Admin"), func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	return app
}

func TestAuthSessionCacheHitAndInvalidation(t *testing.T) {
	now := time.Now()
	cfg := &config.Config{
		JWTSecretKey:    "test-secret",
		SessionCacheTTL: 30 * time.Second,
	}
	store := &fakeSessionStore{
		row: fakeRow{role: "Admin", expiresAt: now.Add(time.Hour)},
	}
	m := newWithStore(cfg, store)
	m.cache.now = func() time.Time { return now }

	sessionID := uuid.New()
	userID := uuid.New()
	token, _, err := utils.CreateToken(sessionID, userID, "a@example.com", time.Hour, cfg.JWTSecretKey)
	if err != nil {
		t.Fatal(err)
	}

	app := newTestApp(m)
	doReq := func() int {
		req := httptest.NewRequest("GET", "/me", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		return res.StatusCode
	}

	if code := doReq(); code != 200 {
		t.Fatalf("first request status = %d, want 200", code)
	}
	if code := doReq(); code != 200 {
		t.Fatalf("second request status = %d, want 200", code)
	}
	if got := store.queries.Load(); got != 1 {
		t.Fatalf("db queries = %d, want 1 (cache hit expected)", got)
	}

	// Advancing past TTL triggers re-validation.
	now = now.Add(31 * time.Second)
	if code := doReq(); code != 200 {
		t.Fatalf("post-TTL request status = %d, want 200", code)
	}
	if got := store.queries.Load(); got != 2 {
		t.Fatalf("db queries after TTL = %d, want 2", got)
	}

	// InvalidateSession forces DB lookup and sees blocked session immediately.
	store.setRow(fakeRow{blocked: true, role: "Admin", expiresAt: now.Add(time.Hour)})
	m.InvalidateSession(sessionID)
	if code := doReq(); code != 401 {
		t.Fatalf("blocked session status = %d, want 401", code)
	}
	if got := store.queries.Load(); got != 3 {
		t.Fatalf("db queries after invalidate = %d, want 3", got)
	}
	// Negative result must not be cached.
	if code := doReq(); code != 401 {
		t.Fatalf("second blocked session status = %d, want 401", code)
	}
	if got := store.queries.Load(); got != 4 {
		t.Fatalf("negative result was cached: queries = %d, want 4", got)
	}

	// InvalidateUser clears cached entries for that user (e.g., role change).
	store.setRow(fakeRow{role: "Admin", expiresAt: now.Add(time.Hour)})
	if code := doReq(); code != 200 {
		t.Fatalf("re-enabled session status = %d, want 200", code)
	}
	store.setRow(fakeRow{role: "User", expiresAt: now.Add(time.Hour)})
	m.InvalidateUser(userID)
	if code := doReq(); code != 403 {
		t.Fatalf("demoted user status = %d, want 403", code)
	}
}

func TestSessionCacheRulesAndConcurrency(t *testing.T) {
	now := time.Now()
	c := newSessionCache(30*time.Second, 2)
	c.now = func() time.Time { return now }

	s1, u1 := uuid.New(), uuid.New()
	s2, u2 := uuid.New(), uuid.New()
	s3, u3 := uuid.New(), uuid.New()

	// Session expiry shorter than TTL caps entry lifetime.
	c.put(s1, u1, "Admin", now.Add(5*time.Second))
	if _, ok := c.get(s1, uuid.New()); ok {
		t.Fatal("mismatched userID must not hit cache")
	}
	if role, ok := c.get(s1, u1); !ok || role != "Admin" {
		t.Fatalf("expected cache hit, got %q %v", role, ok)
	}

	// Max cap skips new inserts while full, then accepts after expired entries sweep.
	c.put(s2, u2, "User", now.Add(time.Minute))
	c.put(s3, u3, "User", now.Add(time.Minute))
	if _, ok := c.get(s3, u3); ok {
		t.Fatal("entry beyond max capacity should not be stored")
	}

	now = now.Add(6 * time.Second) // s1 expired
	c.put(s3, u3, "User", now.Add(time.Minute))
	if role, ok := c.get(s3, u3); !ok || role != "User" {
		t.Fatalf("expected s3 after expired sweep, got %q %v", role, ok)
	}

	// TTL=0 disables caching.
	disabled := newSessionCache(0, 10)
	disabled.put(s1, u1, "Admin", now.Add(time.Hour))
	if _, ok := disabled.get(s1, u1); ok {
		t.Fatal("ttl=0 must disable cache")
	}

	// Concurrency check under -race.
	cfg := &config.Config{JWTSecretKey: "race-secret", SessionCacheTTL: time.Minute}
	store := &fakeSessionStore{row: fakeRow{role: "Admin", expiresAt: time.Now().Add(time.Hour)}}
	m := newWithStore(cfg, store)
	token, _, err := utils.CreateToken(uuid.New(), uuid.New(), "race@example.com", time.Hour, cfg.JWTSecretKey)
	if err != nil {
		t.Fatal(err)
	}
	app := newTestApp(m)
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/me", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			res, err := app.Test(req)
			if err != nil || res.StatusCode != 200 {
				t.Errorf("concurrent request failed: status=%v err=%v", res, err)
			}
		}()
	}
	wg.Wait()
}
