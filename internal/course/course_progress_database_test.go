package course

import (
	"context"
	"errors"
	"testing"

	"nihon-no-hikari-api/internal/course/model"
	"nihon-no-hikari-api/internal/testdb"
	"nihon-no-hikari-api/pkg/utils"

	"github.com/google/uuid"
)

func TestModuleAndLessonProgressSynchronizationDatabase(t *testing.T) {
	pool := testdb.New(t, 20)
	ctx := context.Background()
	userID, _, _, _ := testdb.Fixture(t, pool)
	repo := NewRepository(pool)

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}

	var courseID, module1ID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT m.course_id, m.id FROM modules m WHERE m.is_published = true LIMIT 1`).Scan(&courseID, &module1ID); err != nil {
		t.Fatal(err)
	}

	// Remove default fixture practice quiz from module 1's first lesson so we can test direct lesson completion.
	var lesson1AID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM lessons WHERE module_id = $1 AND is_published = true ORDER BY order_index ASC LIMIT 1`, module1ID).Scan(&lesson1AID); err != nil {
		t.Fatal(err)
	}
	exec(`DELETE FROM question_sets WHERE lesson_id = $1`, lesson1AID)

	// Add a second lesson in module 1.
	lesson1BID := uuid.New()
	exec(`INSERT INTO lessons(id, module_id, slug, title, order_index, is_entry, is_published, content)
		VALUES($1, $2, 'lesson-1b', 'Lesson 1B', 1, false, true, '{}')`, lesson1BID, module1ID)

	// Add module 2 depending on module 1 via module_prerequisites.
	module2ID := uuid.New()
	lesson2AID := uuid.New()
	exec(`INSERT INTO modules(id, course_id, slug, title, order_index, is_entry, is_published)
		VALUES($1, $2, 'module-2', 'Module 2', 1, false, true)`, module2ID, courseID)
	exec(`INSERT INTO module_prerequisites(module_id, required_module_id) VALUES($1, $2)`, module2ID, module1ID)
	exec(`INSERT INTO lessons(id, module_id, slug, title, order_index, is_entry, is_published, content)
		VALUES($1, $2, 'lesson-2a', 'Lesson 2A', 0, true, true, '{}')`, lesson2AID, module2ID)

	// Add module 3 (no prerequisite) with a published active final_exam.
	module3ID := uuid.New()
	lesson3AID := uuid.New()
	exam3ID := uuid.New()
	exec(`INSERT INTO modules(id, course_id, slug, title, order_index, is_entry, is_published)
		VALUES($1, $2, 'module-3', 'Module 3', 2, false, true)`, module3ID, courseID)
	exec(`INSERT INTO lessons(id, module_id, slug, title, order_index, is_entry, is_published, content)
		VALUES($1, $2, 'lesson-3a', 'Lesson 3A', 0, true, true, '{}')`, lesson3AID, module3ID)
	exec(`INSERT INTO question_sets(id, lineage_id, module_id, title, kind, skill, passing_score, is_published, order_index)
		VALUES($1, $1, $2, 'Final Exam 3', 'final_exam', 'reading', 80, true, 0)`, exam3ID, module3ID)

	// 1. ListLessons marks lessons in module 2 as locked when module_prerequisites are not completed.
	moduleLessons, err := repo.ListLessons(ctx, courseID, userID)
	if err != nil {
		t.Fatalf("ListLessons() error = %v", err)
	}
	statusByLesson := make(map[uuid.UUID]model.Status)
	for _, ml := range moduleLessons {
		for _, ls := range ml.Lessons {
			statusByLesson[ls.ID] = ls.Status
		}
	}
	if statusByLesson[lesson1AID] != model.StatusUnlocked || statusByLesson[lesson1BID] != model.StatusUnlocked {
		t.Fatalf("expected module 1 lessons unlocked, got 1A=%s 1B=%s", statusByLesson[lesson1AID], statusByLesson[lesson1BID])
	}
	if statusByLesson[lesson2AID] != model.StatusLocked {
		t.Fatalf("expected module 2 lesson locked due to module prerequisite, got %s", statusByLesson[lesson2AID])
	}

	// 2. UpsertLessonProgress returns ErrModulePrerequisiteNotCompleted when module prerequisites are not completed.
	inProgress := model.StatusInProgress
	if _, err := repo.UpsertLessonProgress(ctx, lesson2AID, userID, model.UpsertLessonProgressRequest{Status: &inProgress}); !errors.Is(err, utils.ErrModulePrerequisiteNotCompleted) {
		t.Fatalf("expected ErrModulePrerequisiteNotCompleted, got %v", err)
	}

	// 3. UpsertModuleProgress returns ErrModuleLessonsNotCompleted when trying to complete module with incomplete lessons.
	completed := model.StatusCompleted
	if _, err := repo.UpsertModuleProgress(ctx, courseID, module1ID, userID, model.UpsertModuleProgressRequest{Status: &completed}); !errors.Is(err, utils.ErrModuleLessonsNotCompleted) {
		t.Fatalf("expected ErrModuleLessonsNotCompleted, got %v", err)
	}

	// 4. UpsertLessonProgress sets module progress to in_progress while lessons remain incomplete.
	prog1A, err := repo.UpsertLessonProgress(ctx, lesson1AID, userID, model.UpsertLessonProgressRequest{Status: &inProgress})
	if err != nil {
		t.Fatalf("UpsertLessonProgress(1A in_progress) error = %v", err)
	}
	if prog1A.ModuleID != module1ID || prog1A.ModuleStatus != model.StatusInProgress {
		t.Fatalf("expected module 1 in_progress, got module=%s status=%s", prog1A.ModuleID, prog1A.ModuleStatus)
	}

	prog1ACompleted, err := repo.UpsertLessonProgress(ctx, lesson1AID, userID, model.UpsertLessonProgressRequest{Status: &completed})
	if err != nil {
		t.Fatalf("UpsertLessonProgress(1A completed) error = %v", err)
	}
	if prog1ACompleted.ModuleStatus != model.StatusInProgress {
		t.Fatalf("expected module 1 still in_progress with 1B incomplete, got %s", prog1ACompleted.ModuleStatus)
	}

	// 5. Completing the last lesson in module 1 (no final exam) auto-completes module 1.
	prog1BCompleted, err := repo.UpsertLessonProgress(ctx, lesson1BID, userID, model.UpsertLessonProgressRequest{Status: &completed})
	if err != nil {
		t.Fatalf("UpsertLessonProgress(1B completed) error = %v", err)
	}
	if prog1BCompleted.ModuleID != module1ID || prog1BCompleted.ModuleStatus != model.StatusCompleted {
		t.Fatalf("expected module 1 auto-completed, got module=%s status=%s", prog1BCompleted.ModuleID, prog1BCompleted.ModuleStatus)
	}

	// 6. ListLessons now unlocks lesson 2A and UpsertLessonProgress succeeds for lesson 2A.
	moduleLessonsAfter, err := repo.ListLessons(ctx, courseID, userID)
	if err != nil {
		t.Fatalf("ListLessons() after module 1 completion error = %v", err)
	}
	for _, ml := range moduleLessonsAfter {
		for _, ls := range ml.Lessons {
			if ls.ID == lesson2AID && ls.Status != model.StatusUnlocked {
				t.Fatalf("expected lesson 2A unlocked after module 1 completion, got %s", ls.Status)
			}
		}
	}
	prog2A, err := repo.UpsertLessonProgress(ctx, lesson2AID, userID, model.UpsertLessonProgressRequest{Status: &inProgress})
	if err != nil {
		t.Fatalf("UpsertLessonProgress(2A in_progress) error = %v", err)
	}
	if prog2A.ModuleID != module2ID || prog2A.ModuleStatus != model.StatusInProgress {
		t.Fatalf("expected module 2 in_progress, got module=%s status=%s", prog2A.ModuleID, prog2A.ModuleStatus)
	}

	// 7. Completing all lessons in module 3 keeps module 3 in_progress because a published active final_exam exists.
	prog3A, err := repo.UpsertLessonProgress(ctx, lesson3AID, userID, model.UpsertLessonProgressRequest{Status: &completed})
	if err != nil {
		t.Fatalf("UpsertLessonProgress(3A completed) error = %v", err)
	}
	if prog3A.ModuleID != module3ID || prog3A.ModuleStatus != model.StatusInProgress {
		t.Fatalf("expected module 3 in_progress when final_exam exists, got module=%s status=%s", prog3A.ModuleID, prog3A.ModuleStatus)
	}
}
