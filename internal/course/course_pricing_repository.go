package course

import (
	"context"
	"encoding/json"
	"errors"

	"nihon-no-hikari-api/internal/course/model"
)

type PricingRepository interface {
	PricingPlans(ctx context.Context) ([]model.PricingPlan, error)
	PricingPlanByCode(ctx context.Context, code string) (model.PricingPlan, error)
	PricingModulesByLevelCode(ctx context.Context, code string) ([]model.PricingModule, error)
}

func (r *dbRepository) PricingPlans(ctx context.Context) ([]model.PricingPlan, error) {
	items, err := r.rows(ctx, `SELECT to_jsonb(t) FROM public.levels t ORDER BY order_index`)
	if err != nil {
		return nil, err
	}
	plans, err := decodeRecords[model.PricingPlan](items)
	if err != nil {
		return nil, err
	}
	for i := range plans {
		if plans[i].Points == nil {
			plans[i].Points = []string{}
		}
	}
	return plans, nil
}

func (r *dbRepository) PricingPlanByCode(ctx context.Context, code string) (model.PricingPlan, error) {
	var raw json.RawMessage
	err := r.QueryRow(ctx, `SELECT to_jsonb(t) FROM public.levels t WHERE UPPER(code) = UPPER($1)`, code).Scan(&raw)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return model.PricingPlan{}, notFoundMessage("level tidak ditemukan")
		}
		return model.PricingPlan{}, err
	}
	var plan model.PricingPlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return model.PricingPlan{}, err
	}
	if plan.Points == nil {
		plan.Points = []string{}
	}
	return plan, nil
}

func (r *dbRepository) PricingModulesByLevelCode(ctx context.Context, code string) ([]model.PricingModule, error) {
	items, err := r.rows(ctx, `
		SELECT jsonb_build_object(
			'id', m.id,
			'slug', m.slug,
			'title', m.title,
			'description', m.description,
			'estimated_minutes', COALESCE(m.estimated_minutes, 0),
			'lesson_count', COUNT(l.id) FILTER (WHERE l.is_published AND l.archived_at IS NULL),
			'order_index', m.order_index,
			'lessons', COALESCE(
				jsonb_agg(
					jsonb_build_object(
						'id', l.id,
						'slug', l.slug,
						'title', l.title,
						'order_index', l.order_index
					) ORDER BY l.order_index ASC
				) FILTER (WHERE l.id IS NOT NULL AND l.is_published AND l.archived_at IS NULL),
				'[]'::jsonb
			)
		)
		FROM public.modules m
		JOIN public.courses c ON c.id = m.course_id
		JOIN public.levels lv ON lv.id = c.level_id
		LEFT JOIN public.lessons l ON l.module_id = m.id
		WHERE UPPER(lv.code) = UPPER($1)
		  AND c.is_published AND c.archived_at IS NULL
		  AND m.is_published AND m.archived_at IS NULL
		GROUP BY m.id, c.order_index
		ORDER BY c.order_index, m.order_index
	`, code)
	if err != nil {
		return nil, err
	}
	modules, err := decodeRecords[model.PricingModule](items)
	if err != nil {
		return nil, err
	}
	for i := range modules {
		if modules[i].Lessons == nil {
			modules[i].Lessons = []model.PricingLesson{}
		}
	}
	return modules, nil
}
