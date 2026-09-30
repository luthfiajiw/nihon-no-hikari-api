package course

import (
	"context"
	"errors"
	"nihon-no-hikari-api/internal/course/model"
	"nihon-no-hikari-api/pkg/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	List(ctx context.Context) ([]model.Course, error)
	ListLessons(ctx context.Context, courseID uuid.UUID) ([]model.ModuleLesson, error)
	GetLessonDetail(ctx context.Context, courseID, lessonID uuid.UUID) (*model.LessonDetail, error)
}

func (r *dbRepository) GetLessonDetail(ctx context.Context, courseID, lessonID uuid.UUID) (*model.LessonDetail, error) {
	const query = `
		SELECT
			ls.id,
			ls.slug,
			ls.title,
			COALESCE(ls.content, '{}'::jsonb)::text
		FROM lessons ls
		JOIN modules m ON m.id = ls.module_id
		JOIN courses c ON c.id = m.course_id
		WHERE c.id = $1
			AND ls.id = $2
			AND c.is_published = true
			AND m.is_published = true
			AND ls.is_published = true
	`

	var lesson model.LessonDetail
	if err := r.pool.QueryRow(ctx, query, courseID, lessonID).Scan(
		&lesson.ID,
		&lesson.Slug,
		&lesson.Title,
		&lesson.Content,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, utils.ErrLessonNotFound
		}
		return nil, err
	}

	return &lesson, nil
}

func (r *dbRepository) ListLessons(ctx context.Context, courseID uuid.UUID) ([]model.ModuleLesson, error) {
	const courseQuery = `
		SELECT id
		FROM courses
		WHERE id = $1 AND is_published = true
	`

	if err := r.pool.QueryRow(ctx, courseQuery, courseID).Scan(&courseID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, utils.ErrCourseNotFound
		}
		return nil, err
	}

	const moduleQuery = `
		SELECT
			id,
			slug,
			title,
			description,
			is_mandatory,
			is_entry,
			COALESCE(estimated_minutes, 0)
		FROM modules
		WHERE course_id = $1 AND is_published = true
		ORDER BY order_index ASC
	`

	moduleRows, err := r.pool.Query(ctx, moduleQuery, courseID)
	if err != nil {
		return nil, err
	}
	defer moduleRows.Close()

	items := make([]model.ModuleLesson, 0)
	moduleIndexes := make(map[uuid.UUID]int)
	for moduleRows.Next() {
		var item model.ModuleLesson
		if err := moduleRows.Scan(
			&item.Module.ID,
			&item.Module.Slug,
			&item.Module.Title,
			&item.Module.Description,
			&item.Module.IsMandatory,
			&item.Module.IsEntry,
			&item.Module.EstimatedMinutes,
		); err != nil {
			return nil, err
		}
		item.Lessons = make([]model.Lesson, 0)
		moduleIndexes[item.Module.ID] = len(items)
		items = append(items, item)
	}
	if err := moduleRows.Err(); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return items, nil
	}

	const lessonQuery = `
		SELECT
			ls.id,
			ls.module_id,
			ls.slug,
			ls.title
		FROM lessons ls
		JOIN modules m ON m.id = ls.module_id
		WHERE m.course_id = $1
			AND m.is_published = true
			AND ls.is_published = true
		ORDER BY m.order_index ASC, ls.order_index ASC
	`

	lessonRows, err := r.pool.Query(ctx, lessonQuery, courseID)
	if err != nil {
		return nil, err
	}
	defer lessonRows.Close()

	for lessonRows.Next() {
		var lesson model.Lesson
		var moduleID uuid.UUID
		if err := lessonRows.Scan(
			&lesson.ID,
			&moduleID,
			&lesson.Slug,
			&lesson.Title,
		); err != nil {
			return nil, err
		}

		if index, ok := moduleIndexes[moduleID]; ok {
			items[index].Lessons = append(items[index].Lessons, lesson)
		}
	}
	if err := lessonRows.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

type dbRepository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &dbRepository{pool: pool}
}

func (r *dbRepository) List(ctx context.Context) ([]model.Course, error) {
	const query = `
		SELECT
			c.id,
			c.slug,
			c.title,
			c.description,
			c.thumbnail_url,
			COALESCE(c.hours, 0),
			(
				SELECT COUNT(*)::integer
				FROM modules m
				JOIN lessons ls ON ls.module_id = m.id
				WHERE m.course_id = c.id
					AND m.is_published = true
					AND ls.is_published = true
			),
			l.id,
			l.code,
			l.name
		FROM courses c
		JOIN levels l ON l.id = c.level_id
		WHERE c.is_published = true
		ORDER BY l.order_index ASC, c.created_at ASC
	`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	courses := make([]model.Course, 0)
	for rows.Next() {
		var item model.Course
		if err := rows.Scan(
			&item.ID,
			&item.Slug,
			&item.Title,
			&item.Description,
			&item.ThumbnailUrl,
			&item.TotalHours,
			&item.TotalLessons,
			&item.Level.ID,
			&item.Level.Code,
			&item.Level.Name,
		); err != nil {
			return nil, err
		}
		courses = append(courses, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return courses, nil
}
