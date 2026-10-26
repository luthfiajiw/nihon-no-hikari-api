package kotoba

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"nihon-no-hikari-api/pkg/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type testAuth struct{ user uuid.UUID }

func (a testAuth) AuthMiddleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		if c.Get("Authorization") == "" {
			return c.Status(401).JSON(ErrorResponse{Error: "login diperlukan"})
		}
		c.Locals(utils.UserIDKey, a.user)
		return c.Next()
	}
}
func (a testAuth) RequireRole(role string) fiber.Handler {
	return func(c fiber.Ctx) error {
		if c.Get("Authorization") != "Bearer Admin" {
			return c.Status(403).JSON(ErrorResponse{Error: "akses tidak diizinkan"})
		}
		return c.Next()
	}
}

type operationsStub struct {
	Operations
	err    error
	saved  TopicInput
	filter Filter
	admin  bool
	id     uuid.UUID
}

func (s *operationsStub) List(ctx context.Context, f Filter, admin bool) ([]Topic, error) {
	s.filter, s.admin = f, admin
	return []Topic{}, s.err
}
func (s *operationsStub) Detail(ctx context.Context, id uuid.UUID, admin bool) (Topic, error) {
	s.id, s.admin = id, admin
	return Topic{ID: id}, s.err
}
func (s *operationsStub) Save(ctx context.Context, id uuid.UUID, in TopicInput) (Topic, error) {
	s.id, s.saved = id, in
	return Topic{ID: uuid.New()}, s.err
}
func (s *operationsStub) Delete(ctx context.Context, id uuid.UUID) error { s.id = id; return s.err }

func makeApp(s Operations) *fiber.App {
	app := fiber.New()
	NewHandler(s, testAuth{uuid.New()}).RegisterRoutes(app)
	return app
}
func request(t *testing.T, app *fiber.App, method, path, auth, contentType string, body io.Reader) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, data
}
func TestRouteAuthorization(t *testing.T) {
	id := uuid.NewString()
	for _, route := range []struct{ method, path string }{
		{"POST", "/api/v1/admin/kotoba/media"}, {"GET", "/api/v1/admin/kotoba/topics"}, {"POST", "/api/v1/admin/kotoba/topics"},
		{"GET", "/api/v1/admin/kotoba/topics/" + id}, {"PUT", "/api/v1/admin/kotoba/topics/" + id}, {"DELETE", "/api/v1/admin/kotoba/topics/" + id},
		{"GET", "/api/v1/kotoba/topics"}, {"GET", "/api/v1/kotoba/topics/" + id},
	} {
		app := makeApp(&operationsStub{})
		if status, _ := request(t, app, route.method, route.path, "", "", nil); status != 401 {
			t.Fatalf("missing auth on %s %s: %d", route.method, route.path, status)
		}
		if strings.Contains(route.path, "/admin/") {
			if status, _ := request(t, app, route.method, route.path, "Bearer Student", "", nil); status != 403 {
				t.Fatalf("student reached admin route: %d", status)
			}
		}
	}
}
func TestHandlerContracts(t *testing.T) {
	stub := &operationsStub{}
	app := makeApp(stub)
	if status, data := request(t, app, "GET", "/api/v1/admin/kotoba/topics?level=N5&status=active&sort=title&search=sekolah", "Bearer Admin", "", nil); status != 200 || !bytes.Contains(data, []byte(`"data":[]`)) || !stub.admin || stub.filter.Search != "sekolah" {
		t.Fatalf("list: %d %s", status, data)
	}
	if status, _ := request(t, app, "GET", "/api/v1/kotoba/topics?sort=vocabulary", "Bearer Student", "", nil); status != 200 || stub.admin {
		t.Fatal("student list failed")
	}
	in := validInput()
	body, _ := json.Marshal(in)
	if status, _ := request(t, app, "POST", "/api/v1/admin/kotoba/topics", "Bearer Admin", "application/json", bytes.NewReader(body)); status != 201 || stub.id != uuid.Nil || stub.saved.Title != "Sekolah" {
		t.Fatal("create contract failed")
	}
	id := uuid.New()
	in.Version = 2
	body, _ = json.Marshal(in)
	if status, _ := request(t, app, "PUT", "/api/v1/admin/kotoba/topics/"+id.String(), "Bearer Admin", "application/json", bytes.NewReader(body)); status != 200 || stub.id != id || stub.saved.Version != 2 {
		t.Fatal("update contract failed")
	}
	for _, method := range []string{"GET", "DELETE"} {
		if status, _ := request(t, app, method, "/api/v1/admin/kotoba/topics/"+id.String(), "Bearer Admin", "", nil); status != 200 || stub.id != id {
			t.Fatalf("%s failed", method)
		}
	}
	if status, _ := request(t, app, "GET", "/api/v1/kotoba/topics/"+id.String(), "Bearer Student", "", nil); status != 200 || stub.admin {
		t.Fatal("student detail failed")
	}
	for _, body := range []string{`null`, `{}`, `{"unexpected":true}`, `{} {}`, `{"title":"x"`, strings.Replace(string(body), `"version":2`, `"version":null`, 1)} {
		if status, _ := request(t, app, "PUT", "/api/v1/admin/kotoba/topics/"+id.String(), "Bearer Admin", "application/json", strings.NewReader(body)); status != 400 {
			t.Fatalf("accepted bad body %s: %d", body, status)
		}
	}
	if status, _ := request(t, app, "GET", "/api/v1/kotoba/topics/not-a-uuid", "Bearer Student", "", nil); status != 400 {
		t.Fatal("bad UUID accepted")
	}
	if status, _ := request(t, app, "GET", "/api/v1/admin/kotoba/topics?sort=invalid", "Bearer Admin", "", nil); status != 400 {
		t.Fatal("bad sorting accepted")
	}
}
func TestHandlerErrorEnvelopes(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{
		{FieldErrors{"title": "required"}, 400}, {ErrNotFound, 404}, {ErrConflict, 409}, {ErrTooLarge, 413}, {ErrStorage, 502}, {ErrUnavailable, 503}, {errors.New("secret database detail"), 500},
	} {
		stub := &operationsStub{err: tc.err}
		status, data := request(t, makeApp(stub), "GET", "/api/v1/admin/kotoba/topics", "Bearer Admin", "", nil)
		var result ErrorResponse
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatal(err)
		}
		if status != tc.status || result.Success || result.Error == "" || bytes.Contains(data, []byte("secret database")) {
			t.Fatalf("%v: %d %s", tc.err, status, data)
		}
		if tc.status == 400 && result.FieldErrors["title"] == "" {
			t.Fatal("missing field errors")
		}
	}
}
func multipartBody(t *testing.T, data []byte) (*bytes.Buffer, string) {
	t.Helper()
	b := &bytes.Buffer{}
	w := multipart.NewWriter(b)
	if err := w.WriteField("kind", "cover"); err != nil {
		t.Fatal(err)
	}
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="file"; filename="cover.png"`)
	header.Set("Content-Type", "image/png")
	part, err := w.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b, w.FormDataContentType()
}
func TestUploadHTTP(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		repo := &repositoryStub{}
		storage := &memoryStorage{enabled: enabled}
		svc := NewService(transactionStub{}, repo, storage, "covers", "audio")
		body, contentType := multipartBody(t, pngFixture(t))
		status, data := request(t, makeApp(svc), http.MethodPost, "/api/v1/admin/kotoba/media", "Bearer Admin", contentType, body)
		want := 503
		if enabled {
			want = 201
		}
		if status != want {
			t.Fatalf("enabled=%v: %d %s", enabled, status, data)
		}
		if enabled {
			var result MediaResponse
			if err := json.Unmarshal(data, &result); err != nil || result.Data.URL == "" || result.Data.Kind != "cover" {
				t.Fatalf("bad upload response: %s", data)
			}
		}
	}
	svc := NewService(transactionStub{}, &repositoryStub{}, &memoryStorage{enabled: true}, "covers", "audio")
	app := makeApp(svc)
	body, contentType := multipartBody(t, make([]byte, MaxCoverBytes+1))
	if status, _ := request(t, app, "POST", "/api/v1/admin/kotoba/media", "Bearer Admin", contentType, body); status != 413 {
		t.Fatalf("large file: %d", status)
	}
	if status, _ := request(t, app, "POST", "/api/v1/admin/kotoba/media", "Bearer Admin", "application/json", strings.NewReader(`{}`)); status != 400 {
		t.Fatal("non multipart accepted")
	}
}
