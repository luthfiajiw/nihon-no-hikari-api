package user

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"nihon-no-hikari-api/pkg/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type testAuth struct{ user uuid.UUID }

func (a testAuth) AuthMiddleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		if c.Get("Authorization") == "" {
			return c.Status(401).JSON(utils.ErrorRes{Success: false, Error: "pengguna tidak terautentikasi"})
		}
		c.Locals(utils.UserIDKey, a.user)
		return c.Next()
	}
}

func (a testAuth) RequireRole(role string) fiber.Handler {
	return func(c fiber.Ctx) error {
		return c.Next()
	}
}

type serviceStub struct {
	Service
	unlinkErr error
	passedID  uuid.UUID
	passedReq UnlinkGoogleRequest
}

func (s *serviceStub) UnlinkGoogle(_ context.Context, userID uuid.UUID, req UnlinkGoogleRequest) error {
	s.passedID = userID
	s.passedReq = req
	return s.unlinkErr
}

func TestUnlinkGoogleHandler(t *testing.T) {
	testUserID := uuid.New()

	setupApp := func(svc Service) *fiber.App {
		app := fiber.New()
		h := NewHandler(nil, svc, testAuth{user: testUserID})
		h.RegisterRoutes(app)
		return app
	}

	t.Run("unauthorized request returns 401", func(t *testing.T) {
		app := setupApp(&serviceStub{})
		req := httptest.NewRequest("POST", "/api/v1/user/unlink-google", bytes.NewBufferString(`{"current_password":"password123"}`))
		req.Header.Set("Content-Type", "application/json")
		// No Authorization header

		res, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.StatusCode != 401 {
			t.Fatalf("expected status 401, got %d", res.StatusCode)
		}
	})

	t.Run("invalid json body returns 400", func(t *testing.T) {
		app := setupApp(&serviceStub{})
		req := httptest.NewRequest("POST", "/api/v1/user/unlink-google", bytes.NewBufferString(`{invalid json`))
		req.Header.Set("Authorization", "Bearer token")
		req.Header.Set("Content-Type", "application/json")

		res, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.StatusCode != 400 {
			t.Fatalf("expected status 400, got %d", res.StatusCode)
		}
	})

	t.Run("business validation errors return 400", func(t *testing.T) {
		tests := []struct {
			name        string
			err         error
			expectedMsg string
		}{
			{"cannot unlink without password", utils.ErrCannotUnlinkWithoutPassword, utils.ErrCannotUnlinkWithoutPassword.Error()},
			{"current password required", utils.ErrCurrentPasswordRequired, utils.ErrCurrentPasswordRequired.Error()},
			{"current password incorrect", utils.ErrCurrentPasswordIncorrect, utils.ErrCurrentPasswordIncorrect.Error()},
			{"google not linked", utils.ErrGoogleNotLinked, utils.ErrGoogleNotLinked.Error()},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				stub := &serviceStub{unlinkErr: tt.err}
				app := setupApp(stub)

				req := httptest.NewRequest("POST", "/api/v1/user/unlink-google", bytes.NewBufferString(`{"current_password":"pass"}`))
				req.Header.Set("Authorization", "Bearer token")
				req.Header.Set("Content-Type", "application/json")

				res, err := app.Test(req)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if res.StatusCode != 400 {
					t.Fatalf("expected status 400, got %d", res.StatusCode)
				}

				var body utils.ErrorRes
				if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if body.Error != tt.expectedMsg {
					t.Fatalf("expected message %q, got %q", tt.expectedMsg, body.Error)
				}
			})
		}
	})

	t.Run("user not found returns 404", func(t *testing.T) {
		stub := &serviceStub{unlinkErr: utils.ErrUserNotFound}
		app := setupApp(stub)

		req := httptest.NewRequest("POST", "/api/v1/user/unlink-google", bytes.NewBufferString(`{"current_password":"pass"}`))
		req.Header.Set("Authorization", "Bearer token")
		req.Header.Set("Content-Type", "application/json")

		res, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.StatusCode != 404 {
			t.Fatalf("expected status 404, got %d", res.StatusCode)
		}
	})

	t.Run("successful unlink returns 200", func(t *testing.T) {
		stub := &serviceStub{}
		app := setupApp(stub)

		req := httptest.NewRequest("POST", "/api/v1/user/unlink-google", bytes.NewBufferString(`{"current_password":"validpassword"}`))
		req.Header.Set("Authorization", "Bearer token")
		req.Header.Set("Content-Type", "application/json")

		res, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.StatusCode != 200 {
			t.Fatalf("expected status 200, got %d", res.StatusCode)
		}

		if stub.passedID != testUserID {
			t.Fatalf("expected passed userID %v, got %v", testUserID, stub.passedID)
		}
		if stub.passedReq.CurrentPassword != "validpassword" {
			t.Fatalf("expected passed password %q, got %q", "validpassword", stub.passedReq.CurrentPassword)
		}

		var body utils.SuccessRes
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if !body.Success || body.Message != "tautan akun Google berhasil diputuskan" {
			t.Fatalf("unexpected response body: %+v", body)
		}
	})
}
