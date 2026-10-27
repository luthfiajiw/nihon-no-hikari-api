package question

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type ContentRepository interface {
	CatalogReader
	UseTx(pgx.Tx) ContentRepository
	Templates(context.Context) ([]StrokeTemplate, error)
	ListContent(context.Context, string) ([]json.RawMessage, error)
	Content(context.Context, string, uuid.UUID) (json.RawMessage, error)
	ContentState(context.Context, string, uuid.UUID) (ContentState, error)
	Draft(context.Context, string, uuid.UUID) (json.RawMessage, error)
	SaveDraft(context.Context, string, uuid.UUID, json.RawMessage) error
	DeleteDraft(context.Context, string, uuid.UUID) error
	SetPublished(context.Context, string, uuid.UUID, bool) error
	HasHistory(context.Context, string, uuid.UUID) (bool, error)
	Archive(context.Context, string, uuid.UUID) error
	DeleteContent(context.Context, string, uuid.UUID) (bool, error)
	RestoreContent(context.Context, string, uuid.UUID) error
	WithQuestions(context.Context, json.RawMessage, uuid.UUID) (json.RawMessage, error)
	QuestionSetState(context.Context, uuid.UUID) (QuestionSetState, error)
	InsertSet(context.Context, uuid.UUID, uuid.UUID, int, SetInput) error
	UpdateSet(context.Context, uuid.UUID, SetInput) error
	DeleteQuestions(context.Context, uuid.UUID) error
	InsertQuestions(context.Context, uuid.UUID, SetInput) error
	Template(context.Context, uuid.UUID, bool) (StrokeTemplateDetail, error)
	InsertTemplate(context.Context, StrokeTemplateDetail) error
	UpdateTemplate(context.Context, StrokeTemplateDetail) error
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

func table(kind string) (string, bool) { return "question_sets", kind == "question_set" }

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
	return r.rows(ctx, fmt.Sprintf(`SELECT %s FROM public.%s t ORDER BY t.order_index,t.created_at,t.id`, projection, name))
}

func (r *dbRepository) Content(ctx context.Context, kind string, id uuid.UUID) (json.RawMessage, error) {
	name, ok := table(kind)
	if !ok {
		return nil, inputMessage("jenis konten tidak valid")
	}
	projection := "to_jsonb(t)"
	var live json.RawMessage
	err := r.QueryRow(ctx, fmt.Sprintf(`SELECT %s FROM public.%s t WHERE id=$1`, projection, name), id).Scan(&live)
	return live, err
}

func (r *dbRepository) ContentState(ctx context.Context, kind string, id uuid.UUID) (ContentState, error) {
	name, ok := table(kind)
	if !ok {
		return ContentState{}, inputMessage("jenis konten tidak valid")
	}
	var state ContentState
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
	// Question set publication historically leaves updated_at unchanged.
	updated := ",updated_at=now()"
	if kind == "question_set" {
		updated = ""
	}
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
		"question_set": `SELECT EXISTS(SELECT 1 FROM attempts a JOIN question_sets attempted_set ON attempted_set.id=a.question_set_id JOIN question_sets target_set ON target_set.lineage_id=attempted_set.lineage_id WHERE target_set.id=$1)`,
	}
	var used bool
	if err := r.QueryRow(ctx, historySQL[kind], id).Scan(&used); err != nil {
		return false, err
	}
	return used, nil
}

func (r *dbRepository) Templates(ctx context.Context) ([]StrokeTemplate, error) {
	items, err := r.rows(ctx, `SELECT jsonb_build_object('id',id,'character',character,'romaji',romaji,'script',script,'source_version',source_version,'stroke_count',jsonb_array_length(stroke_data->'strokes')) FROM public.kana_stroke_templates ORDER BY script,character,source_version DESC`)
	if err != nil {
		return nil, err
	}
	return decodeRecords[StrokeTemplate](items)
}

func (r *dbRepository) QuestionSetState(ctx context.Context, id uuid.UUID) (QuestionSetState, error) {
	var s QuestionSetState
	err := r.QueryRow(ctx, `SELECT lineage_id,version,is_published,archived_at IS NOT NULL,kind,skill,lesson_id,module_id,EXISTS(SELECT 1 FROM attempts WHERE question_set_id=$1) FROM question_sets WHERE id=$1 FOR UPDATE`, id).Scan(&s.LineageID, &s.Version, &s.Published, &s.Archived, &s.Kind, &s.Skill, &s.LessonID, &s.ModuleID, &s.HasAttempts)
	return s, err
}

func (r *dbRepository) UpdateSet(ctx context.Context, id uuid.UUID, v SetInput) error {
	_, err := r.Exec(ctx, `UPDATE question_sets SET kind=$2,module_id=$3,lesson_id=$4,title=$5,skill=$6,passing_score=$7,total_questions=$8,time_limit_seconds=$9,max_attempts=$10,cooldown_minutes=$11,shuffle_questions=$12,order_index=$13 WHERE id=$1`, id, v.Kind, v.ModuleID, v.LessonID, v.Title, v.Skill, v.PassingScore, v.TotalQuestions, v.TimeLimitSeconds, v.MaxAttempts, v.CooldownMinutes, v.ShuffleQuestions, v.OrderIndex)
	return err
}

func (r *dbRepository) DeleteQuestions(ctx context.Context, id uuid.UUID) error {
	_, err := r.Exec(ctx, `DELETE FROM questions WHERE question_set_id=$1`, id)
	return err
}

func (r *dbRepository) WithQuestions(ctx context.Context, live json.RawMessage, id uuid.UUID) (json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(live, &fields); err != nil {
		return nil, err
	}
	var questions json.RawMessage
	err := r.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(q) || jsonb_build_object('options',COALESCE((SELECT jsonb_agg(to_jsonb(o) ORDER BY o.order_index) FROM question_options o WHERE o.question_id=q.id),'[]'::jsonb)) ORDER BY q.order_index),'[]'::jsonb) FROM questions q WHERE q.question_set_id=$1`, id).Scan(&questions)
	if err != nil {
		return nil, err
	}
	fields["questions"] = questions
	return json.Marshal(fields)
}

func (r *dbRepository) InsertSet(ctx context.Context, id, lineage uuid.UUID, version int, v SetInput) error {
	_, err := r.Exec(ctx, `INSERT INTO question_sets(id,lineage_id,version,kind,module_id,lesson_id,title,skill,passing_score,total_questions,time_limit_seconds,max_attempts,cooldown_minutes,shuffle_questions,is_published,order_index) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,false,$15)`, id, lineage, version, v.Kind, v.ModuleID, v.LessonID, v.Title, v.Skill, v.PassingScore, v.TotalQuestions, v.TimeLimitSeconds, v.MaxAttempts, v.CooldownMinutes, v.ShuffleQuestions, v.OrderIndex)
	if err != nil {
		return err
	}
	return r.InsertQuestions(ctx, id, v)
}

func (r *dbRepository) InsertQuestions(ctx context.Context, id uuid.UUID, v SetInput) error {
	for i, q := range v.Questions {
		qid := uuid.New()
		_, err := r.Exec(ctx, `INSERT INTO questions(id,question_set_id,question_type,skill,prompt_text,stimulus_text,stimulus_media_url,explanation,points,order_index,stroke_template_id,show_writing_guide) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, qid, id, q.QuestionType, v.Skill, q.PromptText, q.StimulusText, q.StimulusMediaURL, q.Explanation, q.Points, i, q.StrokeTemplateID, q.ShowWritingGuide)
		if err != nil {
			return err
		}
		for j, o := range q.Options {
			_, err = r.Exec(ctx, `INSERT INTO question_options(question_id,label,media_url,is_correct,order_index) VALUES($1,$2,$3,$4,$5)`, qid, o.Label, o.MediaURL, o.IsCorrect, j)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *dbRepository) Template(ctx context.Context, id uuid.UUID, lock bool) (StrokeTemplateDetail, error) {
	query := `SELECT id,script,character,romaji,source_version,stroke_data FROM public.kana_stroke_templates WHERE id=$1`
	if lock {
		query += ` FOR UPDATE`
	}
	var item StrokeTemplateDetail
	err := r.QueryRow(ctx, query, id).Scan(&item.ID, &item.Script, &item.Character, &item.Romaji, &item.SourceVersion, &item.StrokeData)
	item.StrokeCount = len(item.StrokeData.Strokes)
	return item, err
}

func (r *dbRepository) InsertTemplate(ctx context.Context, item StrokeTemplateDetail) error {
	strokeJSON, err := json.Marshal(item.StrokeData)
	if err != nil {
		return err
	}
	_, err = r.Exec(ctx, `INSERT INTO public.kana_stroke_templates(id,script,character,romaji,source_version,stroke_data) VALUES($1,$2,$3,$4,$5,$6::jsonb)`,
		item.ID, item.Script, item.Character, item.Romaji, item.SourceVersion, string(strokeJSON))
	return err
}

func (r *dbRepository) UpdateTemplate(ctx context.Context, item StrokeTemplateDetail) error {
	strokeJSON, err := json.Marshal(item.StrokeData)
	if err != nil {
		return err
	}
	_, err = r.Exec(ctx, `UPDATE public.kana_stroke_templates SET script=$2,character=$3,romaji=$4,source_version=$5,stroke_data=$6::jsonb WHERE id=$1`,
		item.ID, item.Script, item.Character, item.Romaji, item.SourceVersion, string(strokeJSON))
	return err
}

// CatalogReader exposes only the administrative catalog projection to other modules.
type CatalogReader interface {
	CatalogQuestionSets(context.Context) ([]json.RawMessage, error)
}

func (r *dbRepository) CatalogQuestionSets(ctx context.Context) ([]json.RawMessage, error) {
	return r.rows(ctx, `SELECT to_jsonb(t) FROM public.question_sets t ORDER BY t.order_index,t.created_at,t.id`)
}
