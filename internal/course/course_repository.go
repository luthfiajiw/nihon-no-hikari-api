package course

import (
	"context"
	"nihon-no-hikari-api/internal/course/models"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	List(ctx context.Context) ([]models.Course, error)
}

type dbRepository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &dbRepository{pool: pool}
}

func (r *dbRepository) List(ctx context.Context) ([]models.Course, error) {
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

	courses := make([]models.Course, 0)
	for rows.Next() {
		var item models.Course
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
