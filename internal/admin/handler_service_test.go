package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type handlerServiceStub struct {
	Service
	list   func(context.Context, string) (ContentListResponse, error)
	create func(context.Context, string, coreInput) (CreateResponse, error)
}

func (s *handlerServiceStub) ListContent(c context.Context, k string) (ContentListResponse, error) {
	return s.list(c, k)
}

func (s *handlerServiceStub) CreateContent(c context.Context, k string, v coreInput) (CreateResponse, error) {
	return s.create(c, k, v)
}

func TestHandlerMapsServiceErrorsWithoutDatabase(t *testing.T) {
	for _, tc := range []struct {
		name    string
		err     error
		status  int
		message string
	}{
		{"input", inputMessage("invalid title"), 400, "invalid title"},
		{"missing", ErrNotFound, 404, "konten tidak ditemukan"},
		{"conflict", ErrConflict, 409, "slug atau urutan sudah dipakai"},
		{"relation", ErrInvalidRelation, 400, "relasi atau konfigurasi konten tidak valid"},
		{"stale", ErrStaleOrder, 409, ErrStaleOrder.Error()},
		{"large", &applicationError{kind: ErrPayloadTooLarge, message: "SVG too large"}, 413, "SVG too large"},
		{"unknown", errors.New("private database details"), 500, "gagal menyimpan konten"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			stub := &handlerServiceStub{list: func(_ context.Context, kind string) (ContentListResponse, error) {
				if kind != "lesson" {
					t.Fatal(kind)
				}
				called = true
				return ContentListResponse{}, tc.err
			}}
			h := NewHandler(stub, nil)
			app := fiber.New()
			app.Get("/content/:kind", h.list)
			res, err := app.Test(httptest.NewRequest("GET", "/content/lesson", nil))
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			var body map[string]any
			if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if !called || res.StatusCode != tc.status || body["success"] != false || body["error"] != tc.message {
				t.Fatalf("%d %v", res.StatusCode, body)
			}
		})
	}
}

func TestHandlerForwardsRequestAndReturnsTypedResult(t *testing.T) {
	parent, id := uuid.New(), uuid.New()
	stub := &handlerServiceStub{create: func(_ context.Context, kind string, v coreInput) (CreateResponse, error) {
		if kind != "module" || v.CourseID != parent || v.Title != "Full payload" || v.ModulePrerequisites == nil || len(v.ModulePrerequisites) != 0 {
			t.Fatalf("incorrect input: %+v", v)
		}
		return CreateResponse{Success: true, ID: id}, nil
	}}
	h := NewHandler(stub, nil)
	app := fiber.New()
	app.Post("/content/:kind", h.create)
	raw := fmt.Sprintf(`{"course_id":%q,"title":"Full payload","module_prerequisites":[]}`, parent)
	req := httptest.NewRequest("POST", "/content/module", strings.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var result map[string]any
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 201 || len(result) != 2 || result["success"] != true || result["id"] != id.String() {
		t.Fatalf("%d %v", res.StatusCode, result)
	}
}

func TestDatabaseErrorsUseCodesAndKeepOriginalCause(t *testing.T) {
	for _, tc := range []struct{ source, want error }{
		{pgx.ErrNoRows, ErrNotFound},
		{&pgconn.PgError{Code: "23505"}, ErrConflict},
		{&pgconn.PgError{Code: "23503"}, ErrInvalidRelation},
		{&pgconn.PgError{Code: "23514"}, ErrInvalidRelation},
	} {
		err := databaseError(fmt.Errorf("operation: %w", tc.source))
		if !errors.Is(err, tc.want) || !errors.Is(err, tc.source) {
			t.Fatalf("%v", err)
		}
	}
	ordinary := errors.New("SQLSTATE 23505")
	if err := databaseError(ordinary); err != ordinary || errors.Is(err, ErrConflict) {
		t.Fatalf("parsed error text: %v", err)
	}
}

func TestResponseEnvelopesPreserveNullAndOmittedFields(t *testing.T) {
	for _, tc := range []struct {
		value any
		want  string
	}{
		{PublishResponse{Success: true}, `{"success":true}`},
		{ContentDetailResponse{Success: true, Data: json.RawMessage(`{}`)}, `{"success":true,"data":{},"draft":null}`},
		{ContentListResponse{Success: true, Data: []json.RawMessage{}}, `{"success":true,"data":[]}`},
	} {
		raw, err := json.Marshal(tc.value)
		if err != nil || string(raw) != tc.want {
			t.Fatalf("%s != %s (%v)", raw, tc.want, err)
		}
	}
}
