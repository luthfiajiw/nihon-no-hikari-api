package question

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"nihon-no-hikari-api/internal/testdb"
	"nihon-no-hikari-api/pkg/kanastroke"
	"nihon-no-hikari-api/pkg/utils"

	"github.com/google/uuid"
)

func TestWritingSnapshotDatabase(t *testing.T) {
	pool := testdb.New(t, 12)
	ctx := context.Background()
	user, set, qid, template := testdb.Fixture(t, pool)
	legacy := uuid.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO attempts(id,user_id,question_set_id,attempt_number) VALUES($1,$2,$3,1)`, legacy, user, set)
	exec(`INSERT INTO attempt_answers(attempt_id,question_id) VALUES($1,$2)`, legacy, qid)
	// A second unpublished reading set verifies choice answers remain NULL.
	reading, choice, choiceAttempt := uuid.New(), uuid.New(), uuid.New()
	exec(`INSERT INTO question_sets(id,lineage_id,lesson_id,title,kind,skill,passing_score,is_published,order_index)
		SELECT $1,$1,lesson_id,'Reading','practice','reading',100,false,1 FROM question_sets WHERE id=$2`, reading, set)
	exec(`INSERT INTO questions(id,question_set_id,question_type,skill,prompt_text,stimulus_text,points,order_index) VALUES($1,$2,'multiple_choice','reading','Read','あ',1,0)`, choice, reading)
	exec(`INSERT INTO attempts(id,user_id,question_set_id,attempt_number) VALUES($1,$2,$3,1)`, choiceAttempt, user, reading)
	exec(`INSERT INTO attempt_answers(attempt_id,question_id) VALUES($1,$2)`, choiceAttempt, choice)
	testdb.Migrate(t, pool, 13, "up")
	var raw []byte
	if err := pool.QueryRow(ctx, `SELECT writing_guide_snapshot FROM attempt_answers WHERE attempt_id=$1`, legacy).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	oldGuide, err := decodeWritingSnapshot(raw)
	if err != nil || oldGuide.Character != "あ" {
		t.Fatalf("backfill: %v", err)
	}
	var null bool
	if err := pool.QueryRow(ctx, `SELECT writing_guide_snapshot IS NULL FROM attempt_answers WHERE attempt_id=$1`, choiceAttempt).Scan(&null); err != nil || !null {
		t.Fatal("choice snapshot not null", err)
	}
	repo := NewRepository(pool)
	old, err := repo.StartAttempt(ctx, set, user)
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT writing_guide_snapshot FROM attempt_answers WHERE attempt_id=$1`, old.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	responseGuide, _ := json.Marshal(old.Questions[0].WritingGuide)
	var stored, returned any
	json.Unmarshal(raw, &stored)
	json.Unmarshal(responseGuide, &returned)
	a, _ := json.Marshal(stored)
	b, _ := json.Marshal(returned)
	if string(a) != string(b) {
		t.Fatal("stored snapshot differs from start response")
	}
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 109 109"><g id="kvg:StrokePaths_030ab"><path id="kvg:030ab-s1" d="M10 10 C20 10 30 20 40 20"/></g></svg>`
	data, err := kanastroke.Convert(svg, "katakana", "カ")
	if err != nil {
		t.Fatal(err)
	}
	dataJSON, _ := json.Marshal(data)
	exec(`UPDATE kana_stroke_templates SET script='katakana',character='カ',romaji='ka',stroke_data=$2::jsonb WHERE id=$1`, template, string(dataJSON))
	newAttempt, err := repo.StartAttempt(ctx, set, user)
	if err != nil {
		t.Fatal(err)
	}
	if newAttempt.Questions[0].WritingGuide.Character != "カ" || newAttempt.Questions[0].WritingGuide.Romaji != "ka" || len(newAttempt.Questions[0].WritingGuide.Strokes) != 1 {
		t.Fatal("new attempt did not use current template")
	}
	for _, attempt := range []*Attempt{old, newAttempt} {
		input, _ := json.Marshal(referenceInput(*attempt.Questions[0].WritingGuide))
		strokeInput := json.RawMessage(input)
		result, err := repo.SubmitAttempt(ctx, set, attempt.ID, user, SubmitAttemptRequest{Answers: []SubmitAnswerRequest{{QuestionID: qid, StrokeInput: &strokeInput}}})
		if err != nil || result.Score != 100 {
			t.Fatalf("grade after edit: %+v %v", result, err)
		}
	}
	input, _ := json.Marshal(referenceInput(*oldGuide))
	strokeInput := json.RawMessage(input)
	if result, err := repo.SubmitAttempt(ctx, set, legacy, user, SubmitAttemptRequest{Answers: []SubmitAnswerRequest{{QuestionID: qid, StrokeInput: &strokeInput}}}); err != nil || result.Score != 100 {
		t.Fatalf("backfilled attempt: %+v %v", result, err)
	}
	broken, err := repo.StartAttempt(ctx, set, user)
	if err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE attempt_answers SET writing_guide_snapshot=NULL WHERE attempt_id=$1`, broken.ID)
	if _, err := repo.SubmitAttempt(ctx, set, broken.ID, user, SubmitAttemptRequest{}); !errors.Is(err, utils.ErrInvalidQuestionConfiguration) {
		t.Fatalf("missing snapshot fell back to current template: %v", err)
	}
	var score int
	if err := pool.QueryRow(ctx, `SELECT score FROM attempts WHERE id=$1`, old.ID).Scan(&score); err != nil || score != 100 {
		t.Fatal("old score changed", err)
	}
	if err := repo.AbandonAttempt(ctx, set, broken.ID, user); err != nil {
		t.Fatal(err)
	}
	testdb.Migrate(t, pool, 13, "down")
	if _, err := pool.Exec(ctx, `UPDATE kana_stroke_templates SET romaji='changed' WHERE id=$1`, template); err == nil {
		t.Fatal("down migration failed to restore immutability")
	}
}
