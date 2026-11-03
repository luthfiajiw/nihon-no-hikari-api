package question

import (
	"context"
	"testing"

	"nihon-no-hikari-api/internal/testdb"

	"github.com/google/uuid"
)

func TestAttemptAnswerKeyDisclosureDatabase(t *testing.T) {
	pool := testdb.New(t, 13)
	ctx := context.Background()
	user, _, _, _ := testdb.Fixture(t, pool)
	repo := NewRepository(pool)

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}

	var lessonID, moduleID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id, module_id FROM lessons WHERE is_published = true LIMIT 1`).Scan(&lessonID, &moduleID); err != nil {
		t.Fatal(err)
	}

	practiceSetID := uuid.New()
	practiceQID := uuid.New()
	practiceOptCorrect := uuid.New()
	practiceOptWrong := uuid.New()

	exec(`INSERT INTO question_sets(id, lineage_id, lesson_id, title, kind, skill, passing_score, is_published, order_index, shuffle_questions)
		VALUES($1, $1, $2, 'Practice Reading', 'practice', 'reading', 80, true, 10, false)`,
		practiceSetID, lessonID)
	exec(`INSERT INTO questions(id, question_set_id, question_type, skill, prompt_text, stimulus_text, points, order_index, explanation)
		VALUES($1, $2, 'multiple_choice', 'reading', 'Pilih arti', 'あ', 1, 0, '  Penjelasan untuk karakter A  ')`,
		practiceQID, practiceSetID)
	exec(`INSERT INTO question_options(id, question_id, label, is_correct, order_index)
		VALUES($1, $2, 'a', true, 0), ($3, $2, 'i', false, 1)`,
		practiceOptCorrect, practiceQID, practiceOptWrong)

	t.Run("practice attempt discloses is_correct and explanation", func(t *testing.T) {
		attempt, err := repo.StartAttempt(ctx, practiceSetID, user)
		if err != nil {
			t.Fatalf("StartAttempt() error = %v", err)
		}
		if len(attempt.Questions) != 1 {
			t.Fatalf("expected 1 question, got %d", len(attempt.Questions))
		}
		q := attempt.Questions[0]
		if q.Explanation == nil || *q.Explanation != "Penjelasan untuk karakter A" {
			t.Fatalf("expected trimmed explanation 'Penjelasan untuk karakter A', got %#v", q.Explanation)
		}
		if len(q.Options) != 2 {
			t.Fatalf("expected 2 options, got %d", len(q.Options))
		}
		for _, opt := range q.Options {
			if opt.IsCorrect == nil {
				t.Fatalf("expected is_correct not nil for practice attempt option %v", opt.ID)
			}
			if opt.ID == practiceOptCorrect && !*opt.IsCorrect {
				t.Fatalf("expected is_correct = true for correct option, got false")
			}
			if opt.ID == practiceOptWrong && *opt.IsCorrect {
				t.Fatalf("expected is_correct = false for wrong option, got true")
			}
		}

		_ = repo.AbandonAttempt(ctx, practiceSetID, attempt.ID, user)
	})

	t.Run("practice get detail omits is_correct and explanation", func(t *testing.T) {
		detail, err := repo.GetQuestionSetDetail(ctx, practiceSetID, user)
		if err != nil {
			t.Fatalf("GetQuestionSetDetail() error = %v", err)
		}
		if len(detail.Questions) != 1 {
			t.Fatalf("expected 1 question, got %d", len(detail.Questions))
		}
		q := detail.Questions[0]
		if q.Explanation != nil {
			t.Fatalf("expected nil explanation on GetQuestionSetDetail, got %v", *q.Explanation)
		}
		for _, opt := range q.Options {
			if opt.IsCorrect != nil {
				t.Fatalf("expected nil is_correct on GetQuestionSetDetail, got %v", *opt.IsCorrect)
			}
		}
	})

	examSetID := uuid.New()
	examQID := uuid.New()
	examOptCorrect := uuid.New()
	examOptWrong := uuid.New()

	exec(`INSERT INTO question_sets(id, lineage_id, module_id, title, kind, skill, passing_score, is_published, order_index, shuffle_questions)
		VALUES($1, $1, $2, 'Final Exam Reading', 'final_exam', 'reading', 80, true, 20, false)`,
		examSetID, moduleID)
	exec(`INSERT INTO questions(id, question_set_id, question_type, skill, prompt_text, stimulus_text, points, order_index, explanation)
		VALUES($1, $2, 'multiple_choice', 'reading', 'Ujian arti', 'あ', 1, 0, 'Penjelasan ujian')`,
		examQID, examSetID)
	exec(`INSERT INTO question_options(id, question_id, label, is_correct, order_index)
		VALUES($1, $2, 'a', true, 0), ($3, $2, 'i', false, 1)`,
		examOptCorrect, examQID, examOptWrong)

	t.Run("final exam attempt omits is_correct and explanation", func(t *testing.T) {
		attempt, err := repo.StartAttempt(ctx, examSetID, user)
		if err != nil {
			t.Fatalf("StartAttempt() final_exam error = %v", err)
		}
		if len(attempt.Questions) != 1 {
			t.Fatalf("expected 1 question, got %d", len(attempt.Questions))
		}
		q := attempt.Questions[0]
		if q.Explanation != nil {
			t.Fatalf("expected nil explanation for final_exam attempt, got %v", *q.Explanation)
		}
		for _, opt := range q.Options {
			if opt.IsCorrect != nil {
				t.Fatalf("expected nil is_correct for final_exam attempt, got %v", *opt.IsCorrect)
			}
		}

		_ = repo.AbandonAttempt(ctx, examSetID, attempt.ID, user)
	})
}
