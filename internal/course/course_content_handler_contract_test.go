package course

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestHandlerInputContracts(t *testing.T) {
	h := &Handler{service: NewService(nil, nil, nil)}
	app := fiber.New()
	app.Get("/catalog", h.catalog)
	h.registerAdminContent(app, "/courses", "course")
	h.registerAdminContent(app, "/modules", "module")
	h.registerAdminContent(app, "/lessons", "lesson")
	app.Post("/preview", h.preview)
	for _, tc := range []struct{ method, path, body, message string }{
		{"GET", "/catalog?course_id=bad", "", "ID kursus tidak valid"},
		{"GET", "/courses/bad", "", "ID tidak valid"},
		{"POST", "/courses", `{}`, "slug, judul, atau urutan tidak valid"},
		{"POST", "/courses", `[]`, "json: cannot unmarshal array into Go value of type admin.coreInput"},
		{"POST", "/courses", `{"order_index":"bad"}`, "json: cannot unmarshal string into Go struct field coreInput.order_index of type int16"},
		{"PATCH", "/modules/bad", `{}`, "ID tidak valid"},
		{"POST", "/lessons/bad/publish", "", "ID tidak valid"},
		{"DELETE", "/courses/bad", "", "ID tidak valid"},
		{"POST", "/courses/bad/restore", "", "ID tidak valid"},
		{"POST", "/preview", `{`, "konten tidak valid"},
		{"POST", "/preview", `{"content":[]}`, "konten lesson harus berupa objek dengan content dan tips"},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			res, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			var body map[string]any
			if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if res.StatusCode != 400 || len(body) != 2 || body["success"] != false || body["error"] != tc.message {
				t.Fatalf("status=%d body=%v; want 400/%q", res.StatusCode, body, tc.message)
			}
		})
	}
}

func TestPreviewContentContract(t *testing.T) {
	app := fiber.New()
	h := &Handler{service: NewService(nil, nil, nil)}
	app.Post("/preview", h.preview)
	for _, raw := range []string{`{}`, `{"content":null}`, `{"content":"<p>legacy</p>"}`, `{"content":{"content":"<p>safe</p><script>bad()</script>","tips":" tip ","custom":{"enabled":true}}}`} {
		req := httptest.NewRequest("POST", "/preview", strings.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil || res.StatusCode != 200 || strings.Contains(string(body), "<script>") {
			t.Fatalf("%s: %d %s %v", raw, res.StatusCode, body, err)
		}
		var actual struct {
			Success bool                       `json:"success"`
			Data    map[string]json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(body, &actual); err != nil {
			t.Fatal(err)
		}
		if !actual.Success || actual.Data["content"] == nil || actual.Data["tips"] == nil {
			t.Fatalf("missing defaults: %s", body)
		}
		if strings.Contains(raw, "custom") && string(actual.Data["custom"]) != `{"enabled":true}` {
			t.Fatalf("extension lost: %s", body)
		}
	}
}
