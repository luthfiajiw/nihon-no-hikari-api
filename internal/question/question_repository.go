package question

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"nihon-no-hikari-api/pkg/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	ContentRepository
	ListQuestionSets(ctx context.Context, lessonID, userID uuid.UUID) ([]QuestionSet, error)
	ListModuleExams(ctx context.Context, moduleID, userID uuid.UUID) ([]QuestionSet, error)
	GetQuestionSetDetail(ctx context.Context, questionSetID, userID uuid.UUID) (*QuestionSetDetail, error)
	StartAttempt(ctx context.Context, questionSetID, userID uuid.UUID) (*Attempt, error)
	AbandonAttempt(ctx context.Context, questionSetID, attemptID, userID uuid.UUID) error
	SubmitAttempt(ctx context.Context, questionSetID, attemptID, userID uuid.UUID, req SubmitAttemptRequest) (*AttemptResult, error)
}

func (r *dbRepository) GetQuestionSetDetail(ctx context.Context, questionSetID, userID uuid.UUID) (*QuestionSetDetail, error) {
	const query = `
		SELECT
			qs.id,
			qs.lesson_id,
			qs.module_id,
			qs.kind,
			qs.title,
			qs.order_index,
			qs.skill,
			qs.passing_score,
			LEAST(
				COALESCE(qs.total_questions::integer, (SELECT COUNT(*)::integer FROM questions q WHERE q.question_set_id = qs.id)),
				(SELECT COUNT(*)::integer FROM questions q WHERE q.question_set_id = qs.id)
			),
			qs.time_limit_seconds,
			qs.max_attempts,
			qs.cooldown_minutes,
			qs.shuffle_questions,
			EXISTS (
				SELECT 1 FROM attempts a
				JOIN question_sets attempted_set ON attempted_set.id = a.question_set_id
				WHERE attempted_set.lineage_id = qs.lineage_id AND a.user_id = $2 AND a.is_passed = true
			),
			(
				SELECT COUNT(*)::integer FROM attempts a
				WHERE a.question_set_id = qs.id AND a.user_id = $2 AND a.status <> 'abandoned'
			)
		FROM question_sets qs
		LEFT JOIN lessons ls ON ls.id = qs.lesson_id
		JOIN modules m ON m.id = COALESCE(qs.module_id, ls.module_id)
		JOIN courses c ON c.id = m.course_id
		WHERE qs.id = $1
			AND qs.is_published = true
			AND qs.archived_at IS NULL
			AND c.is_published = true
			AND m.is_published = true
			AND (qs.kind = 'final_exam' OR ls.is_published = true)
	`

	detail := &QuestionSetDetail{}
	err := r.pool.QueryRow(ctx, query, questionSetID, userID).Scan(
		&detail.ID,
		&detail.LessonID,
		&detail.ModuleID,
		&detail.Kind,
		&detail.Title,
		&detail.OrderIndex,
		&detail.Skill,
		&detail.PassingScore,
		&detail.QuestionCount,
		&detail.TimeLimitSeconds,
		&detail.MaxAttempts,
		&detail.CooldownMinutes,
		&detail.ShuffleQuestions,
		&detail.IsPassed,
		&detail.AttemptsUsed,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, utils.ErrQuestionSetNotFound
	}
	if err != nil {
		return nil, err
	}

	detail.Questions, err = loadQuestionSetQuestions(ctx, r.pool, questionSetID, detail.ShuffleQuestions)
	if err != nil {
		return nil, err
	}
	return detail, nil
}

type dbRepository struct {
	query dbQuerier
	pool  *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &dbRepository{pool: pool, query: pool}
}

func (r *dbRepository) ListQuestionSets(ctx context.Context, lessonID, userID uuid.UUID) ([]QuestionSet, error) {
	const lessonQuery = `
		SELECT EXISTS (
			SELECT 1
			FROM lessons ls
			JOIN modules m ON m.id = ls.module_id
			JOIN courses c ON c.id = m.course_id
			WHERE ls.id = $1
				AND c.is_published = true
				AND m.is_published = true
				AND ls.is_published = true
		)
	`

	var lessonExists bool
	if err := r.pool.QueryRow(ctx, lessonQuery, lessonID).Scan(&lessonExists); err != nil {
		return nil, err
	}
	if !lessonExists {
		return nil, utils.ErrLessonNotFound
	}

	const query = `
		SELECT
			qs.id,
			qs.lesson_id,
			qs.module_id,
			qs.kind,
			qs.title,
			qs.order_index,
			qs.skill,
			qs.passing_score,
			LEAST(COALESCE(qs.total_questions::integer, COUNT(q.id)::integer), COUNT(q.id)::integer)::integer,
			qs.time_limit_seconds,
			qs.max_attempts,
			qs.cooldown_minutes,
			qs.shuffle_questions,
			EXISTS (
				SELECT 1 FROM attempts a JOIN question_sets old_set ON old_set.id=a.question_set_id
				WHERE old_set.lineage_id = qs.lineage_id AND a.user_id = $2 AND a.is_passed = true
			),
			(
				SELECT COUNT(*)::integer FROM attempts a
				WHERE a.question_set_id = qs.id AND a.user_id = $2 AND a.status <> 'abandoned'
			)
		FROM question_sets qs
		LEFT JOIN questions q ON q.question_set_id = qs.id
		WHERE qs.lesson_id = $1
			AND qs.kind = 'practice'
			AND qs.is_published = true
			AND qs.archived_at IS NULL
		GROUP BY qs.id
		ORDER BY qs.order_index, qs.id
	`

	rows, err := r.pool.Query(ctx, query, lessonID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sets := make([]QuestionSet, 0)
	for rows.Next() {
		var set QuestionSet
		if err := rows.Scan(
			&set.ID,
			&set.LessonID,
			&set.ModuleID,
			&set.Kind,
			&set.Title,
			&set.OrderIndex,
			&set.Skill,
			&set.PassingScore,
			&set.QuestionCount,
			&set.TimeLimitSeconds,
			&set.MaxAttempts,
			&set.CooldownMinutes,
			&set.ShuffleQuestions,
			&set.IsPassed,
			&set.AttemptsUsed,
		); err != nil {
			return nil, err
		}
		sets = append(sets, set)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return sets, nil
}

func (r *dbRepository) ListModuleExams(ctx context.Context, moduleID, userID uuid.UUID) ([]QuestionSet, error) {
	const query = `
		SELECT qs.id,qs.lesson_id,qs.module_id,qs.kind,qs.title,qs.order_index,qs.skill,
			qs.passing_score,LEAST(COALESCE(qs.total_questions::integer,COUNT(q.id)::integer),COUNT(q.id)::integer)::integer,
			qs.time_limit_seconds,qs.max_attempts,qs.cooldown_minutes,qs.shuffle_questions,
			EXISTS(SELECT 1 FROM attempts a JOIN question_sets old_set ON old_set.id=a.question_set_id
				WHERE old_set.lineage_id=qs.lineage_id AND a.user_id=$2 AND a.is_passed),
			(SELECT COUNT(*)::integer FROM attempts a WHERE a.question_set_id=qs.id AND a.user_id=$2 AND a.status<>'abandoned')
		FROM question_sets qs
		JOIN modules m ON m.id=qs.module_id
		JOIN courses c ON c.id=m.course_id
		LEFT JOIN questions q ON q.question_set_id=qs.id
		WHERE qs.module_id=$1 AND qs.kind='final_exam' AND qs.is_published AND qs.archived_at IS NULL
			AND m.is_published AND c.is_published
		GROUP BY qs.id ORDER BY qs.order_index,qs.id`
	rows, err := r.pool.Query(ctx, query, moduleID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sets := make([]QuestionSet, 0)
	for rows.Next() {
		var s QuestionSet
		if err := rows.Scan(&s.ID, &s.LessonID, &s.ModuleID, &s.Kind, &s.Title, &s.OrderIndex, &s.Skill, &s.PassingScore, &s.QuestionCount, &s.TimeLimitSeconds, &s.MaxAttempts, &s.CooldownMinutes, &s.ShuffleQuestions, &s.IsPassed, &s.AttemptsUsed); err != nil {
			return nil, err
		}
		sets = append(sets, s)
	}
	return sets, rows.Err()
}

func (r *dbRepository) StartAttempt(ctx context.Context, questionSetID, userID uuid.UUID) (*Attempt, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const setQuery = `
		SELECT
			qs.id,
			qs.lesson_id,
			qs.module_id,
			qs.kind,
			qs.title,
			qs.order_index,
			qs.skill,
			qs.passing_score,
			qs.total_questions,
			qs.time_limit_seconds,
			qs.max_attempts,
			qs.cooldown_minutes,
			qs.shuffle_questions
		FROM question_sets qs
		LEFT JOIN lessons ls ON ls.id = qs.lesson_id
		JOIN modules m ON m.id = COALESCE(qs.module_id,ls.module_id)
		JOIN courses c ON c.id = m.course_id
		WHERE qs.id = $1
			AND qs.is_published = true
			AND qs.archived_at IS NULL
			AND c.is_published = true
			AND m.is_published = true
			AND (qs.kind='final_exam' OR ls.is_published = true)
		FOR UPDATE OF qs
	`

	var set QuestionSet
	var configuredQuestionCount *int16
	err = tx.QueryRow(ctx, setQuery, questionSetID).Scan(
		&set.ID,
		&set.LessonID,
		&set.ModuleID,
		&set.Kind,
		&set.Title,
		&set.OrderIndex,
		&set.Skill,
		&set.PassingScore,
		&configuredQuestionCount,
		&set.TimeLimitSeconds,
		&set.MaxAttempts,
		&set.CooldownMinutes,
		&set.ShuffleQuestions,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, utils.ErrQuestionSetNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := tx.QueryRow(ctx, `SELECT COUNT(*)::integer FROM questions WHERE question_set_id = $1`, questionSetID).Scan(&set.QuestionCount); err != nil {
		return nil, err
	}
	if configuredQuestionCount != nil && int32(*configuredQuestionCount) < set.QuestionCount {
		set.QuestionCount = int32(*configuredQuestionCount)
	}
	if set.QuestionCount <= 0 {
		return nil, utils.ErrQuestionSetEmpty
	}
	if (set.TimeLimitSeconds != nil && *set.TimeLimitSeconds < 0) ||
		(set.MaxAttempts != nil && *set.MaxAttempts <= 0) || set.CooldownMinutes < 0 {
		return nil, utils.ErrInvalidQuestionConfiguration
	}

	const prerequisiteQuery = `
		SELECT NOT EXISTS (
			SELECT 1
			FROM lesson_prerequisites lp
			LEFT JOIN user_lesson_progress progress
				ON progress.lesson_id = lp.required_lesson_id
				AND progress.user_id = $2
			WHERE lp.lesson_id = $1
				AND COALESCE(progress.status::text, '') <> 'completed'
		)
	`
	var prerequisitesCompleted bool
	if set.Kind == QuestionSetKindPractice {
		if err := tx.QueryRow(ctx, prerequisiteQuery, set.LessonID, userID).Scan(&prerequisitesCompleted); err != nil {
			return nil, err
		}
	} else {
		const moduleReady = `SELECT
			NOT EXISTS(SELECT 1 FROM lessons l LEFT JOIN user_lesson_progress p ON p.lesson_id=l.id AND p.user_id=$2
				WHERE l.module_id=$1 AND l.is_published AND COALESCE(p.status::text,'')<>'completed')
			AND NOT EXISTS(SELECT 1 FROM module_prerequisites mp
				LEFT JOIN user_module_progress p ON p.module_id=mp.required_module_id AND p.user_id=$2
				WHERE mp.module_id=$1 AND COALESCE(p.status::text,'')<>'completed')`
		if err := tx.QueryRow(ctx, moduleReady, set.ModuleID, userID).Scan(&prerequisitesCompleted); err != nil {
			return nil, err
		}
	}
	if !prerequisitesCompleted {
		return nil, utils.ErrLessonPrerequisiteNotCompleted
	}

	const attemptStatsQuery = `
		SELECT
			COUNT(*) FILTER (WHERE status <> 'abandoned')::integer,
			MAX(submitted_at) FILTER (WHERE status <> 'abandoned'),
			COALESCE(bool_or(is_passed) FILTER (WHERE status <> 'abandoned'), false),
			COALESCE(MAX(attempt_number), 0)::smallint
		FROM attempts
		WHERE user_id = $1
			AND question_set_id = $2
	`
	var attemptCount int32
	var lastSubmittedAt *time.Time
	var lastAttemptNumber int16
	if err := tx.QueryRow(ctx, attemptStatsQuery, userID, questionSetID).Scan(&attemptCount, &lastSubmittedAt, &set.IsPassed, &lastAttemptNumber); err != nil {
		return nil, err
	}
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM attempts a JOIN question_sets old_set ON old_set.id=a.question_set_id JOIN question_sets current_set ON current_set.lineage_id=old_set.lineage_id WHERE current_set.id=$1 AND a.user_id=$2 AND a.is_passed)`, questionSetID, userID).Scan(&set.IsPassed); err != nil {
		return nil, err
	}
	set.AttemptsUsed = attemptCount
	if set.MaxAttempts != nil && attemptCount >= int32(*set.MaxAttempts) {
		return nil, utils.ErrAttemptLimitReached
	}
	if lastSubmittedAt != nil && set.CooldownMinutes > 0 {
		nextAttemptAt := lastSubmittedAt.Add(time.Duration(set.CooldownMinutes) * time.Minute)
		if time.Now().Before(nextAttemptAt) {
			return nil, utils.ErrAttemptCooldown
		}
	}

	questions, err := loadAttemptQuestions(ctx, tx, questionSetID, set.QuestionCount, set.ShuffleQuestions)
	if err != nil {
		return nil, err
	}
	if len(questions) == 0 {
		return nil, utils.ErrQuestionSetEmpty
	}
	if set.Skill != nil {
		for _, item := range questions {
			if item.Skill != *set.Skill {
				return nil, utils.ErrInvalidQuestionConfiguration
			}
		}
	}

	const insertAttemptQuery = `
		INSERT INTO attempts (user_id, question_set_id, attempt_number)
		VALUES ($1, $2, $3)
		RETURNING id, status, started_at
	`
	attempt := &Attempt{
		Number:      lastAttemptNumber + 1,
		QuestionSet: set,
		Questions:   questions,
	}
	if err := tx.QueryRow(ctx, insertAttemptQuery, userID, questionSetID, attempt.Number).Scan(
		&attempt.ID,
		&attempt.Status,
		&attempt.StartedAt,
	); err != nil {
		return nil, err
	}
	attempt.QuestionSet.AttemptsUsed++

	for _, item := range questions {
		snapshot, err := encodeWritingSnapshot(item)
		if err != nil {
			return nil, err
		}
		var snapshotJSON any
		if snapshot != nil {
			snapshotJSON = string(snapshot)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO attempt_answers (attempt_id, question_id, writing_guide_snapshot) VALUES ($1, $2, $3::jsonb)`, attempt.ID, item.ID, snapshotJSON); err != nil {
			return nil, err
		}
	}

	if set.TimeLimitSeconds != nil && *set.TimeLimitSeconds > 0 {
		expiresAt := attempt.StartedAt.Add(time.Duration(*set.TimeLimitSeconds) * time.Second)
		attempt.ExpiresAt = &expiresAt
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return attempt, nil
}

func (r *dbRepository) AbandonAttempt(ctx context.Context, questionSetID, attemptID, userID uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const attemptQuery = `
		SELECT a.status
		FROM attempts a
		JOIN question_sets qs ON qs.id = a.question_set_id
		WHERE a.id = $1
			AND a.user_id = $2
			AND qs.id = $3
		FOR UPDATE OF a
	`
	var status string
	err = tx.QueryRow(ctx, attemptQuery, attemptID, userID, questionSetID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return utils.ErrAttemptNotFound
	}
	if err != nil {
		return err
	}
	if status == "submitted" {
		return utils.ErrAttemptAlreadySubmitted
	}
	if status == "in_progress" {
		if _, err := tx.Exec(ctx, `UPDATE attempts SET status = 'abandoned' WHERE id = $1`, attemptID); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func loadAttemptQuestions(ctx context.Context, tx pgx.Tx, questionSetID uuid.UUID, limit int32, shuffle bool) ([]Question, error) {
	const query = `
		SELECT q.id, q.question_type, q.skill, q.prompt_text, q.stimulus_text, q.stimulus_media_url,
			q.points, q.order_index, t.character, t.romaji, t.stroke_data,
			CASE WHEN q.question_type = 'stroke_writing' THEN q.show_writing_guide END
		FROM questions q
		LEFT JOIN kana_stroke_templates t ON t.id = q.stroke_template_id
		WHERE q.question_set_id = $1
		ORDER BY
			CASE WHEN $3 THEN random() ELSE q.order_index::double precision END,
			q.order_index,
			q.id
		LIMIT $2
	`
	rows, err := tx.Query(ctx, query, questionSetID, limit, shuffle)
	if err != nil {
		return nil, err
	}
	questions := make([]Question, 0, limit)
	for rows.Next() {
		var item Question
		var character, romaji *string
		var strokeData []byte
		if err := rows.Scan(
			&item.ID,
			&item.QuestionType,
			&item.Skill,
			&item.PromptText,
			&item.StimulusText,
			&item.StimulusMediaURL,
			&item.Points,
			&item.OrderIndex,
			&character,
			&romaji,
			&strokeData,
			&item.ShowWritingGuide,
		); err != nil {
			return nil, err
		}
		if err := assignWritingGuide(&item, character, romaji, strokeData); err != nil {
			return nil, err
		}
		questions = append(questions, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for index := range questions {
		if !questions[index].Skill.IsSupported() {
			return nil, utils.ErrUnsupportedQuestionSkill
		}
		questions[index].Options, err = loadQuestionOptions(ctx, tx, questions[index].ID)
		if err != nil {
			return nil, err
		}
	}
	return questions, nil
}

type rowsQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func loadQuestionSetQuestions(ctx context.Context, querier rowsQuerier, questionSetID uuid.UUID, shuffle bool) ([]Question, error) {
	const query = `
		SELECT q.id, q.question_type, q.skill, q.prompt_text, q.stimulus_text, q.stimulus_media_url,
			q.points, q.order_index, t.character, t.romaji, t.stroke_data,
			CASE WHEN q.question_type = 'stroke_writing' THEN q.show_writing_guide END
		FROM questions q
		LEFT JOIN kana_stroke_templates t ON t.id = q.stroke_template_id
		WHERE q.question_set_id = $1
		ORDER BY
			CASE WHEN $2 THEN random() ELSE q.order_index::double precision END,
			q.order_index,
			q.id
	`
	rows, err := querier.Query(ctx, query, questionSetID, shuffle)
	if err != nil {
		return nil, err
	}

	questions := make([]Question, 0)
	for rows.Next() {
		var item Question
		var character, romaji *string
		var strokeData []byte
		if err := rows.Scan(
			&item.ID,
			&item.QuestionType,
			&item.Skill,
			&item.PromptText,
			&item.StimulusText,
			&item.StimulusMediaURL,
			&item.Points,
			&item.OrderIndex,
			&character,
			&romaji,
			&strokeData,
			&item.ShowWritingGuide,
		); err != nil {
			rows.Close()
			return nil, err
		}
		if err := assignWritingGuide(&item, character, romaji, strokeData); err != nil {
			rows.Close()
			return nil, err
		}
		questions = append(questions, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	for index := range questions {
		if !questions[index].Skill.IsSupported() {
			return nil, utils.ErrUnsupportedQuestionSkill
		}
		questions[index].Options, err = loadQuestionOptions(ctx, querier, questions[index].ID)
		if err != nil {
			return nil, err
		}
	}
	return questions, nil
}

func assignWritingGuide(item *Question, character, romaji *string, strokeData []byte) error {
	if item.QuestionType != QuestionTypeStrokeWriting {
		return nil
	}
	if character == nil || romaji == nil || len(strokeData) == 0 {
		return utils.ErrInvalidQuestionConfiguration
	}
	guide := &WritingGuide{Character: *character, Romaji: *romaji}
	if err := json.Unmarshal(strokeData, guide); err != nil {
		return fmt.Errorf("decode writing guide: %w", err)
	}
	if err := validateWritingGuide(guide); err != nil {
		return err
	}
	item.WritingGuide = guide
	return nil
}

func loadQuestionOptions(ctx context.Context, querier rowsQuerier, questionID uuid.UUID) ([]QuestionOption, error) {
	const query = `
		SELECT id, label, media_url, order_index
		FROM question_options
		WHERE question_id = $1
		ORDER BY order_index, id
	`
	rows, err := querier.Query(ctx, query, questionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	options := make([]QuestionOption, 0)
	for rows.Next() {
		var option QuestionOption
		if err := rows.Scan(&option.ID, &option.Label, &option.MediaURL, &option.OrderIndex); err != nil {
			return nil, err
		}
		options = append(options, option)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return options, nil
}

type gradingQuestion struct {
	ID               uuid.UUID
	QuestionType     QuestionType
	Skill            Skill
	Explanation      *string
	Points           int16
	Guide            *WritingGuide
	ShowWritingGuide bool
}

func (r *dbRepository) SubmitAttempt(ctx context.Context, questionSetID, attemptID, userID uuid.UUID, req SubmitAttemptRequest) (*AttemptResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const attemptQuery = `
		SELECT a.status, a.started_at, qs.lesson_id, qs.module_id, qs.kind, qs.passing_score, qs.time_limit_seconds
		FROM attempts a
		JOIN question_sets qs ON qs.id = a.question_set_id
		WHERE a.id = $1
			AND a.user_id = $2
			AND qs.id = $3
		FOR UPDATE OF a
	`
	var status string
	var startedAt time.Time
	var lessonID, moduleID *uuid.UUID
	var kind QuestionSetKind
	var passingScore int16
	var timeLimitSeconds *int32
	err = tx.QueryRow(ctx, attemptQuery, attemptID, userID, questionSetID).Scan(
		&status,
		&startedAt,
		&lessonID,
		&moduleID,
		&kind,
		&passingScore,
		&timeLimitSeconds,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, utils.ErrAttemptNotFound
	}
	if err != nil {
		return nil, err
	}
	if status != "in_progress" {
		return nil, utils.ErrAttemptAlreadySubmitted
	}
	if timeLimitSeconds != nil && *timeLimitSeconds > 0 && time.Now().After(startedAt.Add(time.Duration(*timeLimitSeconds)*time.Second)) {
		return nil, utils.ErrAttemptExpired
	}

	const questionsQuery = `
		SELECT q.id, q.question_type, q.skill, q.explanation, q.points,
			aa.writing_guide_snapshot, q.show_writing_guide
		FROM attempt_answers aa
		JOIN questions q ON q.id = aa.question_id
		WHERE aa.attempt_id = $1
		ORDER BY q.order_index, q.id
	`
	rows, err := tx.Query(ctx, questionsQuery, attemptID)
	if err != nil {
		return nil, err
	}
	questions := make([]gradingQuestion, 0)
	for rows.Next() {
		var item gradingQuestion
		var snapshot []byte
		if err := rows.Scan(&item.ID, &item.QuestionType, &item.Skill, &item.Explanation, &item.Points, &snapshot, &item.ShowWritingGuide); err != nil {
			rows.Close()
			return nil, err
		}
		if item.QuestionType == QuestionTypeStrokeWriting {
			guide, err := decodeWritingSnapshot(snapshot)
			if err != nil {
				rows.Close()
				return nil, err
			}
			item.Guide = guide
		}
		questions = append(questions, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	if len(questions) == 0 {
		return nil, utils.ErrInvalidAttempt
	}
	answerByQuestion := make(map[uuid.UUID]SubmitAnswerRequest, len(req.Answers))
	for _, answer := range req.Answers {
		answerByQuestion[answer.QuestionID] = answer
	}
	questionIDs := make(map[uuid.UUID]struct{}, len(questions))
	for _, item := range questions {
		questionIDs[item.ID] = struct{}{}
	}
	for questionID := range answerByQuestion {
		if _, exists := questionIDs[questionID]; !exists {
			return nil, utils.ErrInvalidAttempt
		}
	}

	results := make([]AnswerResult, 0, len(questions))
	var totalPoints int32
	var earnedPoints int32
	skillTotals := map[Skill]int32{SkillReading: 0, SkillWriting: 0}
	skillEarned := map[Skill]int32{SkillReading: 0, SkillWriting: 0}
	for _, item := range questions {
		answer := answerByQuestion[item.ID]
		if !item.Skill.IsSupported() || item.Points <= 0 ||
			(item.QuestionType != QuestionTypeMultipleChoice && item.QuestionType != QuestionTypeStrokeWriting) {
			return nil, utils.ErrInvalidQuestionConfiguration
		}

		isCorrect := false
		var accuracy *float64
		if answer.hasResponse() {
			var err error
			if item.QuestionType == QuestionTypeStrokeWriting {
				if answer.SelectedOptionID != nil || item.Guide == nil {
					return nil, utils.ErrInvalidAttempt
				}
				isCorrect, accuracy, err = gradeStrokeWriting(*item.Guide, item.ShowWritingGuide, answer.normalizedStrokeInput())
				if err != nil {
					return nil, fmt.Errorf("%w: %v", utils.ErrInvalidAttempt, err)
				}
			} else {
				isCorrect, err = gradeAnswer(ctx, tx, item, answer)
			}
			if err != nil {
				return nil, err
			}
		}
		earned := int16(0)
		if isCorrect {
			earned = item.Points
		}
		totalPoints += int32(item.Points)
		earnedPoints += int32(earned)
		skillTotals[item.Skill] += int32(item.Points)
		skillEarned[item.Skill] += int32(earned)

		const updateAnswerQuery = `
			UPDATE attempt_answers
			SET selected_option_id = $3,
				stroke_input = $4,
				accuracy = $5,
				is_correct = $6,
				earned_points = $7,
				answered_at = now()
			WHERE attempt_id = $1 AND question_id = $2
		`
		if _, err := tx.Exec(ctx, updateAnswerQuery, attemptID, item.ID, answer.SelectedOptionID, answer.normalizedStrokeInput(), accuracy, isCorrect, earned); err != nil {
			return nil, err
		}

		results = append(results, AnswerResult{
			QuestionID:   item.ID,
			Skill:        item.Skill,
			IsCorrect:    isCorrect,
			EarnedPoints: earned,
			MaxPoints:    item.Points,
			Explanation:  item.Explanation,
			Accuracy:     accuracy,
		})
	}
	if totalPoints <= 0 || totalPoints > 32767 || earnedPoints > 32767 {
		return nil, utils.ErrInvalidQuestionConfiguration
	}

	score := int16((earnedPoints*100 + totalPoints/2) / totalPoints)
	isPassed := score >= passingScore
	skillResults := make([]SkillResult, 0, 2)
	for _, skill := range []Skill{SkillReading, SkillWriting} {
		if skillTotals[skill] == 0 {
			continue
		}
		skillScore := int16((skillEarned[skill]*100 + skillTotals[skill]/2) / skillTotals[skill])
		skillPassed := skillScore >= passingScore
		if !skillPassed {
			isPassed = false
		}
		skillResults = append(skillResults, SkillResult{
			Skill:        skill,
			Score:        skillScore,
			EarnedPoints: int16(skillEarned[skill]),
			TotalPoints:  int16(skillTotals[skill]),
			IsPassed:     skillPassed,
		})
	}
	const finishAttemptQuery = `
		UPDATE attempts
		SET status = 'submitted',
			score = $2,
			total_points = $3,
			earned_points = $4,
			is_passed = $5,
			submitted_at = now()
		WHERE id = $1
		RETURNING submitted_at
	`
	var submittedAt time.Time
	if err := tx.QueryRow(ctx, finishAttemptQuery, attemptID, score, int16(totalPoints), int16(earnedPoints), isPassed).Scan(&submittedAt); err != nil {
		return nil, err
	}

	lessonStatus, moduleStatus := "", ""
	if kind == QuestionSetKindPractice {
		const lessonPassedQuery = `
		SELECT NOT EXISTS (
			SELECT 1
			FROM question_sets required_set
			WHERE required_set.lesson_id = $1
				AND required_set.kind = 'practice'
				AND required_set.is_published = true
				AND NOT EXISTS (
					SELECT 1
					FROM attempts passed_attempt
					JOIN question_sets attempted_set ON attempted_set.id=passed_attempt.question_set_id
					WHERE attempted_set.lineage_id=required_set.lineage_id
						AND passed_attempt.user_id = $2
						AND passed_attempt.is_passed = true
				)
		)
	`
		var lessonPassed bool
		if err := tx.QueryRow(ctx, lessonPassedQuery, lessonID, userID).Scan(&lessonPassed); err != nil {
			return nil, err
		}

		desiredStatus := "in_progress"
		if lessonPassed {
			desiredStatus = "completed"
		}
		const progressQuery = `
		INSERT INTO user_lesson_progress AS progress (user_id, lesson_id, status, completed_at, updated_at)
		VALUES (
			$1,
			$2,
			$3::progress_status,
			CASE WHEN $3 = 'completed' THEN now() END,
			now()
		)
		ON CONFLICT (user_id, lesson_id) DO UPDATE SET
			status = CASE
				WHEN progress.status = 'completed' THEN progress.status
				ELSE EXCLUDED.status
			END,
			completed_at = CASE
				WHEN progress.status = 'completed' THEN progress.completed_at
				WHEN EXCLUDED.status = 'completed' THEN now()
				ELSE NULL
			END,
			updated_at = now()
		RETURNING status::text
	`
		if err := tx.QueryRow(ctx, progressQuery, userID, lessonID, desiredStatus).Scan(&lessonStatus); err != nil {
			return nil, err
		}
	} else {
		const modulePassedQuery = `SELECT NOT EXISTS(
			SELECT 1 FROM question_sets required_set
			WHERE required_set.module_id=$1 AND required_set.kind='final_exam' AND required_set.is_published AND required_set.archived_at IS NULL
			AND NOT EXISTS(SELECT 1 FROM attempts passed_attempt
			JOIN question_sets attempted_set ON attempted_set.id=passed_attempt.question_set_id
			WHERE attempted_set.lineage_id=required_set.lineage_id AND passed_attempt.user_id=$2 AND passed_attempt.is_passed))`
		var modulePassed bool
		if err := tx.QueryRow(ctx, modulePassedQuery, moduleID, userID).Scan(&modulePassed); err != nil {
			return nil, err
		}
		moduleStatus = "in_progress"
		if modulePassed {
			moduleStatus = "completed"
		}
		const moduleProgressQuery = `INSERT INTO user_module_progress AS p(user_id,module_id,status,best_score,unlocked_at,completed_at,updated_at)
			VALUES($1,$2,$3::progress_status,$4,now(),CASE WHEN $3='completed' THEN now() END,now())
			ON CONFLICT(user_id,module_id) DO UPDATE SET status=CASE WHEN p.status='completed' THEN p.status ELSE EXCLUDED.status END,
			best_score=GREATEST(COALESCE(p.best_score,0),EXCLUDED.best_score),
			completed_at=CASE WHEN p.status='completed' THEN p.completed_at ELSE EXCLUDED.completed_at END,updated_at=now()
			RETURNING status::text`
		if err := tx.QueryRow(ctx, moduleProgressQuery, userID, moduleID, moduleStatus, score).Scan(&moduleStatus); err != nil {
			return nil, err
		}
	}

	result := &AttemptResult{
		AttemptID:    attemptID,
		Score:        score,
		EarnedPoints: int16(earnedPoints),
		TotalPoints:  int16(totalPoints),
		PassingScore: passingScore,
		IsPassed:     isPassed,
		LessonStatus: lessonStatus,
		ModuleStatus: moduleStatus,
		SubmittedAt:  submittedAt,
		Skills:       skillResults,
		Answers:      results,
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

func gradeAnswer(ctx context.Context, tx pgx.Tx, item gradingQuestion, answer SubmitAnswerRequest) (bool, error) {
	if item.QuestionType != QuestionTypeMultipleChoice {
		return false, utils.ErrInvalidQuestionConfiguration
	}
	if answer.SelectedOptionID == nil || len(answer.normalizedStrokeInput()) != 0 {
		return false, utils.ErrInvalidAttempt
	}

	const query = `SELECT is_correct FROM question_options WHERE id = $1 AND question_id = $2`
	var isCorrect bool
	if err := tx.QueryRow(ctx, query, *answer.SelectedOptionID, item.ID).Scan(&isCorrect); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, utils.ErrInvalidAttempt
		}
		return false, err
	}
	return isCorrect, nil
}
