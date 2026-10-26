package admin

import (
	"context"
	"encoding/json"

	"nihon-no-hikari-api/internal/admin/model"

	"github.com/google/uuid"
)

func (r *dbRepository) QuestionSetState(ctx context.Context, id uuid.UUID) (model.QuestionSetState, error) {
	var s model.QuestionSetState
	err := r.QueryRow(ctx, `SELECT lineage_id,version,is_published,archived_at IS NOT NULL,kind,skill,lesson_id,module_id,EXISTS(SELECT 1 FROM attempts WHERE question_set_id=$1) FROM question_sets WHERE id=$1 FOR UPDATE`, id).Scan(&s.LineageID, &s.Version, &s.Published, &s.Archived, &s.Kind, &s.Skill, &s.LessonID, &s.ModuleID, &s.HasAttempts)
	return s, err
}

func (r *dbRepository) UpdateSet(ctx context.Context, id uuid.UUID, v setInput) error {
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

func (r *dbRepository) InsertSet(ctx context.Context, id, lineage uuid.UUID, version int, v setInput) error {
	_, err := r.Exec(ctx, `INSERT INTO question_sets(id,lineage_id,version,kind,module_id,lesson_id,title,skill,passing_score,total_questions,time_limit_seconds,max_attempts,cooldown_minutes,shuffle_questions,is_published,order_index) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,false,$15)`, id, lineage, version, v.Kind, v.ModuleID, v.LessonID, v.Title, v.Skill, v.PassingScore, v.TotalQuestions, v.TimeLimitSeconds, v.MaxAttempts, v.CooldownMinutes, v.ShuffleQuestions, v.OrderIndex)
	if err != nil {
		return err
	}
	return r.InsertQuestions(ctx, id, v)
}

func (r *dbRepository) InsertQuestions(ctx context.Context, id uuid.UUID, v setInput) error {
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
