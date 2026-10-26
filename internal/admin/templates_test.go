package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"nihon-no-hikari-api/config"
	"nihon-no-hikari-api/internal/testdb"
	"nihon-no-hikari-api/pkg/db"
	"nihon-no-hikari-api/pkg/kanastroke"
	"nihon-no-hikari-api/pkg/middleware"
	"nihon-no-hikari-api/pkg/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

func TestParseTemplateInput(t *testing.T) {
	for _, raw := range []string{`null`, `[]`, `{}`, `{"script":"kanji","character":"日","romaji":"hi","source_version":"test"}`,
		`{"script":"hiragana","character":"カ","romaji":"ka","source_version":"test"}`,
		`{"script":"hiragana","character":"あい","romaji":"ai","source_version":"test"}`,
		`{"script":"hiragana","character":"あ","romaji":"123456789","source_version":"test"}`,
		`{"script":"hiragana","character":"あ","romaji":"a","source_version":" "}`,
		`{"script":"hiragana","character":"あ","romaji":"a","source_version":"test","stroke_data":{}}`,
		`{"script":"hiragana","character":"あ","romaji":"a","source_version":"test"}{}`} {
		if _, err := validatedTemplateInput([]byte(raw)); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	input, err := validatedTemplateInput([]byte(`{"script":" hiragana ","character":"か\u3099","romaji":" ga ","source_version":" test "}`))
	if err != nil || input.Character != "が" || input.Romaji != "ga" {
		t.Fatalf("normalization: %+v %v", input, err)
	}
}

func TestTemplatesDatabase(t *testing.T) {
	pool := testdb.New(t, 13)
	ctx := context.Background()
	user, _, _, template := testdb.Fixture(t, pool)
	session := uuid.New()
	_, err := pool.Exec(ctx, `INSERT INTO user_sessions(id,user_id,refresh_token,user_agent,client_ip,expires_at) VALUES($1,$2,'test','test','127.0.0.1',now()+interval '1 hour')`, session, user)
	if err != nil {
		t.Fatal(err)
	}
	const secret = "kana-integration-test-only"
	token, _, err := utils.CreateToken(session, user, "test@example.test", time.Hour, secret)
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	NewHandler(NewService(db.NewTx(pool), NewRepository(pool)), middleware.New(&config.Config{JWTSecretKey: secret}, pool)).RegisterRoutes(app)
	call := func(method, path string, body any, auth bool, status int) []byte {
		t.Helper()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(method, "/api/v1/admin/templates"+path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		if auth {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		result, _ := io.ReadAll(res.Body)
		if res.StatusCode != status {
			t.Fatalf("%s %s: %d, want %d: %s", method, path, res.StatusCode, status, result)
		}
		return result
	}
	svg, err := os.ReadFile("../../pkg/db/stroke_sources/03044.svg")
	if err != nil {
		t.Fatal(err)
	}
	content := string(svg)
	input := StrokeTemplateInput{Script: "hiragana", Character: "い", Romaji: "i", SourceVersion: "test", SVGContent: &content}
	for _, route := range [][2]string{{"GET", ""}, {"GET", "/" + template.String()}, {"POST", "/preview"}, {"POST", ""}, {"PATCH", "/" + template.String()}} {
		call(route[0], route[1], input, false, 401)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET role='User' WHERE id=$1`, user); err != nil {
		t.Fatal(err)
	}
	for _, route := range [][2]string{{"GET", ""}, {"GET", "/" + template.String()}, {"POST", "/preview"}, {"POST", ""}, {"PATCH", "/" + template.String()}} {
		call(route[0], route[1], input, true, 403)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET role='Admin' WHERE id=$1`, user); err != nil {
		t.Fatal(err)
	}
	call("POST", "/preview", input, true, 200)
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM kana_stroke_templates`).Scan(&count); err != nil || count != 1 {
		t.Fatal("preview persisted a template", err)
	}
	var created StrokeTemplateResponse
	if err := json.Unmarshal(call("POST", "", input, true, 201), &created); err != nil {
		t.Fatal(err)
	}
	if created.Data.StrokeCount != 2 {
		t.Fatal("wrong stroke count")
	}
	call("POST", "", input, true, 409)
	call("GET", "/bad-id", nil, true, 400)
	call("GET", "/"+uuid.NewString(), nil, true, 404)
	call("PATCH", "/"+uuid.NewString(), input, true, 404)
	var list TemplatesResponse
	if err := json.Unmarshal(call("GET", "", nil, true, 200), &list); err != nil || len(list.Data) != 2 {
		t.Fatal("list failed", err)
	}
	if bytes.Contains(call("GET", "", nil, true, 200), []byte("stroke_data")) {
		t.Fatal("list includes geometry")
	}
	input.SVGContent = nil
	input.Romaji = "ii"
	var updated StrokeTemplateResponse
	json.Unmarshal(call("PATCH", "/"+created.Data.ID.String(), input, true, 200), &updated)
	if updated.Data.ID != created.Data.ID || updated.Data.StrokeData.Strokes[0].Path != created.Data.StrokeData.Strokes[0].Path {
		t.Fatal("metadata edit changed geometry/ID")
	}
	input.Character = "あ"
	call("PATCH", "/"+created.Data.ID.String(), input, true, 400)
	input.Script, input.Character, input.Romaji = "katakana", "カ", "ka"
	content = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 109 109"><g id="kvg:StrokePaths_030ab"><path id="kvg:030ab-s1" d="M10 10 C20 10 30 20 40 20"/></g></svg>`
	input.SVGContent = &content
	json.Unmarshal(call("PATCH", "/"+created.Data.ID.String(), input, true, 200), &updated)
	if updated.Data.ID != created.Data.ID || updated.Data.Character != "カ" || updated.Data.StrokeCount != 1 {
		t.Fatal("identity edit failed")
	}
	// An invalid replacement must leave all metadata and geometry intact.
	content = "invalid SVG"
	input.Romaji = "broken"
	call("PATCH", "/"+created.Data.ID.String(), input, true, 400)
	var current StrokeTemplateResponse
	json.Unmarshal(call("GET", "/"+created.Data.ID.String(), nil, true, 200), &current)
	if current.Data.Romaji != "ka" {
		t.Fatal("partial save after invalid SVG")
	}
	content = strings.Repeat(" ", kanastroke.MaxSVGBytes+1)
	call("POST", "", input, true, 413)
	call("POST", "/preview", input, true, 413)
	call("PATCH", "/"+created.Data.ID.String(), input, true, 413)
	input.Script, input.Character, input.Romaji, input.SourceVersion, input.SVGContent = "hiragana", "あ", "a", "r20260714", nil
	call("PATCH", "/"+created.Data.ID.String(), input, true, 400)
	// A conflict rolls back replacement geometry and metadata together.
	svg, _ = os.ReadFile("../../pkg/db/stroke_sources/03042.svg")
	content = string(svg)
	input.SVGContent = &content
	call("PATCH", "/"+created.Data.ID.String(), input, true, 409)
	json.Unmarshal(call("GET", "/"+created.Data.ID.String(), nil, true, 200), &current)
	if current.Data.Character != "カ" || current.Data.StrokeCount != 1 {
		t.Fatal("conflict did not roll back")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM kana_stroke_templates WHERE id=$1`, created.Data.ID); err == nil {
		t.Fatal("delete guard was removed")
	}
}

func validatedTemplateInput(raw []byte) (StrokeTemplateInput, error) {
	input, err := decodeTemplateInput(raw)
	if err != nil {
		return input, err
	}
	err = input.Validate()
	return input, err
}
