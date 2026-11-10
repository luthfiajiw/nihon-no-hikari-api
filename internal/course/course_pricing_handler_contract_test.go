package course

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"nihon-no-hikari-api/internal/course/model"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

func TestPricingHandlerContract(t *testing.T) {
	tag := "Favorit Pemula"
	n5ID := uuid.New()
	repo := &pricingRepoStub{
		pricingPlans: func(ctx context.Context) ([]model.PricingPlan, error) {
			return []model.PricingPlan{
				{
					ID:         n5ID,
					Code:       "N5",
					Name:       "JLPT N5",
					Tag:        &tag,
					Price:      49000,
					Points:     []string{"Poin 1"},
					OrderIndex: 0,
				},
			}, nil
		},
		pricingPlan: func(ctx context.Context, code string) (model.PricingPlan, error) {
			if code == "N5" {
				return model.PricingPlan{
					ID:         n5ID,
					Code:       "N5",
					Name:       "JLPT N5",
					Tag:        &tag,
					Price:      49000,
					Points:     []string{"Poin 1"},
					OrderIndex: 0,
				}, nil
			}
			return model.PricingPlan{}, notFoundMessage("level tidak ditemukan")
		},
		pricingModules: func(ctx context.Context, code string) ([]model.PricingModule, error) {
			if code == "N5" {
				return []model.PricingModule{
					{
						ID:               uuid.New(),
						Slug:             "fondasi-aksara",
						Title:            "Fondasi Aksara",
						EstimatedMinutes: 120,
						LessonCount:      5,
						OrderIndex:       1,
						Lessons: []model.PricingLesson{
							{
								ID:         uuid.New(),
								Slug:       "hiragana-dasar",
								Title:      "Pengenalan Huruf Hiragana & Aturan Penulisan",
								OrderIndex: 1,
							},
						},
					},
				}, nil
			}
			return nil, nil
		},
	}

	h := &Handler{service: NewService(nil, repo, nil)}
	app := fiber.New()
	app.Get("/pricing", h.pricingList)
	app.Get("/pricing/:code", h.pricingDetail)

	t.Run("GET /pricing returns 200", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/pricing", nil)
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			t.Fatalf("expected 200, got %d", res.StatusCode)
		}
		var body model.PricingListResponse
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if !body.Success || len(body.Data) != 1 {
			t.Fatalf("unexpected body: %+v", body)
		}
	})

	t.Run("GET /pricing/n5 returns 200", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/pricing/n5", nil)
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			t.Fatalf("expected 200, got %d", res.StatusCode)
		}
		var body model.PricingDetailResponse
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if !body.Success || body.Data.Code != "N5" || len(body.Data.Modules) != 1 || len(body.Data.Modules[0].Lessons) != 1 {
			t.Fatalf("unexpected body: %+v", body)
		}
	})

	t.Run("GET /pricing/unknown returns 404", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/pricing/unknown", nil)
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode != 404 {
			t.Fatalf("expected 404, got %d", res.StatusCode)
		}
	})
}
