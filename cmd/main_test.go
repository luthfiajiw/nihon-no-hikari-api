package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"nihon-no-hikari-api/internal/auth"
	"nihon-no-hikari-api/internal/course"
	"nihon-no-hikari-api/internal/course/model"
	"nihon-no-hikari-api/internal/question"
	"nihon-no-hikari-api/pkg/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type routeAuth struct{}

func (routeAuth) AuthMiddleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		if c.Get("Authorization") == "" {
			return c.SendStatus(401)
		}
		return c.Next()
	}
}

func (routeAuth) RequireRole(role string) fiber.Handler {
	if role != "Admin" {
		panic("unexpected administrative role")
	}
	return func(c fiber.Ctx) error {
		if c.Get("Authorization") != "Bearer Admin" {
			return c.SendStatus(403)
		}
		return c.Next()
	}
}

type courseRoutesService struct{ course.Service }

func (courseRoutesService) ListAdminContent(_ context.Context, kind string) (model.ContentListResponse, error) {
	return model.ContentListResponse{Success: true, Data: []json.RawMessage{json.RawMessage(`{"kind":"` + kind + `"}`)}}, nil
}

func (courseRoutesService) AdminLevels(context.Context) (model.LevelsResponse, error) {
	return model.LevelsResponse{Success: true, Data: []model.Level{}}, nil
}

type questionRoutesService struct{ question.Service }

func (questionRoutesService) ListAdminQuestionSets(context.Context) (question.ContentListResponse, error) {
	return question.ContentListResponse{Success: true, Data: []json.RawMessage{}}, nil
}

func (questionRoutesService) Templates(context.Context) (question.TemplatesResponse, error) {
	return question.TemplatesResponse{Success: true, Data: []question.StrokeTemplate{}}, nil
}

func domainRoutes() *fiber.App {
	app := fiber.New()
	m := routeAuth{}
	auth.NewHandler(nil, utils.BodyValidator{}, m).RegisterRoutes(app)
	course.NewHandler(courseRoutesService{course.NewService(nil, nil, nil)}, m).RegisterRoutes(app)
	question.NewHandler(questionRoutesService{question.NewService(nil, nil)}, m).RegisterRoutes(app)
	return app
}

func TestAdminResourceRoutes(t *testing.T) {
	app := domainRoutes()
	routes := []struct {
		method, path string
		adminStatus  int
	}{
		{"GET", "/admin/me", 200},
		{"GET", "/admin/catalog?course_id=bad", 400},
		{"GET", "/admin/levels", 200},
		{"POST", "/admin/lessons/preview", 200},
		{"POST", "/admin/modules/reorder", 400},
		{"POST", "/admin/lessons/reorder", 400},
		{"GET", "/admin/templates", 200},
		{"GET", "/admin/templates/bad", 400},
		{"POST", "/admin/templates/preview", 400},
		{"POST", "/admin/templates", 400},
		{"PATCH", "/admin/templates/bad", 400},
	}
	for _, resource := range []string{"courses", "modules", "lessons", "question-sets"} {
		base := "/admin/" + resource
		routes = append(routes, []struct {
			method, path string
			adminStatus  int
		}{
			{"GET", base, 200}, {"POST", base, 400},
			{"GET", base + "/bad", 400}, {"PATCH", base + "/bad", 400},
			{"DELETE", base + "/bad", 400}, {"POST", base + "/bad/publish", 400},
			{"POST", base + "/bad/restore", 400},
		}...)
	}
	for _, route := range routes {
		for _, access := range []struct {
			header string
			status int
		}{{"", 401}, {"Bearer Student", 403}, {"Bearer Admin", route.adminStatus}} {
			t.Run(route.method+route.path+access.header, func(t *testing.T) {
				req := httptest.NewRequest(route.method, "/api/v1"+route.path, strings.NewReader(`{}`))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", access.header)
				res, err := app.Test(req)
				if err != nil {
					t.Fatal(err)
				}
				defer res.Body.Close()
				if res.StatusCode != access.status {
					t.Fatalf("got %d; want %d", res.StatusCode, access.status)
				}
			})
		}
	}
}

func TestLegacyAdminContentRoutesAreRemoved(t *testing.T) {
	app := domainRoutes()
	for _, kind := range []string{"course", "module", "lesson", "question_set", "unknown"} {
		for _, route := range []struct{ method, suffix string }{
			{"GET", ""}, {"POST", ""}, {"POST", "/reorder"},
			{"GET", "/" + uuid.NewString()}, {"PATCH", "/" + uuid.NewString()},
			{"DELETE", "/" + uuid.NewString()}, {"POST", "/" + uuid.NewString() + "/publish"},
			{"POST", "/" + uuid.NewString() + "/restore"},
		} {
			req := httptest.NewRequest(route.method, "/api/v1/admin/content/"+kind+route.suffix, nil)
			req.Header.Set("Authorization", "Bearer Admin")
			res, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			res.Body.Close()
			if res.StatusCode != 404 {
				t.Fatalf("legacy route still available: %s %s (%d)", route.method, req.URL.Path, res.StatusCode)
			}
		}
	}
	req := httptest.NewRequest("POST", "/api/v1/admin/lesson-content/preview", nil)
	req.Header.Set("Authorization", "Bearer Admin")
	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 404 {
		t.Fatalf("legacy lesson preview returned %d", res.StatusCode)
	}
}
