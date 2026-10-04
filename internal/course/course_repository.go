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
	GetCourseDetail(ctx context.Context, courseID, userID uuid.UUID) (*model.CourseDetail, error)
	ListLessons(ctx context.Context, courseID, userID uuid.UUID) ([]model.ModuleLesson, error)
	GetLessonDetail(ctx context.Context, courseID, lessonID uuid.UUID) (*model.LessonDetail, error)
	UpsertModuleProgress(ctx context.Context, courseID, moduleID, userID uuid.UUID, req model.UpsertModuleProgressRequest) (*model.ModuleProgress, error)
	UpsertLessonProgress(ctx context.Context, courseID, lessonID, userID uuid.UUID, req model.UpsertLessonProgressRequest) (*model.LessonProgress, error)
}

func (r *dbRepository) UpsertLessonProgress(ctx context.Context, courseID, lessonID, userID uuid.UUID, req model.UpsertLessonProgressRequest) (*model.LessonProgress, error) {
	const prerequisiteQuery = `
		SELECT NOT EXISTS (
			SELECT 1
			FROM lesson_prerequisites lp
			LEFT JOIN user_lesson_progress prerequisite_progress
				ON prerequisite_progress.lesson_id = lp.required_lesson_id
				AND prerequisite_progress.user_id = $3
			WHERE lp.lesson_id = ls.id
				AND COALESCE(prerequisite_progress.status::text, '') <> 'completed'
		)
		FROM lessons ls
		JOIN modules m ON m.id = ls.module_id
		JOIN courses c ON c.id = m.course_id
		WHERE ls.id = $2
			AND c.id = $1
			AND ls.is_published = true
			AND m.is_published = true
			AND c.is_published = true
	`

	var prerequisitesCompleted bool
	if err := r.pool.QueryRow(ctx, prerequisiteQuery, courseID, lessonID, userID).Scan(&prerequisitesCompleted); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, utils.ErrLessonNotFound
		}
		return nil, err
	}
	if !prerequisitesCompleted {
		return nil, utils.ErrLessonPrerequisiteNotCompleted
	}

	const query = `
		INSERT INTO user_lesson_progress AS progress (
			user_id, lesson_id, status, completed_at, updated_at
		)
		SELECT
			$1,
			ls.id,
			$4::progress_status,
			CASE WHEN $4::progress_status = 'completed' THEN now() END,
			now()
		FROM lessons ls
		JOIN modules m ON m.id = ls.module_id
		JOIN courses c ON c.id = m.course_id
		WHERE ls.id = $2
			AND c.id = $3
			AND ls.is_published = true
			AND m.is_published = true
			AND c.is_published = true
		ON CONFLICT (user_id, lesson_id) DO UPDATE SET
			status = CASE
				WHEN progress.status = 'completed' THEN progress.status
				ELSE $4::progress_status
			END,
			completed_at = CASE
				WHEN progress.status = 'completed' THEN progress.completed_at
				WHEN $4::progress_status = 'completed' THEN COALESCE(progress.completed_at, now())
				ELSE NULL
			END,
			updated_at = now()
		RETURNING user_id, lesson_id, status, completed_at, updated_at
	`

	var progress model.LessonProgress
	err := r.pool.QueryRow(ctx, query, userID, lessonID, courseID, req.Status).Scan(
		&progress.UserID,
		&progress.LessonID,
		&progress.Status,
		&progress.CompletedAt,
		&progress.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, utils.ErrLessonNotFound
	}
	if err != nil {
		return nil, err
	}

	return &progress, nil
}

func (r *dbRepository) UpsertModuleProgress(ctx context.Context, courseID, moduleID, userID uuid.UUID, req model.UpsertModuleProgressRequest) (*model.ModuleProgress, error) {
	if req.Status != nil {
		const prerequisiteQuery = `
			SELECT NOT EXISTS (
				SELECT 1
				FROM module_prerequisites mp
				LEFT JOIN user_module_progress prerequisite_progress
					ON prerequisite_progress.module_id = mp.required_module_id
					AND prerequisite_progress.user_id = $3
				WHERE mp.module_id = m.id
					AND COALESCE(prerequisite_progress.status::text, '') <> 'completed'
			)
			FROM modules m
			JOIN courses c ON c.id = m.course_id
			WHERE m.id = $2
				AND c.id = $1
				AND m.is_published = true
				AND c.is_published = true
		`

		var prerequisitesCompleted bool
		if err := r.pool.QueryRow(ctx, prerequisiteQuery, courseID, moduleID, userID).Scan(&prerequisitesCompleted); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, utils.ErrModuleNotFound
			}
			return nil, err
		}
		if !prerequisitesCompleted {
			return nil, utils.ErrModulePrerequisiteNotCompleted
		}
	}

	const query = `
		INSERT INTO user_module_progress AS progress (
			user_id, module_id, status, best_score, unlocked_at, completed_at, updated_at
		)
		SELECT
			$1,
			m.id,
			COALESCE($4::progress_status, 'locked'::progress_status),
			$5::smallint,
			CASE WHEN COALESCE($4::progress_status, 'locked'::progress_status) <> 'locked' THEN now() END,
			CASE WHEN $4::progress_status = 'completed' THEN now() END,
			now()
		FROM modules m
		JOIN courses c ON c.id = m.course_id
		WHERE m.id = $2
			AND c.id = $3
			AND m.is_published = true
			AND c.is_published = true
		ON CONFLICT (user_id, module_id) DO UPDATE SET
			status = COALESCE($4::progress_status, progress.status),
			best_score = COALESCE($5::smallint, progress.best_score),
			unlocked_at = CASE
				WHEN COALESCE($4::progress_status, progress.status) <> 'locked'
					THEN COALESCE(progress.unlocked_at, now())
				ELSE progress.unlocked_at
			END,
			completed_at = CASE
				WHEN COALESCE($4::progress_status, progress.status) = 'completed'
					THEN COALESCE(progress.completed_at, now())
				WHEN $4::progress_status IS NOT NULL THEN NULL
				ELSE progress.completed_at
			END,
			updated_at = now()
		RETURNING user_id, module_id, status, best_score, unlocked_at, completed_at, updated_at
	`

	var progress model.ModuleProgress
	err := r.pool.QueryRow(ctx, query, userID, moduleID, courseID, req.Status, req.BestScore).Scan(
		&progress.UserID,
		&progress.ModuleID,
		&progress.Status,
		&progress.BestScore,
		&progress.UnlockedAt,
		&progress.CompletedAt,
		&progress.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, utils.ErrModuleNotFound
	}
	if err != nil {
		return nil, err
	}

	return &progress, nil
}

func (r *dbRepository) GetCourseDetail(ctx context.Context, courseID, userID uuid.UUID) (*model.CourseDetail, error) {
	const query = `
		SELECT
			c.id,
			c.slug,
			c.title,
			c.description,
			c.thumbnail_url,
			COALESCE((
				SELECT SUM(m.estimated_minutes)::integer
				FROM modules m
				WHERE m.course_id = c.id
					AND m.is_published = true
			), 0),
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
		WHERE c.id = $1 AND c.is_published = true
	`

	var course model.CourseDetail
	if err := r.pool.QueryRow(ctx, query, courseID).Scan(
		&course.ID,
		&course.Slug,
		&course.Title,
		&course.Description,
		&course.ThumbnailUrl,
		&course.TotalMinutes,
		&course.TotalLessons,
		&course.Level.ID,
		&course.Level.Code,
		&course.Level.Name,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, utils.ErrCourseNotFound
		}
		return nil, err
	}

	modules, err := r.listModules(ctx, courseID, userID)
	if err != nil {
		return nil, err
	}
	course.Modules = modules

	return &course, nil
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

func (r *dbRepository) ListLessons(ctx context.Context, courseID, userID uuid.UUID) ([]model.ModuleLesson, error) {
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

	modules, err := r.listModules(ctx, courseID, userID)
	if err != nil {
		return nil, err
	}

	items := make([]model.ModuleLesson, 0, len(modules))
	moduleIndexes := make(map[uuid.UUID]int, len(modules))
	for _, module := range modules {
		items = append(items, model.ModuleLesson{
			Module:  module,
			Lessons: make([]model.Lesson, 0),
		})
		moduleIndexes[module.ID] = len(items) - 1
	}
	if len(items) == 0 {
		return items, nil
	}

	const lessonQuery = `
		SELECT
			ls.id,
			ls.module_id,
			ls.slug,
			ls.title,
			CASE
				WHEN EXISTS (
					SELECT 1
					FROM lesson_prerequisites lp
					LEFT JOIN user_lesson_progress prerequisite_progress
						ON prerequisite_progress.lesson_id = lp.required_lesson_id
						AND prerequisite_progress.user_id = $2
					WHERE lp.lesson_id = ls.id
						AND COALESCE(prerequisite_progress.status::text, '') <> 'completed'
				) THEN 'locked'
				ELSE COALESCE(lesson_progress.status::text, 'unlocked')
			END
		FROM lessons ls
		JOIN modules m ON m.id = ls.module_id
		LEFT JOIN user_lesson_progress lesson_progress
			ON lesson_progress.lesson_id = ls.id
			AND lesson_progress.user_id = $2
		WHERE m.course_id = $1
			AND m.is_published = true
			AND ls.is_published = true
		ORDER BY m.order_index ASC, ls.order_index ASC
	`

	lessonRows, err := r.pool.Query(ctx, lessonQuery, courseID, userID)
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
			&lesson.Status,
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

func (r *dbRepository) listModules(ctx context.Context, courseID, userID uuid.UUID) ([]model.Module, error) {
	const query = `
		SELECT
			m.id,
			m.slug,
			m.title,
			m.description,
			m.is_mandatory,
			m.is_entry,
			CASE
				WHEN EXISTS (
					SELECT 1
					FROM module_prerequisites mp
					LEFT JOIN user_module_progress prerequisite_progress
						ON prerequisite_progress.module_id = mp.required_module_id
						AND prerequisite_progress.user_id = $2
					WHERE mp.module_id = m.id
						AND COALESCE(prerequisite_progress.status::text, '') <> 'completed'
				) THEN 'locked'
				ELSE COALESCE(module_progress.status::text, 'unlocked')
			END,
			COALESCE(m.estimated_minutes, 0)
		FROM modules m
		LEFT JOIN user_module_progress module_progress
			ON module_progress.module_id = m.id
			AND module_progress.user_id = $2
		WHERE m.course_id = $1 AND m.is_published = true
		ORDER BY m.order_index ASC
	`

	rows, err := r.pool.Query(ctx, query, courseID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	modules := make([]model.Module, 0)
	for rows.Next() {
		var module model.Module
		if err := rows.Scan(
			&module.ID,
			&module.Slug,
			&module.Title,
			&module.Description,
			&module.IsMandatory,
			&module.IsEntry,
			&module.Status,
			&module.EstimatedMinutes,
		); err != nil {
			return nil, err
		}
		modules = append(modules, module)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return modules, nil
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
			COALESCE((
				SELECT SUM(m.estimated_minutes)::integer
				FROM modules m
				WHERE m.course_id = c.id
					AND m.is_published = true
			), 0),
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
			&item.TotalMinutes,
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
