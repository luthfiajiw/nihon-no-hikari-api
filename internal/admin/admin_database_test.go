package admin

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"nihon-no-hikari-api/internal/testdb"
	"nihon-no-hikari-api/pkg/db"

	"github.com/google/uuid"
)

func TestAdminContentDatabase(t *testing.T) {
	pool := testdb.New(t, 13)
	ctx := context.Background()
	user, setID, _, template := testdb.Fixture(t, pool)
	repository := NewRepository(pool)
	svc := NewService(db.NewTx(pool), repository)
	var lesson, module, course, level uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT l.id,l.module_id,m.course_id,c.level_id FROM question_sets s JOIN lessons l ON l.id=s.lesson_id JOIN modules m ON m.id=l.module_id JOIN courses c ON c.id=m.course_id WHERE s.id=$1`, setID).Scan(&lesson, &module, &course, &level); err != nil {
		t.Fatal(err)
	}
	exec := func(tb testing.TB, sql string, args ...any) {
		tb.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			tb.Fatal(err)
		}
	}
	count := func(tb testing.TB, sql string, args ...any) int {
		tb.Helper()
		var n int
		if err := pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
			tb.Fatal(err)
		}
		return n
	}

	t.Run("catalog and response projections", func(t *testing.T) {
		all, err := svc.Catalog(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(all.Data.Courses) == 0 || len(all.Data.Modules) == 0 || len(all.Data.Lessons) == 0 || len(all.Data.QuestionSets) == 0 {
			t.Fatalf("missing catalog data: %+v", all)
		}
		filtered, err := svc.Catalog(ctx, &course)
		if err != nil {
			t.Fatal(err)
		}
		if filtered.Data.Courses == nil || len(filtered.Data.Courses) != 0 || filtered.Data.QuestionSets == nil || len(filtered.Data.QuestionSets) != 0 {
			t.Fatal("filtered catalog arrays changed")
		}
		var summary map[string]json.RawMessage
		if err := json.Unmarshal(filtered.Data.Lessons[0], &summary); err != nil {
			t.Fatal(err)
		}
		if len(summary) != 7 || summary["content"] != nil {
			t.Fatalf("lesson projection changed: %v", summary)
		}
		listed, err := svc.ListContent(ctx, "lesson")
		if err != nil {
			t.Fatal(err)
		}
		var item map[string]json.RawMessage
		if err := json.Unmarshal(listed.Data[0], &item); err != nil {
			t.Fatal(err)
		}
		if item["content"] != nil {
			t.Fatal("lesson list includes content")
		}
		levels, err := svc.Levels(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var expected json.RawMessage
		if err := pool.QueryRow(ctx, `SELECT jsonb_agg(to_jsonb(t) ORDER BY order_index) FROM levels t`).Scan(&expected); err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(levels.Data)
		if err != nil {
			t.Fatal(err)
		}
		var want, got any
		if err := json.Unmarshal(expected, &want); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(want, got) {
			t.Fatalf("level wire shape changed: %s != %s", expected, raw)
		}
	})

	t.Run("live draft prerequisites and extensions", func(t *testing.T) {
		input := coreInput{ModuleID: module, Slug: "contract-lesson", Title: "Initial", OrderIndex: 10, LessonPrerequisites: []uuid.UUID{lesson}, Content: json.RawMessage(`{"content":"<p>safe</p><script>bad()</script>","tips":" tip ","custom":{"enabled":true}}`)}
		created, err := svc.CreateContent(ctx, "lesson", input)
		if err != nil {
			t.Fatal(err)
		}
		input.Title = "Unpublished edit"
		if result, err := svc.UpdateContent(ctx, "lesson", created.ID, input); err != nil || result.Draft {
			t.Fatalf("%+v %v", result, err)
		}
		if _, err := svc.PublishContent(ctx, "lesson", created.ID); err != nil {
			t.Fatal(err)
		}
		input.Title = "Published draft"
		input.LessonPrerequisites = nil
		if result, err := svc.UpdateContent(ctx, "lesson", created.ID, input); err != nil || !result.Draft {
			t.Fatalf("%+v %v", result, err)
		}
		detail, err := svc.Detail(ctx, "lesson", created.ID)
		if err != nil {
			t.Fatal(err)
		}
		var live struct {
			Title   string                     `json:"title"`
			Content map[string]json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(detail.Data, &live); err != nil {
			t.Fatal(err)
		}
		if live.Title != "Unpublished edit" || string(live.Content["custom"]) != `{"enabled": true}` && string(live.Content["custom"]) != `{"enabled":true}` || detail.Draft == nil {
			t.Fatalf("live/draft contract: %s %s", detail.Data, detail.Draft)
		}
		if _, err := svc.PublishContent(ctx, "lesson", created.ID); err != nil {
			t.Fatal(err)
		}
		if n := count(t, `SELECT count(*) FROM lesson_prerequisites WHERE lesson_id=$1`, created.ID); n != 1 {
			t.Fatalf("nil cleared prerequisites: %d", n)
		}
		input.LessonPrerequisites = []uuid.UUID{}
		if _, err := svc.UpdateContent(ctx, "lesson", created.ID, input); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.PublishContent(ctx, "lesson", created.ID); err != nil {
			t.Fatal(err)
		}
		if n := count(t, `SELECT count(*) FROM lesson_prerequisites WHERE lesson_id=$1`, created.ID); n != 0 {
			t.Fatalf("[] did not clear prerequisites: %d", n)
		}
		detail, err = svc.Detail(ctx, "lesson", created.ID)
		if err != nil || detail.Draft != nil {
			t.Fatalf("draft not removed: %s %v", detail.Draft, err)
		}
		exec(t, `INSERT INTO user_lesson_progress(user_id,lesson_id,status) VALUES($1,$2,'completed')`, user, created.ID)
		removed, err := svc.RemoveContent(ctx, "lesson", created.ID)
		if err != nil || !removed.Archived {
			t.Fatalf("%+v %v", removed, err)
		}
		if _, err := svc.PublishContent(ctx, "lesson", created.ID); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("published archive: %v", err)
		}
		if _, err := svc.RestoreContent(ctx, "lesson", created.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.RestoreContent(ctx, "lesson", created.ID); !errors.Is(err, ErrNotFound) || err.Error() != "arsip tidak ditemukan" {
			t.Fatalf("restore contract: %v", err)
		}
	})

	t.Run("publish rollback and question set lineage", func(t *testing.T) {
		v := setInput{Kind: "practice", LessonID: &lesson, Title: "Updated writing", Skill: "writing", PassingScore: 100, Questions: []questionInput{{QuestionType: "stroke_writing", PromptText: "Write a", Points: 1, StrokeTemplateID: &template}}}
		if _, err := svc.UpdateQuestionSet(ctx, setID, v); err != nil {
			t.Fatal(err)
		}
		exec(t, `CREATE FUNCTION fail_admin_publish() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected draft delete failure'; END $$;
       CREATE TRIGGER fail_admin_publish BEFORE DELETE ON admin_drafts FOR EACH ROW EXECUTE FUNCTION fail_admin_publish();`)
		if _, err := svc.PublishContent(ctx, "question_set", setID); err == nil {
			t.Fatal("injected failure was ignored")
		}
		if n := count(t, `SELECT count(*) FROM question_sets WHERE lineage_id=$1 AND is_published`, setID); n != 1 {
			t.Fatal("active version was lost")
		}
		if n := count(t, `SELECT count(*) FROM question_sets WHERE lineage_id=$1`, setID); n != 1 {
			t.Fatal("failed new version survived rollback")
		}
		if n := count(t, `SELECT count(*) FROM admin_drafts WHERE resource_type='question_set' AND resource_id=$1`, setID); n != 1 {
			t.Fatal("draft was lost")
		}
		exec(t, `DROP TRIGGER fail_admin_publish ON admin_drafts;DROP FUNCTION fail_admin_publish();`)
		published, err := svc.PublishContent(ctx, "question_set", setID)
		if err != nil || published.ID == nil || *published.ID == setID {
			t.Fatalf("%+v %v", published, err)
		}
		if n := count(t, `SELECT count(*) FROM question_sets WHERE id=$1 AND lineage_id=$2 AND version=2 AND is_published`, *published.ID, setID); n != 1 {
			t.Fatal("lineage/version changed")
		}
		if n := count(t, `SELECT count(*) FROM questions WHERE question_set_id=$1`, setID); n != 1 {
			t.Fatal("old questions were replaced")
		}
		if n := count(t, `SELECT count(*) FROM admin_drafts WHERE resource_id=$1`, setID); n != 0 {
			t.Fatal("original draft was not deleted")
		}
	})

	t.Run("blocking relation archives via savepoint", func(t *testing.T) {
		created, err := svc.CreateContent(ctx, "course", coreInput{LevelID: level, Slug: "blocked-course", Title: "Blocked"})
		if err != nil {
			t.Fatal(err)
		}
		exec(t, `CREATE TABLE admin_test_blocker(id uuid REFERENCES courses(id))`)
		exec(t, `INSERT INTO admin_test_blocker(id) VALUES($1)`, created.ID)
		result, err := svc.RemoveContent(ctx, "course", created.ID)
		if err != nil || !result.Archived {
			t.Fatalf("%+v %v", result, err)
		}
		if n := count(t, `SELECT count(*) FROM courses WHERE id=$1 AND archived_at IS NOT NULL AND NOT is_published`, created.ID); n != 1 {
			t.Fatal("savepoint did not recover delete")
		}
	})

	t.Run("deferred commit conflict is classified and rolled back", func(t *testing.T) {
		id := uuid.New()
		var callbackErr error
		err := svc.(*service).execTx(ctx, func(r Repository) error {
			_, callbackErr = r.(*dbRepository).Exec(ctx, `INSERT INTO modules(id,course_id,slug,title,order_index) VALUES($1,$2,'deferred-conflict','Deferred',0)`, id, course)
			return callbackErr
		})
		if callbackErr != nil || !errors.Is(err, ErrConflict) {
			t.Fatalf("callback=%v commit=%v", callbackErr, err)
		}
		if n := count(t, `SELECT count(*) FROM modules WHERE id=$1`, id); n != 0 {
			t.Fatal("failed commit persisted")
		}
	})

	t.Run("transaction repository holds row locks", func(t *testing.T) {
		locked, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
		go func() {
			done <- svc.(*service).execTx(ctx, func(r Repository) error {
				if _, err := r.ContentState(ctx, "lesson", lesson); err != nil {
					close(locked)
					return err
				}
				close(locked)
				<-release
				return nil
			})
		}()
		<-locked
		limited, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
		err := svc.(*service).execTx(limited, func(r Repository) error { _, err := r.ContentState(limited, "lesson", lesson); return err })
		cancel()
		close(release)
		if first := <-done; first != nil {
			t.Fatal(first)
		}
		if err == nil {
			t.Fatal("second transaction bypassed row lock")
		}
	})
}
