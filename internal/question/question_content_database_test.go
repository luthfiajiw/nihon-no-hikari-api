package question

import (
	"context"
	"testing"

	"nihon-no-hikari-api/internal/testdb"
	"nihon-no-hikari-api/pkg/db"

	"github.com/google/uuid"
)

func TestQuestionSetPublishDatabase(t *testing.T) {
	pool := testdb.New(t, 13)
	ctx := context.Background()
	_, setID, _, template := testdb.Fixture(t, pool)
	repository := NewRepository(pool)
	svc := NewService(db.NewTx(pool), repository)
	var lesson uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT lesson_id FROM question_sets WHERE id=$1`, setID).Scan(&lesson); err != nil {
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

	t.Run("publish rollback and question set lineage", func(t *testing.T) {
		v := SetInput{Kind: "practice", LessonID: &lesson, Title: "Updated writing", Skill: "writing", PassingScore: 100, Questions: []QuestionInput{{QuestionType: "stroke_writing", PromptText: "Write a", Points: 1, StrokeTemplateID: &template}}}
		if _, err := svc.UpdateQuestionSet(ctx, setID, v); err != nil {
			t.Fatal(err)
		}
		exec(t, `CREATE FUNCTION fail_admin_publish() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected draft delete failure'; END $$;
       CREATE TRIGGER fail_admin_publish BEFORE DELETE ON admin_drafts FOR EACH ROW EXECUTE FUNCTION fail_admin_publish();`)
		if _, err := svc.PublishQuestionSet(ctx, setID); err == nil {
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
		published, err := svc.PublishQuestionSet(ctx, setID)
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
}
