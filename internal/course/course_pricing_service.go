package course

import (
	"context"
	"strings"

	"nihon-no-hikari-api/internal/course/model"
)

type PricingService interface {
	PricingList(ctx context.Context) (model.PricingListResponse, error)
	PricingDetail(ctx context.Context, code string) (model.PricingDetailResponse, error)
}

func (s *service) PricingList(ctx context.Context) (model.PricingListResponse, error) {
	plans, err := s.repository.PricingPlans(ctx)
	if err != nil {
		return model.PricingListResponse{}, err
	}
	return model.PricingListResponse{
		Success: true,
		Data:    plans,
	}, nil
}

func (s *service) PricingDetail(ctx context.Context, code string) (model.PricingDetailResponse, error) {
	cleanCode := strings.ToUpper(strings.TrimSpace(code))
	if cleanCode == "" {
		return model.PricingDetailResponse{}, notFoundMessage("level tidak ditemukan")
	}
	plan, err := s.repository.PricingPlanByCode(ctx, cleanCode)
	if err != nil {
		return model.PricingDetailResponse{}, err
	}
	modules, err := s.repository.PricingModulesByLevelCode(ctx, cleanCode)
	if err != nil {
		return model.PricingDetailResponse{}, err
	}
	if modules == nil {
		modules = []model.PricingModule{}
	}
	for i := range modules {
		if modules[i].Lessons == nil {
			modules[i].Lessons = []model.PricingLesson{}
		}
	}

	var totalLessons int32
	for _, m := range modules {
		totalLessons += m.LessonCount
	}

	detail := model.PricingDetail{
		ID:           plan.ID,
		Code:         plan.Code,
		Name:         plan.Name,
		Tag:          plan.Tag,
		Description:  plan.Description,
		Price:        plan.Price,
		Points:       plan.Points,
		OrderIndex:   plan.OrderIndex,
		TotalModules: int32(len(modules)),
		TotalLessons: totalLessons,
		Modules:      modules,
	}

	return model.PricingDetailResponse{
		Success: true,
		Data:    detail,
	}, nil
}
