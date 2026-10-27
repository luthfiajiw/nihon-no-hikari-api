package question

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestHandlerInputContracts(t *testing.T) {
	h := &Handler{service: NewService(nil, nil)}
	app := fiber.New()
	app.Post("/question-sets", h.adminCreate)
	app.Get("/question-sets/:id", h.adminDetail)
	app.Patch("/question-sets/:id", h.adminUpdate)
	app.Post("/question-sets/:id/publish", h.adminPublish)
	app.Delete("/question-sets/:id", h.adminRemove)
	app.Post("/question-sets/:id/restore", h.adminRestore)
	app.Post("/templates/preview", h.previewTemplate)
	for _, tc := range []struct{ method, path, body, message string }{
		{"GET", "/question-sets/bad", "", "ID tidak valid"},
		{"PATCH", "/question-sets/bad", `{}`, "ID tidak valid"},
		{"POST", "/question-sets/bad/publish", "", "ID tidak valid"},
		{"DELETE", "/question-sets/bad", "", "ID tidak valid"},
		{"POST", "/question-sets/bad/restore", "", "ID tidak valid"},
		{"POST", "/question-sets", `[]`, "json: cannot unmarshal array into Go value of type admin.setInput"},
		{"POST", "/question-sets", `{"passing_score":"bad"}`, "json: cannot unmarshal string into Go struct field setInput.passing_score of type int16"},
		{"POST", "/question-sets", `{}`, "jenis set tidak valid"},
		{"POST", "/templates/preview", `{"unknown":true}`, "payload template tidak valid"},
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
