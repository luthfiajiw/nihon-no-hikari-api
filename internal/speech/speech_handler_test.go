package speech

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nihon-no-hikari-api/config"
	"nihon-no-hikari-api/internal/course/model"
	"nihon-no-hikari-api/pkg/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type fakeLessons struct{ err error }

func (f fakeLessons) GetLessonDetail(context.Context, uuid.UUID) (*model.LessonDetail, error) {
	return &model.LessonDetail{}, f.err
}

type testAuth struct{ userID uuid.UUID }

func (a testAuth) AuthMiddleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		if c.Get("Authorization") == "" {
			return c.SendStatus(401)
		}
		c.Locals(utils.UserIDKey, a.userID)
		return c.Next()
	}
}
func (a testAuth) RequireRole(string) fiber.Handler { return a.AuthMiddleware() }

type roundTrip func(*http.Request) (*http.Response, error)

func (fn roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }

func TestIssueToken(t *testing.T) {
	for _, tc := range []struct {
		name, key, region, upstreamBody string
		auth                            bool
		lessonErr, upstreamErr          error
		upstreamStatus, want            int
	}{
		{name: "success", auth: true, key: "private-key", region: "southeastasia", upstreamStatus: 200, upstreamBody: "temporary-token", want: 200},
		{name: "requires auth", want: 401},
		{name: "unpublished lesson", auth: true, lessonErr: utils.ErrLessonNotFound, want: 404},
		{name: "database failure", auth: true, lessonErr: errors.New("secret database error"), want: 500},
		{name: "disabled", auth: true, want: 503},
		{name: "invalid region", auth: true, key: "private-key", region: "evil.test/path", want: 503},
		{name: "Azure rejection", auth: true, key: "private-key", region: "southeastasia", upstreamStatus: 401, upstreamBody: "private-key", want: 502},
		{name: "empty token", auth: true, key: "private-key", region: "southeastasia", upstreamStatus: 200, want: 502},
		{name: "oversized token", auth: true, key: "private-key", region: "southeastasia", upstreamStatus: 200, upstreamBody: strings.Repeat("x", 17000), want: 502},
		{name: "timeout", auth: true, key: "private-key", region: "southeastasia", upstreamErr: context.DeadlineExceeded, want: 504},
		{name: "network failure", auth: true, key: "private-key", region: "southeastasia", upstreamErr: errors.New("private-key"), want: 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHandler(&config.Config{AzureSpeechKey: tc.key, AzureSpeechRegion: tc.region}, fakeLessons{tc.lessonErr}, testAuth{uuid.New()})
			fixed := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
			h.now = func() time.Time { return fixed }
			h.client.Transport = roundTrip(func(req *http.Request) (*http.Response, error) {
				if req.Method != "POST" || req.URL.String() != "https://southeastasia.api.cognitive.microsoft.com/sts/v1.0/issueToken" || req.Header.Get("Ocp-Apim-Subscription-Key") != "private-key" {
					t.Fatalf("unexpected Azure request: %s %s", req.Method, req.URL)
				}
				if tc.upstreamErr != nil {
					return nil, tc.upstreamErr
				}
				return &http.Response{StatusCode: tc.upstreamStatus, Body: io.NopCloser(strings.NewReader(tc.upstreamBody))}, nil
			})
			app := fiber.New()
			h.RegisterRoutes(app)
			req := httptest.NewRequest("POST", "/api/v1/lessons/"+uuid.NewString()+"/speech-token", nil)
			if tc.auth {
				req.Header.Set("Authorization", "Bearer test")
			}
			res, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			body, _ := io.ReadAll(res.Body)
			if res.StatusCode != tc.want {
				t.Fatalf("status %d, want %d: %s", res.StatusCode, tc.want, body)
			}
			if strings.Contains(string(body), "private-key") || strings.Contains(string(body), "secret database") {
				t.Fatalf("upstream secret leaked: %s", body)
			}
			if tc.auth && res.Header.Get("Cache-Control") != "no-store" {
				t.Fatal("token responses must not be cached")
			}
			if tc.want == 200 {
				var token TokenResponse
				if err := json.Unmarshal(body, &token); err != nil {
					t.Fatal(err)
				}
				if !token.Success || token.Data.Token != "temporary-token" || token.Data.Region != "southeastasia" || !token.Data.ExpiresAt.Equal(fixed.Add(9*time.Minute)) {
					t.Fatalf("unexpected token response: %s", body)
				}
			}
		})
	}
}

func TestInvalidLessonAndUserRateLimit(t *testing.T) {
	userID := uuid.New()
	h := NewHandler(&config.Config{}, fakeLessons{}, testAuth{userID})
	now := time.Now()
	h.now = func() time.Time { return now }
	app := fiber.New()
	h.RegisterRoutes(app)
	send := func(id string) *http.Response {
		t.Helper()
		req := httptest.NewRequest("POST", "/api/v1/lessons/"+id+"/speech-token", nil)
		req.Header.Set("Authorization", "Bearer test")
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { res.Body.Close() })
		return res
	}
	if res := send("invalid"); res.StatusCode != 400 {
		t.Fatalf("invalid UUID status: %d", res.StatusCode)
	}
	for range 10 {
		if res := send(uuid.NewString()); res.StatusCode != 503 {
			t.Fatalf("request should be allowed: %d", res.StatusCode)
		}
	}
	if res := send(uuid.NewString()); res.StatusCode != 429 || res.Header.Get("Retry-After") != "60" {
		t.Fatal("rate limit must apply across lessons for the same user")
	}
	if !h.allow(uuid.New(), now) {
		t.Fatal("other users must have independent limits")
	}
	now = now.Add(time.Minute)
	if res := send(uuid.NewString()); res.StatusCode != 503 {
		t.Fatal("limit must reset after a minute")
	}
}
