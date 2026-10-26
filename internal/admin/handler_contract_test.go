package admin

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
)

// These cases freeze the existing wire contract before the layer boundaries move.
func TestHandlerInputContracts(t *testing.T) {
	h := &Handler{service: NewService(nil, nil)}
	app := fiber.New()
	app.Get("/catalog", h.catalog)
	app.Get("/content/:kind", h.list)
	app.Get("/content/:kind/:id", h.detail)
	app.Post("/content/:kind", h.create)
	app.Patch("/content/:kind/:id", h.update)
	app.Post("/content/:kind/:id/publish", h.publish)
	app.Delete("/content/:kind/:id", h.remove)
	app.Post("/content/:kind/:id/restore", h.restore)
	app.Post("/content/:kind/reorder", h.reorder)
	app.Post("/templates/preview", h.previewTemplate)
	app.Post("/preview", h.preview)
	for _, tc := range []struct{ method, path, body, message string }{
		{"GET", "/catalog?course_id=bad", "", "ID kursus tidak valid"},
		{"GET", "/content/unknown", "", "jenis konten tidak valid"},
		{"GET", "/content/course/bad", "", "ID tidak valid"},
		{"POST", "/content/course", `{}`, "slug, judul, atau urutan tidak valid"},
		{"POST", "/content/course", `[]`, "json: cannot unmarshal array into Go value of type admin.coreInput"},
		{"POST", "/content/question_set", `[]`, "json: cannot unmarshal array into Go value of type admin.setInput"},
		{"POST", "/content/course", `{"order_index":"bad"}`, "json: cannot unmarshal string into Go struct field coreInput.order_index of type int16"},
		{"POST", "/content/question_set", `{"passing_score":"bad"}`, "json: cannot unmarshal string into Go struct field setInput.passing_score of type int16"},
		{"PATCH", "/content/module/bad", `{}`, "ID tidak valid"},
		{"POST", "/content/lesson/bad/publish", "", "ID tidak valid"},
		{"DELETE", "/content/course/bad", "", "ID tidak valid"},
		{"POST", "/content/course/bad/restore", "", "ID tidak valid"},
		{"POST", "/content/course/reorder", `{}`, "jenis konten tidak valid"},
		{"POST", "/content/question_set", `{}`, "jenis set tidak valid"},
		{"POST", "/templates/preview", `{"unknown":true}`, "payload template tidak valid"},
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
	h := &Handler{service: NewService(nil, nil)}
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
