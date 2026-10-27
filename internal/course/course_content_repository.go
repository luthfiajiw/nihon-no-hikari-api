package course

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"nihon-no-hikari-api/internal/course/model"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type ContentRepository interface {
	UseTx(pgx.Tx) ContentRepository
	Catalog(context.Context, *uuid.UUID) (model.CatalogData, error)
	Levels(context.Context) ([]model.Level, error)
	ListContent(context.Context, string) ([]json.RawMessage, error)
	Content(context.Context, string, uuid.UUID) (json.RawMessage, error)
	ContentState(context.Context, string, uuid.UUID) (model.ContentState, error)
	Draft(context.Context, string, uuid.UUID) (json.RawMessage, error)
	SaveDraft(context.Context, string, uuid.UUID, json.RawMessage) error
	DeleteDraft(context.Context, string, uuid.UUID) error
	SaveCore(context.Context, string, uuid.UUID, model.CoreInput, bool) error
	PrerequisiteState(context.Context, string, uuid.UUID, []uuid.UUID) (int, bool, error)
	SetPublished(context.Context, string, uuid.UUID, bool) error
	HasHistory(context.Context, string, uuid.UUID) (bool, error)
	Archive(context.Context, string, uuid.UUID) error
	DeleteContent(context.Context, string, uuid.UUID) (bool, error)
	RestoreContent(context.Context, string, uuid.UUID) error
	LockSiblings(context.Context, string, uuid.UUID) ([]uuid.UUID, error)
	ApplyOrder(context.Context, string, model.ReorderRequest) error
}

type dbQuerier interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// UseTx only binds queries; begin, commit and rollback belong to pkg/db.
func (r *dbRepository) UseTx(tx pgx.Tx) ContentRepository {
	return &dbRepository{pool: r.pool, query: tx}
}

func databaseError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrConflict) || errors.Is(err, ErrInvalidRelation) {
		return err
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: %w", ErrNotFound, err)
	}
	var p *pgconn.PgError
	if errors.As(err, &p) {
		switch p.Code {
		case "23505":
			return fmt.Errorf("%w: %w", ErrConflict, err)
		case "23503", "23514":
			return fmt.Errorf("%w: %w", ErrInvalidRelation, err)
		}
	}
	return err
}

type databaseRow struct{ pgx.Row }

func (r databaseRow) Scan(dest ...any) error { return databaseError(r.Row.Scan(dest...)) }

type databaseRows struct{ pgx.Rows }

func (r databaseRows) Scan(dest ...any) error { return databaseError(r.Rows.Scan(dest...)) }
func (r databaseRows) Err() error             { return databaseError(r.Rows.Err()) }
func (r *dbRepository) QueryRow(ctx context.Context, q string, args ...any) pgx.Row {
	return databaseRow{r.query.QueryRow(ctx, q, args...)}
}

func (r *dbRepository) Query(ctx context.Context, q string, args ...any) (pgx.Rows, error) {
	rows, err := r.query.Query(ctx, q, args...)
	if err != nil {
		return nil, databaseError(err)
	}
	return databaseRows{rows}, nil
}

func (r *dbRepository) Exec(ctx context.Context, q string, args ...any) (pgconn.CommandTag, error) {
	tag, err := r.query.Exec(ctx, q, args...)
	return tag, databaseError(err)
}

func table(kind string) (string, bool) {
	switch kind {
	case "course":
		return "courses", true
	case "module":
		return "modules", true
	case "lesson":
		return "lessons", true
	default:
		return "", false
	}
}

func (r *dbRepository) rows(ctx context.Context, query string, args ...any) ([]json.RawMessage, error) {
	rows, err := r.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]json.RawMessage, 0)
	for rows.Next() {
		var item json.RawMessage
		if err := rows.Scan(&item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func decodeRecords[T any](items []json.RawMessage) ([]T, error) {
	result := make([]T, 0, len(items))
	for _, raw := range items {
		var value T
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}

func (r *dbRepository) ListContent(ctx context.Context, kind string) ([]json.RawMessage, error) {
	name, ok := table(kind)
	if !ok {
		return nil, inputMessage("jenis konten tidak valid")
	}
	projection := "to_jsonb(t) - 'content'"
	if name == "courses" {
		projection = `to_jsonb(t) || jsonb_build_object('module_count',(SELECT count(*) FROM modules m WHERE m.course_id=t.id),'lesson_count',(SELECT count(*) FROM lessons l JOIN modules m ON m.id=l.module_id WHERE m.course_id=t.id))`
	}
	return r.rows(ctx, fmt.Sprintf(`SELECT %s FROM public.%s t ORDER BY t.order_index,t.created_at,t.id`, projection, name))
}

func (r *dbRepository) Content(ctx context.Context, kind string, id uuid.UUID) (json.RawMessage, error) {
	name, ok := table(kind)
	if !ok {
		return nil, inputMessage("jenis konten tidak valid")
	}
	projection := "to_jsonb(t)"
	if kind == "module" || kind == "lesson" {
		projection += fmt.Sprintf(` || jsonb_build_object('%s_prerequisites', COALESCE((SELECT jsonb_agg(p.required_%s_id ORDER BY p.required_%s_id) FROM %s_prerequisites p WHERE p.%s_id=t.id), '[]'::jsonb))`, kind, kind, kind, kind, kind)
	}
	var live json.RawMessage
	err := r.QueryRow(ctx, fmt.Sprintf(`SELECT %s FROM public.%s t WHERE id=$1`, projection, name), id).Scan(&live)
	return live, err
}

func (r *dbRepository) ContentState(ctx context.Context, kind string, id uuid.UUID) (model.ContentState, error) {
	name, ok := table(kind)
	if !ok {
		return model.ContentState{}, inputMessage("jenis konten tidak valid")
	}
	var state model.ContentState
	err := r.QueryRow(ctx, fmt.Sprintf(`SELECT is_published,archived_at IS NOT NULL FROM public.%s WHERE id=$1 FOR UPDATE`, name), id).Scan(&state.Published, &state.Archived)
	return state, err
}

func (r *dbRepository) Draft(ctx context.Context, kind string, id uuid.UUID) (json.RawMessage, error) {
	var raw json.RawMessage
	err := r.QueryRow(ctx, `SELECT payload FROM public.admin_drafts WHERE resource_type=$1 AND resource_id=$2`, kind, id).Scan(&raw)
	return raw, err
}

func (r *dbRepository) SaveDraft(ctx context.Context, kind string, id uuid.UUID, raw json.RawMessage) error {
	_, err := r.Exec(ctx, `INSERT INTO admin_drafts(resource_type,resource_id,payload) VALUES($1,$2,$3::jsonb) ON CONFLICT(resource_type,resource_id) DO UPDATE SET payload=EXCLUDED.payload,updated_at=now()`, kind, id, string(raw))
	return err
}

func (r *dbRepository) DeleteDraft(ctx context.Context, kind string, id uuid.UUID) error {
	_, err := r.Exec(ctx, `DELETE FROM admin_drafts WHERE resource_type=$1 AND resource_id=$2`, kind, id)
	return err
}

func (r *dbRepository) SetPublished(ctx context.Context, kind string, id uuid.UUID, value bool) error {
	name, ok := table(kind)
	if !ok {
		return inputMessage("jenis konten tidak valid")
	}
	updated := ",updated_at=now()"
	_, err := r.Exec(ctx, fmt.Sprintf(`UPDATE public.%s SET is_published=$2%s WHERE id=$1`, name, updated), id, value)
	return err
}

func (r *dbRepository) Archive(ctx context.Context, kind string, id uuid.UUID) error {
	name, ok := table(kind)
	if !ok {
		return inputMessage("jenis konten tidak valid")
	}
	_, err := r.Exec(ctx, fmt.Sprintf(`UPDATE public.%s SET is_published=false,archived_at=now() WHERE id=$1`, name), id)
	return err
}

func (r *dbRepository) DeleteContent(ctx context.Context, kind string, id uuid.UUID) (bool, error) {
	name, ok := table(kind)
	if !ok {
		return false, inputMessage("jenis konten tidak valid")
	}
	if _, err := r.Exec(ctx, `SAVEPOINT admin_delete`); err != nil {
		return false, err
	}
	_, err := r.Exec(ctx, fmt.Sprintf(`DELETE FROM public.%s WHERE id=$1`, name), id)
	var p *pgconn.PgError
	if errors.As(err, &p) && p.Code == "23503" {
		if _, err := r.Exec(ctx, `ROLLBACK TO SAVEPOINT admin_delete`); err != nil {
			return false, err
		}
		return false, nil
	}
	return err == nil, err
}

func (r *dbRepository) RestoreContent(ctx context.Context, kind string, id uuid.UUID) error {
	name, ok := table(kind)
	if !ok {
		return inputMessage("jenis konten tidak valid")
	}
	tag, err := r.Exec(ctx, fmt.Sprintf(`UPDATE public.%s SET archived_at=NULL,is_published=false WHERE id=$1 AND archived_at IS NOT NULL`, name), id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return &applicationError{kind: ErrNotFound, message: "arsip tidak ditemukan"}
	}
	return nil
}

func (r *dbRepository) HasHistory(ctx context.Context, kind string, id uuid.UUID) (bool, error) {
	historySQL := map[string]string{
		"course": `SELECT EXISTS(SELECT 1 FROM enrollments WHERE course_id=$1) OR EXISTS(SELECT 1 FROM user_module_progress p JOIN modules m ON m.id=p.module_id WHERE m.course_id=$1) OR EXISTS(SELECT 1 FROM user_lesson_progress p JOIN lessons l ON l.id=p.lesson_id JOIN modules m ON m.id=l.module_id WHERE m.course_id=$1) OR EXISTS(SELECT 1 FROM attempts a JOIN question_sets s ON s.id=a.question_set_id LEFT JOIN lessons l ON l.id=s.lesson_id JOIN modules m ON m.id=COALESCE(s.module_id,l.module_id) WHERE m.course_id=$1)`,
		"module": `SELECT EXISTS(SELECT 1 FROM user_module_progress WHERE module_id=$1) OR EXISTS(SELECT 1 FROM user_lesson_progress p JOIN lessons l ON l.id=p.lesson_id WHERE l.module_id=$1) OR EXISTS(SELECT 1 FROM attempts a JOIN question_sets s ON s.id=a.question_set_id LEFT JOIN lessons l ON l.id=s.lesson_id WHERE COALESCE(s.module_id,l.module_id)=$1)`,
		"lesson": `SELECT EXISTS(SELECT 1 FROM user_lesson_progress WHERE lesson_id=$1) OR EXISTS(SELECT 1 FROM attempts a JOIN question_sets s ON s.id=a.question_set_id WHERE s.lesson_id=$1)`,
	}
	var used bool
	if err := r.QueryRow(ctx, historySQL[kind], id).Scan(&used); err != nil {
		return false, err
	}
	return used, nil
}

func (r *dbRepository) Catalog(ctx context.Context, courseID *uuid.UUID) (model.CatalogData, error) {
	data := model.CatalogData{Courses: []json.RawMessage{}, Modules: []json.RawMessage{}, Lessons: []json.RawMessage{}, QuestionSets: []json.RawMessage{}}
	for _, name := range []string{"courses", "modules", "lessons"} {
		if courseID != nil && name != "modules" && name != "lessons" {
			continue
		}
		projection := "to_jsonb(t)"
		if name == "lessons" {
			projection = "jsonb_build_object('id',t.id,'module_id',t.module_id,'title',t.title,'slug',t.slug,'order_index',t.order_index,'is_published',t.is_published,'archived_at',t.archived_at)"
		}
		where := ""
		args := []any{}
		if courseID != nil {
			args = append(args, *courseID)
			if name == "modules" {
				where = " WHERE t.course_id=$1"
			} else {
				where = " WHERE t.module_id IN (SELECT id FROM modules WHERE course_id=$1)"
			}
		}
		items, err := r.rows(ctx, fmt.Sprintf(`SELECT %s FROM public.%s t%s ORDER BY t.order_index,t.created_at,t.id`, projection, name, where), args...)
		if err != nil {
			return data, err
		}
		switch name {
		case "courses":
			data.Courses = items
		case "modules":
			data.Modules = items
		case "lessons":
			data.Lessons = items
		}
	}
	return data, nil
}

func (r *dbRepository) Levels(ctx context.Context) ([]model.Level, error) {
	items, err := r.rows(ctx, `SELECT to_jsonb(t) FROM public.levels t ORDER BY order_index`)
	if err != nil {
		return nil, err
	}
	return decodeRecords[model.Level](items)
}

func (r *dbRepository) SaveCore(ctx context.Context, kind string, id uuid.UUID, v model.CoreInput, create bool) error {
	switch kind {
	case "course":
		if create {
			_, err := r.Exec(ctx, `INSERT INTO courses(id,level_id,slug,title,description,thumbnail_url,order_index) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, v.LevelID, v.Slug, v.Title, v.Description, v.ThumbnailURL, v.OrderIndex)
			return err
		}
		_, err := r.Exec(ctx, `UPDATE courses SET level_id=$2,slug=$3,title=$4,description=$5,thumbnail_url=$6,order_index=$7,updated_at=now() WHERE id=$1`, id, v.LevelID, v.Slug, v.Title, v.Description, v.ThumbnailURL, v.OrderIndex)
		return err
	case "module":
		if create {
			if _, err := r.Exec(ctx, `UPDATE modules SET order_index=order_index+1 WHERE course_id=$1 AND order_index >= $2`, v.CourseID, v.OrderIndex); err != nil {
				return err
			}
			_, err := r.Exec(ctx, `INSERT INTO modules(id,course_id,slug,title,description,order_index,is_mandatory,is_entry,estimated_minutes) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, v.CourseID, v.Slug, v.Title, v.Description, v.OrderIndex, v.IsMandatory, v.IsEntry, v.EstimatedMinutes)
			if err != nil {
				return err
			}
			return savePrerequisites(ctx, r, kind, id, v.ModulePrerequisites)
		}
		var oldCourse uuid.UUID
		var oldOrder int16
		if err := r.QueryRow(ctx, `SELECT course_id,order_index FROM modules WHERE id=$1`, id).Scan(&oldCourse, &oldOrder); err != nil {
			return err
		}
		if oldCourse == v.CourseID {
			if v.OrderIndex < oldOrder {
				if _, err := r.Exec(ctx, `UPDATE modules SET order_index=order_index+1 WHERE course_id=$1 AND id<>$2 AND order_index >= $3 AND order_index < $4`, oldCourse, id, v.OrderIndex, oldOrder); err != nil {
					return err
				}
			}
			if v.OrderIndex > oldOrder {
				if _, err := r.Exec(ctx, `UPDATE modules SET order_index=order_index-1 WHERE course_id=$1 AND id<>$2 AND order_index > $3 AND order_index <= $4`, oldCourse, id, oldOrder, v.OrderIndex); err != nil {
					return err
				}
			}
		} else {
			if _, err := r.Exec(ctx, `UPDATE modules SET order_index=order_index-1 WHERE course_id=$1 AND order_index > $2`, oldCourse, oldOrder); err != nil {
				return err
			}
			if _, err := r.Exec(ctx, `UPDATE modules SET order_index=order_index+1 WHERE course_id=$1 AND order_index >= $2`, v.CourseID, v.OrderIndex); err != nil {
				return err
			}
		}
		_, err := r.Exec(ctx, `UPDATE modules SET course_id=$2,slug=$3,title=$4,description=$5,order_index=$6,is_mandatory=$7,is_entry=$8,estimated_minutes=$9,updated_at=now() WHERE id=$1`, id, v.CourseID, v.Slug, v.Title, v.Description, v.OrderIndex, v.IsMandatory, v.IsEntry, v.EstimatedMinutes)
		if err != nil {
			return err
		}
		return savePrerequisites(ctx, r, kind, id, v.ModulePrerequisites)
	case "lesson":
		if create {
			_, err := r.Exec(ctx, `INSERT INTO lessons(id,module_id,slug,title,content,order_index,is_entry) VALUES($1,$2,$3,$4,$5::jsonb,$6,$7)`, id, v.ModuleID, v.Slug, v.Title, string(v.Content), v.OrderIndex, v.IsEntry)
			if err != nil {
				return err
			}
			return savePrerequisites(ctx, r, kind, id, v.LessonPrerequisites)
		}
		_, err := r.Exec(ctx, `UPDATE lessons SET module_id=$2,slug=$3,title=$4,content=$5::jsonb,order_index=$6,is_entry=$7,updated_at=now() WHERE id=$1`, id, v.ModuleID, v.Slug, v.Title, string(v.Content), v.OrderIndex, v.IsEntry)
		if err != nil {
			return err
		}
		return savePrerequisites(ctx, r, kind, id, v.LessonPrerequisites)
	}
	return errors.New("jenis konten tidak valid")
}

func prerequisiteIDs(required []uuid.UUID) []string {
	ids := make([]string, len(required))
	for i, id := range required {
		ids[i] = id.String()
	}
	return ids
}

func savePrerequisites(ctx context.Context, tx dbQuerier, kind string, id uuid.UUID, required []uuid.UUID) error {
	// Omitted/null keeps existing relations for older clients and drafts; [] clears them.
	if required == nil {
		return nil
	}
	if kind != "module" && kind != "lesson" {
		return errInvalidPrerequisites
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf(`DELETE FROM %s_prerequisites WHERE %s_id=$1`, kind, kind), id); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, fmt.Sprintf(`INSERT INTO %s_prerequisites(%s_id,required_%s_id) SELECT $1, unnest($2::uuid[])`, kind, kind, kind), id, prerequisiteIDs(required))
	return err
}

func (r *dbRepository) PrerequisiteState(ctx context.Context, kind string, id uuid.UUID, required []uuid.UUID) (int, bool, error) {
	var count int
	ids := prerequisiteIDs(required)
	if err := r.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM %ss WHERE id=ANY($1::uuid[])`, kind), ids).Scan(&count); err != nil {
		return 0, false, err
	}
	if count != len(required) {
		return count, false, nil
	}
	var cycle bool
	query := fmt.Sprintf(`WITH RECURSIVE dependencies(id) AS (
		SELECT unnest($2::uuid[])
		UNION
		SELECT p.required_%s_id FROM %s_prerequisites p JOIN dependencies d ON p.%s_id=d.id
	) SELECT EXISTS(SELECT 1 FROM dependencies WHERE id=$1)`, kind, kind, kind)
	if err := r.QueryRow(ctx, query, id, ids).Scan(&cycle); err != nil {
		return 0, false, err
	}
	return count, cycle, nil
}

func (r *dbRepository) LockSiblings(ctx context.Context, kind string, parent uuid.UUID) ([]uuid.UUID, error) {
	// Only these fixed identifiers are interpolated into SQL.
	var name, parentTable, parentField string
	switch kind {
	case "module":
		name, parentTable, parentField = "modules", "courses", "course_id"
	case "lesson":
		name, parentTable, parentField = "lessons", "modules", "module_id"
	default:
		return nil, inputMessage("jenis konten tidak valid")
	}
	var parentID uuid.UUID
	if err := r.QueryRow(ctx, fmt.Sprintf(`SELECT id FROM public.%s WHERE id=$1 FOR UPDATE`, parentTable), parent).Scan(&parentID); err != nil {
		return nil, err
	}
	rows, err := r.Query(ctx, fmt.Sprintf(`SELECT id FROM public.%s WHERE %s=$1 ORDER BY id FOR UPDATE`, name, parentField), parent)
	if err != nil {
		return nil, err
	}
	siblings, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, err
	}
	return siblings, nil
}

func (r *dbRepository) ApplyOrder(ctx context.Context, kind string, input model.ReorderRequest) error {
	var name, parentField string
	switch kind {
	case "module":
		name, parentField = "modules", "course_id"
	case "lesson":
		name, parentField = "lessons", "module_id"
	default:
		return inputMessage("jenis konten tidak valid")
	}
	ids := prerequisiteIDs(input.OrderedIDs)
	if _, err := r.Exec(ctx, fmt.Sprintf(`
		UPDATE public.%s t SET order_index=(ordering.position-1)::smallint, updated_at=now()
		FROM unnest($1::uuid[]) WITH ORDINALITY AS ordering(id, position)
		WHERE t.id=ordering.id AND t.%s=$2`, name, parentField), ids, input.ParentID); err != nil {
		return err
	}
	// A later publish must keep the new order without publishing or discarding other edits.
	// Drafts destined for another parent retain their intended destination order.
	_, err := r.Exec(ctx, fmt.Sprintf(`
		UPDATE public.admin_drafts d
		SET payload=jsonb_set(d.payload, '{order_index}', to_jsonb((ordering.position-1)::smallint)), updated_at=now()
		FROM unnest($1::uuid[]) WITH ORDINALITY AS ordering(id, position)
		WHERE d.resource_type=$2 AND d.resource_id=ordering.id AND d.payload->>'%s'=$3`, parentField), ids, kind, input.ParentID.String())
	return err
}
