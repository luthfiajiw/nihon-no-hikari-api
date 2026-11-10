package course

import (
	"context"
	"testing"

	"nihon-no-hikari-api/internal/course/model"

	"github.com/google/uuid"
)

type pricingRepoStub struct {
	Repository
	pricingPlans   func(ctx context.Context) ([]model.PricingPlan, error)
	pricingPlan    func(ctx context.Context, code string) (model.PricingPlan, error)
	pricingModules func(ctx context.Context, code string) ([]model.PricingModule, error)
}

func (s *pricingRepoStub) PricingPlans(ctx context.Context) ([]model.PricingPlan, error) {
	if s.pricingPlans != nil {
		return s.pricingPlans(ctx)
	}
	return nil, nil
}

func (s *pricingRepoStub) PricingPlanByCode(ctx context.Context, code string) (model.PricingPlan, error) {
	if s.pricingPlan != nil {
		return s.pricingPlan(ctx, code)
	}
	return model.PricingPlan{}, notFoundMessage("level tidak ditemukan")
}

func (s *pricingRepoStub) PricingModulesByLevelCode(ctx context.Context, code string) ([]model.PricingModule, error) {
	if s.pricingModules != nil {
		return s.pricingModules(ctx, code)
	}
	return nil, nil
}

func TestPricingService_PricingList(t *testing.T) {
	tag := "Favorit Pemula"
	repo := &pricingRepoStub{
		pricingPlans: func(ctx context.Context) ([]model.PricingPlan, error) {
			return []model.PricingPlan{
				{
					ID:         uuid.New(),
					Code:       "N5",
					Name:       "JLPT N5",
					Tag:        &tag,
					Price:      49000,
					Points:     []string{"Poin 1"},
					OrderIndex: 0,
				},
			}, nil
		},
	}
	svc := NewService(nil, repo, nil)

	res, err := svc.PricingList(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success || len(res.Data) != 1 {
		t.Fatalf("expected 1 plan, got %+v", res)
	}
	if res.Data[0].Code != "N5" || *res.Data[0].Tag != "Favorit Pemula" {
		t.Fatalf("unexpected plan data: %+v", res.Data[0])
	}
}

func TestPricingService_PricingDetail(t *testing.T) {
	tag := "Favorit Pemula"
	n5ID := uuid.New()
	repo := &pricingRepoStub{
		pricingPlan: func(ctx context.Context, code string) (model.PricingPlan, error) {
			if code == "N5" {
				return model.PricingPlan{
					ID:         n5ID,
					Code:       "N5",
					Name:       "Paket Belajar JLPT N5",
					Tag:        &tag,
					Price:      49000,
					Points:     []string{"Poin 1", "Poin 2"},
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
						EstimatedMinutes: 720,
						LessonCount:      12,
						OrderIndex:       1,
						Lessons: []model.PricingLesson{
							{
								ID:         uuid.New(),
								Slug:       "hiragana-dasar",
								Title:      "Hiragana Dasar",
								OrderIndex: 1,
							},
						},
					},
					{
						ID:               uuid.New(),
						Slug:             "kanji-dasar",
						Title:            "Kanji Dasar",
						EstimatedMinutes: 1080,
						LessonCount:      15,
						OrderIndex:       2,
					},
				}, nil
			}
			return nil, nil
		},
	}
	svc := NewService(nil, repo, nil)

	t.Run("found", func(t *testing.T) {
		res, err := svc.PricingDetail(context.Background(), "n5")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.Success {
			t.Fatal("expected success true")
		}
		d := res.Data
		if d.Code != "N5" || d.TotalModules != 2 || d.TotalLessons != 27 {
			t.Fatalf("unexpected detail: %+v", d)
		}
		if len(d.Modules) != 2 {
			t.Fatalf("expected 2 modules, got %d", len(d.Modules))
		}
		if len(d.Modules[0].Lessons) != 1 || d.Modules[0].Lessons[0].Slug != "hiragana-dasar" {
			t.Fatalf("expected 1 lesson in first module, got %+v", d.Modules[0].Lessons)
		}
		if d.Modules[1].Lessons == nil {
			t.Fatal("expected non-nil lessons slice in second module")
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := svc.PricingDetail(context.Background(), "unknown")
		if err == nil {
			t.Fatal("expected error for unknown level, got nil")
		}
	})
}
