package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type optionInput struct {
	Label     string  `json:"label"`
	MediaURL  *string `json:"media_url"`
	IsCorrect bool    `json:"is_correct"`
}

type questionInput struct {
	QuestionType     string        `json:"question_type"`
	PromptText       string        `json:"prompt_text"`
	StimulusText     *string       `json:"stimulus_text"`
	StimulusMediaURL *string       `json:"stimulus_media_url"`
	Explanation      *string       `json:"explanation"`
	Points           int16         `json:"points"`
	StrokeTemplateID *uuid.UUID    `json:"stroke_template_id"`
	ShowWritingGuide bool          `json:"show_writing_guide"`
	Options          []optionInput `json:"options"`
}

type setInput struct {
	Kind             string          `json:"kind"`
	ModuleID         *uuid.UUID      `json:"module_id"`
	LessonID         *uuid.UUID      `json:"lesson_id"`
	Title            string          `json:"title"`
	Skill            string          `json:"skill"`
	PassingScore     int16           `json:"passing_score"`
	TotalQuestions   *int16          `json:"total_questions"`
	TimeLimitSeconds *int32          `json:"time_limit_seconds"`
	MaxAttempts      *int16          `json:"max_attempts"`
	CooldownMinutes  int32           `json:"cooldown_minutes"`
	ShuffleQuestions bool            `json:"shuffle_questions"`
	OrderIndex       int16           `json:"order_index"`
	Questions        []questionInput `json:"questions"`
}

func validMediaURL(raw *string) bool {
	if raw == nil || *raw == "" {
		return true
	}
	u, err := url.Parse(*raw)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != ""
}

func sameUUID(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func parseSet(raw []byte, publish bool) (setInput, error) {
	var v setInput
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, err
	}
	if v.Kind != "practice" && v.Kind != "final_exam" {
		return v, errors.New("jenis set tidak valid")
	}
	if (v.Kind == "practice" && (v.LessonID == nil || v.ModuleID != nil)) || (v.Kind == "final_exam" && (v.ModuleID == nil || v.LessonID != nil)) {
		return v, errors.New("tempat question set tidak valid")
	}
	if v.Skill != "reading" && v.Skill != "writing" {
		return v, errors.New("skill harus reading atau writing")
	}
	if strings.TrimSpace(v.Title) == "" || len(v.Title) > 160 || v.PassingScore < 0 || v.PassingScore > 100 || v.OrderIndex < 0 || v.CooldownMinutes < 0 {
		return v, errors.New("konfigurasi question set tidak valid")
	}
	if v.TimeLimitSeconds != nil && *v.TimeLimitSeconds < 0 {
		return v, errors.New("batas waktu tidak valid")
	}
	if v.MaxAttempts != nil && *v.MaxAttempts <= 0 {
		return v, errors.New("batas attempt tidak valid")
	}
	if v.TotalQuestions != nil && (*v.TotalQuestions <= 0 || (publish && int(*v.TotalQuestions) > len(v.Questions))) {
		return v, errors.New("jumlah soal dipilih melebihi soal tersedia")
	}
	if publish && len(v.Questions) == 0 {
		return v, errors.New("tambahkan setidaknya satu soal sebelum terbit")
	}
	if !publish {
		return v, nil
	}
	for _, q := range v.Questions {
		if strings.TrimSpace(q.PromptText) == "" || q.Points <= 0 || !validMediaURL(q.StimulusMediaURL) {
			return v, errors.New("prompt, poin, atau media soal tidak valid")
		}
		if (q.StimulusText == nil || strings.TrimSpace(*q.StimulusText) == "") && (q.StimulusMediaURL == nil || *q.StimulusMediaURL == "") && q.StrokeTemplateID == nil {
			return v, errors.New("soal memerlukan stimulus atau template")
		}
		if v.Skill == "reading" {
			if q.QuestionType != "multiple_choice" || q.StrokeTemplateID != nil || len(q.Options) < 2 {
				return v, errors.New("soal reading memerlukan minimal dua pilihan")
			}
			correct := 0
			for _, o := range q.Options {
				if strings.TrimSpace(o.Label) == "" || !validMediaURL(o.MediaURL) {
					return v, errors.New("pilihan jawaban tidak valid")
				}
				if o.IsCorrect {
					correct++
				}
			}
			if correct != 1 {
				return v, errors.New("pilih tepat satu jawaban benar")
			}
		} else if q.QuestionType != "stroke_writing" || q.StrokeTemplateID == nil || len(q.Options) != 0 {
			return v, errors.New("soal writing memerlukan template goresan")
		}
	}
	return v, nil
}

func (h *Handler) withQuestions(ctx context.Context, live json.RawMessage, id uuid.UUID) (json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(live, &fields); err != nil {
		return nil, err
	}
	var questions json.RawMessage
	err := h.pool.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(q) || jsonb_build_object('options',COALESCE((SELECT jsonb_agg(to_jsonb(o) ORDER BY o.order_index) FROM question_options o WHERE o.question_id=q.id),'[]'::jsonb)) ORDER BY q.order_index),'[]'::jsonb) FROM questions q WHERE q.question_set_id=$1`, id).Scan(&questions)
	if err != nil {
		return nil, err
	}
	fields["questions"] = questions
	return json.Marshal(fields)
}

func insertSet(ctx context.Context, tx pgx.Tx, id, lineage uuid.UUID, version int, v setInput) error {
	_, err := tx.Exec(ctx, `INSERT INTO question_sets(id,lineage_id,version,kind,module_id,lesson_id,title,skill,passing_score,total_questions,time_limit_seconds,max_attempts,cooldown_minutes,shuffle_questions,is_published,order_index) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,false,$15)`, id, lineage, version, v.Kind, v.ModuleID, v.LessonID, v.Title, v.Skill, v.PassingScore, v.TotalQuestions, v.TimeLimitSeconds, v.MaxAttempts, v.CooldownMinutes, v.ShuffleQuestions, v.OrderIndex)
	if err != nil {
		return err
	}
	return insertQuestions(ctx, tx, id, v)
}

func insertQuestions(ctx context.Context, tx pgx.Tx, id uuid.UUID, v setInput) error {
	for i, q := range v.Questions {
		qid := uuid.New()
		_, err := tx.Exec(ctx, `INSERT INTO questions(id,question_set_id,question_type,skill,prompt_text,stimulus_text,stimulus_media_url,explanation,points,order_index,stroke_template_id,show_writing_guide) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, qid, id, q.QuestionType, v.Skill, q.PromptText, q.StimulusText, q.StimulusMediaURL, q.Explanation, q.Points, i, q.StrokeTemplateID, q.ShowWritingGuide)
		if err != nil {
			return err
		}
		for j, o := range q.Options {
			_, err = tx.Exec(ctx, `INSERT INTO question_options(question_id,label,media_url,is_correct,order_index) VALUES($1,$2,$3,$4,$5)`, qid, o.Label, o.MediaURL, o.IsCorrect, j)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *Handler) createQuestionSet(c fiber.Ctx) error {
	v, err := parseSet(c.Body(), false)
	if err != nil {
		return bad(c, err.Error())
	}
	id := uuid.New()
	tx, err := h.pool.Begin(c.Context())
	if err != nil {
		return serverError(c, err)
	}
	defer tx.Rollback(c.Context())
	initial := v
	initial.Questions = nil
	if err := insertSet(c.Context(), tx, id, id, 1, initial); err != nil {
		return serverError(c, err)
	}
	payload, _ := json.Marshal(v)
	if _, err := tx.Exec(c.Context(), `INSERT INTO admin_drafts(resource_type,resource_id,payload) VALUES('question_set',$1,$2::jsonb)`, id, string(payload)); err != nil {
		return serverError(c, err)
	}
	if err := tx.Commit(c.Context()); err != nil {
		return serverError(c, err)
	}
	return c.Status(201).JSON(fiber.Map{"success": true, "id": id})
}

func (h *Handler) updateQuestionSet(c fiber.Ctx) error {
	id, err := resourceID(c)
	if err != nil {
		return bad(c, "ID tidak valid")
	}
	v, err := parseSet(c.Body(), false)
	if err != nil {
		return bad(c, err.Error())
	}
	tx, err := h.pool.Begin(c.Context())
	if err != nil {
		return serverError(c, err)
	}
	defer tx.Rollback(c.Context())
	var published, hasAttempts bool
	var originalKind string
	var originalSkill *string
	var originalLessonID, originalModuleID *uuid.UUID
	if err := tx.QueryRow(c.Context(), `SELECT is_published,EXISTS(SELECT 1 FROM attempts WHERE question_set_id=$1),kind,skill,lesson_id,module_id FROM question_sets WHERE id=$1 FOR UPDATE`, id).Scan(&published, &hasAttempts, &originalKind, &originalSkill, &originalLessonID, &originalModuleID); err != nil {
		return serverError(c, err)
	}
	if published {
		if v.Kind != originalKind || (originalSkill != nil && v.Skill != *originalSkill) || !sameUUID(v.LessonID, originalLessonID) || !sameUUID(v.ModuleID, originalModuleID) {
			return bad(c, "jenis, skill, dan tempat question set yang sudah terbit tidak dapat diubah")
		}
	} else if hasAttempts {
		return c.Status(409).JSON(fiber.Map{"success": false, "error": "versi lama dengan attempt tidak dapat diubah"})
	}
	payload, _ := json.Marshal(v)
	_, err = tx.Exec(c.Context(), `INSERT INTO admin_drafts(resource_type,resource_id,payload) VALUES('question_set',$1,$2::jsonb) ON CONFLICT(resource_type,resource_id) DO UPDATE SET payload=EXCLUDED.payload,updated_at=now()`, id, string(payload))
	if err != nil {
		return serverError(c, err)
	}
	if err := tx.Commit(c.Context()); err != nil {
		return serverError(c, err)
	}
	return c.JSON(fiber.Map{"success": true, "draft": published})
}

func (h *Handler) publishQuestionSet(c fiber.Ctx) error {
	id, err := resourceID(c)
	if err != nil {
		return bad(c, "ID tidak valid")
	}
	tx, err := h.pool.Begin(c.Context())
	if err != nil {
		return serverError(c, err)
	}
	defer tx.Rollback(c.Context())
	var lineage uuid.UUID
	var version int
	var published, archived bool
	var originalKind string
	var originalSkill *string
	var originalLessonID, originalModuleID *uuid.UUID
	if err := tx.QueryRow(c.Context(), `SELECT lineage_id,version,is_published,archived_at IS NOT NULL,kind,skill,lesson_id,module_id FROM question_sets WHERE id=$1 FOR UPDATE`, id).Scan(&lineage, &version, &published, &archived, &originalKind, &originalSkill, &originalLessonID, &originalModuleID); err != nil {
		return serverError(c, err)
	}
	if archived {
		return bad(c, "pulihkan question set sebelum diterbitkan")
	}
	var raw []byte
	err = tx.QueryRow(c.Context(), `SELECT payload FROM admin_drafts WHERE resource_type='question_set' AND resource_id=$1`, id).Scan(&raw)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return serverError(c, err)
	}
	if published && errors.Is(err, pgx.ErrNoRows) {
		return c.JSON(fiber.Map{"success": true, "id": id})
	}
	if !published && errors.Is(err, pgx.ErrNoRows) {
		var hasAttempts bool
		if err := tx.QueryRow(c.Context(), `SELECT EXISTS(SELECT 1 FROM attempts WHERE question_set_id=$1)`, id).Scan(&hasAttempts); err != nil {
			return serverError(c, err)
		}
		if hasAttempts {
			if _, err := tx.Exec(c.Context(), `UPDATE question_sets SET is_published=true WHERE id=$1`, id); err != nil {
				return serverError(c, err)
			}
			if err := tx.Commit(c.Context()); err != nil {
				return serverError(c, err)
			}
			return c.JSON(fiber.Map{"success": true, "id": id})
		}
	}
	if !published && errors.Is(err, pgx.ErrNoRows) {
		var live json.RawMessage
		if err := tx.QueryRow(c.Context(), `SELECT to_jsonb(s) FROM question_sets s WHERE id=$1`, id).Scan(&live); err != nil {
			return serverError(c, err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(live, &fields); err != nil {
			return serverError(c, err)
		}
		var qjson json.RawMessage
		if err := tx.QueryRow(c.Context(), `SELECT COALESCE(jsonb_agg(to_jsonb(q) || jsonb_build_object('options',COALESCE((SELECT jsonb_agg(to_jsonb(o) ORDER BY o.order_index) FROM question_options o WHERE o.question_id=q.id),'[]'::jsonb)) ORDER BY q.order_index),'[]'::jsonb) FROM questions q WHERE q.question_set_id=$1`, id).Scan(&qjson); err != nil {
			return serverError(c, err)
		}
		fields["questions"] = qjson
		raw, _ = json.Marshal(fields)
	}
	v, err := parseSet(raw, true)
	if err != nil {
		return bad(c, err.Error())
	}
	if published {
		if v.Kind != originalKind || (originalSkill != nil && v.Skill != *originalSkill) || !sameUUID(v.LessonID, originalLessonID) || !sameUUID(v.ModuleID, originalModuleID) {
			return bad(c, "jenis, skill, dan tempat question set yang sudah terbit tidak dapat diubah")
		}
		if _, err := tx.Exec(c.Context(), `UPDATE question_sets SET is_published=false WHERE id=$1`, id); err != nil {
			return serverError(c, err)
		}
		newID := uuid.New()
		if err := insertSet(c.Context(), tx, newID, lineage, version+1, v); err != nil {
			return serverError(c, err)
		}
		id = newID
	} else {
		var hasAttempts bool
		if err := tx.QueryRow(c.Context(), `SELECT EXISTS(SELECT 1 FROM attempts WHERE question_set_id=$1)`, id).Scan(&hasAttempts); err != nil {
			return serverError(c, err)
		}
		if hasAttempts {
			return c.Status(409).JSON(fiber.Map{"success": false, "error": "versi lama dengan attempt tidak dapat diterbitkan ulang"})
		}
		if _, err := tx.Exec(c.Context(), `UPDATE question_sets SET kind=$2,module_id=$3,lesson_id=$4,title=$5,skill=$6,passing_score=$7,total_questions=$8,time_limit_seconds=$9,max_attempts=$10,cooldown_minutes=$11,shuffle_questions=$12,order_index=$13 WHERE id=$1`, id, v.Kind, v.ModuleID, v.LessonID, v.Title, v.Skill, v.PassingScore, v.TotalQuestions, v.TimeLimitSeconds, v.MaxAttempts, v.CooldownMinutes, v.ShuffleQuestions, v.OrderIndex); err != nil {
			return serverError(c, err)
		}
		if _, err := tx.Exec(c.Context(), `DELETE FROM questions WHERE question_set_id=$1`, id); err != nil {
			return serverError(c, err)
		}
		if err := insertQuestions(c.Context(), tx, id, v); err != nil {
			return serverError(c, err)
		}
	}
	if _, err := tx.Exec(c.Context(), `UPDATE question_sets SET is_published=true WHERE id=$1`, id); err != nil {
		return serverError(c, err)
	}
	if _, err := tx.Exec(c.Context(), `DELETE FROM admin_drafts WHERE resource_type='question_set' AND resource_id=$1`, c.Params("id")); err != nil {
		return serverError(c, err)
	}
	if err := tx.Commit(c.Context()); err != nil {
		return serverError(c, err)
	}
	return c.JSON(fiber.Map{"success": true, "id": id})
}

func (h *Handler) validateTemplate(ctx context.Context, id uuid.UUID) error {
	var exists bool
	if err := h.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM kana_stroke_templates WHERE id=$1)`, id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("template tidak ditemukan")
	}
	return nil
}
