package course

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"nihon-no-hikari-api/internal/course/model"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type contentHandlerServiceStub struct {
	Service
	list            func(context.Context, string) (model.ContentListResponse, error)
	listPaginated   func(context.Context, string, model.AdminListFilter) (model.ContentListResponse, error)
	levelsPaginated func(context.Context, model.AdminListFilter) (model.LevelsResponse, error)
	create          func(context.Context, string, model.CoreInput) (model.CreateResponse, error)
	levelDetail     func(context.Context, uuid.UUID) (model.LevelResponse, error)
	createLevel     func(context.Context, model.LevelInput) (model.LevelResponse, error)
	updateLevel     func(context.Context, uuid.UUID, model.LevelInput) (model.LevelResponse, error)
	deleteLevel     func(context.Context, uuid.UUID) (model.StatusResponse, error)
}

func (s *contentHandlerServiceStub) ListAdminContent(c context.Context, k string) (model.ContentListResponse, error) {
	return s.list(c, k)
}

func (s *contentHandlerServiceStub) ListAdminContentPaginated(c context.Context, k string, filter model.AdminListFilter) (model.ContentListResponse, error) {
	if s.listPaginated != nil {
		return s.listPaginated(c, k, filter)
	}
	if s.list != nil {
		return s.list(c, k)
	}
	return model.ContentListResponse{}, nil
}

func (s *contentHandlerServiceStub) AdminLevelsPaginated(c context.Context, filter model.AdminListFilter) (model.LevelsResponse, error) {
	if s.levelsPaginated != nil {
		return s.levelsPaginated(c, filter)
	}
	return model.LevelsResponse{}, nil
}

func (s *contentHandlerServiceStub) CreateAdminContent(c context.Context, k string, v model.CoreInput) (model.CreateResponse, error) {
	return s.create(c, k, v)
}

func (s *contentHandlerServiceStub) AdminLevelDetail(c context.Context, id uuid.UUID) (model.LevelResponse, error) {
	if s.levelDetail != nil {
		return s.levelDetail(c, id)
	}
	return model.LevelResponse{}, nil
}

func (s *contentHandlerServiceStub) CreateAdminLevel(c context.Context, in model.LevelInput) (model.LevelResponse, error) {
	if s.createLevel != nil {
		return s.createLevel(c, in)
	}
	return model.LevelResponse{}, nil
}

func (s *contentHandlerServiceStub) UpdateAdminLevel(c context.Context, id uuid.UUID, in model.LevelInput) (model.LevelResponse, error) {
	if s.updateLevel != nil {
		return s.updateLevel(c, id, in)
	}
	return model.LevelResponse{}, nil
}

func (s *contentHandlerServiceStub) DeleteAdminLevel(c context.Context, id uuid.UUID) (model.StatusResponse, error) {
	if s.deleteLevel != nil {
		return s.deleteLevel(c, id)
	}
	return model.StatusResponse{}, nil
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
			stub := &contentHandlerServiceStub{list: func(_ context.Context, kind string) (model.ContentListResponse, error) {
				if kind != "lesson" {
					t.Fatal(kind)
				}
				called = true
				return model.ContentListResponse{}, tc.err
			}}
			h := NewHandler(stub, nil)
			app := fiber.New()
			app.Get("/lessons", func(c fiber.Ctx) error { return h.adminList(c, "lesson") })
			res, err := app.Test(httptest.NewRequest("GET", "/lessons", nil))
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
	stub := &contentHandlerServiceStub{create: func(_ context.Context, kind string, v model.CoreInput) (model.CreateResponse, error) {
		if kind != "module" || v.CourseID != parent || v.Title != "Full payload" || v.ModulePrerequisites == nil || len(v.ModulePrerequisites) != 0 {
			t.Fatalf("incorrect input: %+v", v)
		}
		return model.CreateResponse{Success: true, ID: id}, nil
	}}
	h := NewHandler(stub, nil)
	app := fiber.New()
	app.Post("/modules", func(c fiber.Ctx) error { return h.adminCreate(c, "module") })
	raw := fmt.Sprintf(`{"course_id":%q,"title":"Full payload","module_prerequisites":[]}`, parent)
	req := httptest.NewRequest("POST", "/modules", strings.NewReader(raw))
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
		{model.PublishResponse{Success: true}, `{"success":true}`},
		{model.ContentDetailResponse{Success: true, Data: json.RawMessage(`{}`)}, `{"success":true,"data":{},"draft":null}`},
		{model.ContentListResponse{Success: true, Data: []json.RawMessage{}}, `{"success":true,"data":[]}`},
	} {
		raw, err := json.Marshal(tc.value)
		if err != nil || string(raw) != tc.want {
			t.Fatalf("%s != %s (%v)", raw, tc.want, err)
		}
	}
}

func TestLevelHandlerEndpoints(t *testing.T) {
	levelID := uuid.New()
	stub := &contentHandlerServiceStub{
		levelDetail: func(_ context.Context, id uuid.UUID) (model.LevelResponse, error) {
			if id != levelID {
				t.Fatalf("unexpected id %v", id)
			}
			return model.LevelResponse{Success: true, Data: model.Level{ID: id, Code: "N5", Name: "JLPT N5", Points: []string{"Point 1"}}}, nil
		},
		createLevel: func(_ context.Context, in model.LevelInput) (model.LevelResponse, error) {
			if in.Code != "N5" {
				t.Fatalf("unexpected code %s", in.Code)
			}
			return model.LevelResponse{Success: true, Data: model.Level{ID: levelID, Code: in.Code, Name: in.Name, Points: in.Points}}, nil
		},
		updateLevel: func(_ context.Context, id uuid.UUID, in model.LevelInput) (model.LevelResponse, error) {
			if id != levelID || in.Code != "N5" {
				t.Fatalf("unexpected update %v %+v", id, in)
			}
			return model.LevelResponse{Success: true, Data: model.Level{ID: id, Code: in.Code, Name: in.Name, Points: in.Points}}, nil
		},
		deleteLevel: func(_ context.Context, id uuid.UUID) (model.StatusResponse, error) {
			if id != levelID {
				t.Fatalf("unexpected delete id %v", id)
			}
			return model.StatusResponse{Success: true}, nil
		},
	}
	h := NewHandler(stub, nil)
	app := fiber.New()
	app.Get("/levels/:id", h.levelDetail)
	app.Post("/levels", h.createLevel)
	app.Patch("/levels/:id", h.updateLevel)
	app.Delete("/levels/:id", h.deleteLevel)

	// GET detail
	req := httptest.NewRequest("GET", "/levels/"+levelID.String(), nil)
	res, err := app.Test(req)
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("detail failed %v %v", err, res.StatusCode)
	}

	// POST create
	req = httptest.NewRequest("POST", "/levels", strings.NewReader(`{"code":"N5","name":"JLPT N5","points":["Point 1"]}`))
	req.Header.Set("Content-Type", "application/json")
	res, err = app.Test(req)
	if err != nil || res.StatusCode != 201 {
		t.Fatalf("create failed %v %v", err, res.StatusCode)
	}

	// PATCH update full
	req = httptest.NewRequest("PATCH", "/levels/"+levelID.String(), strings.NewReader(`{"code":"N5","name":"JLPT N5 Updated","points":["Point 1"]}`))
	req.Header.Set("Content-Type", "application/json")
	res, err = app.Test(req)
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("update failed %v %v", err, res.StatusCode)
	}

	// DELETE
	req = httptest.NewRequest("DELETE", "/levels/"+levelID.String(), nil)
	res, err = app.Test(req)
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("delete failed %v %v", err, res.StatusCode)
	}
}

func TestAdminListAndLevelsFilterForwarding(t *testing.T) {
	var gotCourseFilter model.AdminListFilter
	var gotLevelFilter model.AdminListFilter
	stub := &contentHandlerServiceStub{
		listPaginated: func(_ context.Context, kind string, filter model.AdminListFilter) (model.ContentListResponse, error) {
			if kind != "course" {
				t.Fatalf("unexpected kind %s", kind)
			}
			gotCourseFilter = filter
			return model.ContentListResponse{Success: true, Data: []json.RawMessage{}}, nil
		},
		levelsPaginated: func(_ context.Context, filter model.AdminListFilter) (model.LevelsResponse, error) {
			gotLevelFilter = filter
			return model.LevelsResponse{Success: true, Data: []model.Level{}}, nil
		},
	}
	h := NewHandler(stub, nil)
	app := fiber.New()
	app.Get("/courses", func(c fiber.Ctx) error { return h.adminList(c, "course") })
	app.Get("/levels", h.levels)

	res, err := app.Test(httptest.NewRequest("GET", "/courses?status=published&level=%20N5%20", nil))
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("courses failed: err=%v status=%v", err, res.StatusCode)
	}
	if gotCourseFilter.Status != "published" || gotCourseFilter.Level != "N5" {
		t.Fatalf("unexpected course filter: %+v", gotCourseFilter)
	}

	res, err = app.Test(httptest.NewRequest("GET", "/levels?price_type=%20paid%20", nil))
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("levels failed: err=%v status=%v", err, res.StatusCode)
	}
	if gotLevelFilter.PriceType != "paid" {
		t.Fatalf("unexpected level filter: %+v", gotLevelFilter)
	}
}
