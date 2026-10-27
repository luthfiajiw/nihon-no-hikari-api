package question

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"nihon-no-hikari-api/pkg/kanastroke"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Embedding an unimplemented repository makes unexpected persistence calls fail.
type contentRepositoryStub struct {
	Repository
	active         ContentRepository
	boundTx        pgx.Tx
	state          func(context.Context, string, uuid.UUID) (ContentState, error)
	draft          func(context.Context, string, uuid.UUID) (json.RawMessage, error)
	saveDraft      func(context.Context, string, uuid.UUID, json.RawMessage) error
	deleteDraft    func(context.Context, string, uuid.UUID) error
	published      func(context.Context, string, uuid.UUID, bool) error
	setState       func(context.Context, uuid.UUID) (QuestionSetState, error)
	insertSet      func(context.Context, uuid.UUID, uuid.UUID, int, SetInput) error
	history        func(context.Context, string, uuid.UUID) (bool, error)
	deleteContent  func(context.Context, string, uuid.UUID) (bool, error)
	archive        func(context.Context, string, uuid.UUID) error
	template       func(context.Context, uuid.UUID, bool) (StrokeTemplateDetail, error)
	updateTemplate func(context.Context, StrokeTemplateDetail) error
}

type transactionToken struct{ pgx.Tx }

type transactionRunnerStub struct {
	tx        pgx.Tx
	calls     int
	beginErr  error
	commitErr error
}

func (r *transactionRunnerStub) Exec(ctx context.Context, fn func(pgx.Tx) error) error {
	r.calls++
	if r.beginErr != nil {
		return r.beginErr
	}
	if err := fn(r.tx); err != nil {
		return err
	}
	return r.commitErr
}

func newStubService(repository Repository) Service {
	return NewService(&transactionRunnerStub{}, repository)
}

func (r *contentRepositoryStub) UseTx(tx pgx.Tx) ContentRepository {
	r.boundTx = tx
	if r.active != nil {
		return r.active
	}
	return r
}

func (r *contentRepositoryStub) ContentState(c context.Context, k string, id uuid.UUID) (ContentState, error) {
	return r.state(c, k, id)
}

func (r *contentRepositoryStub) Draft(c context.Context, k string, id uuid.UUID) (json.RawMessage, error) {
	return r.draft(c, k, id)
}

func (r *contentRepositoryStub) SaveDraft(c context.Context, k string, id uuid.UUID, v json.RawMessage) error {
	return r.saveDraft(c, k, id, v)
}

func (r *contentRepositoryStub) DeleteDraft(c context.Context, k string, id uuid.UUID) error {
	return r.deleteDraft(c, k, id)
}

func (r *contentRepositoryStub) SetPublished(c context.Context, k string, id uuid.UUID, b bool) error {
	return r.published(c, k, id, b)
}

func (r *contentRepositoryStub) HasHistory(c context.Context, k string, id uuid.UUID) (bool, error) {
	return r.history(c, k, id)
}

func (r *contentRepositoryStub) DeleteContent(c context.Context, k string, id uuid.UUID) (bool, error) {
	return r.deleteContent(c, k, id)
}

func (r *contentRepositoryStub) Archive(c context.Context, k string, id uuid.UUID) error {
	return r.archive(c, k, id)
}

func (r *contentRepositoryStub) QuestionSetState(c context.Context, id uuid.UUID) (QuestionSetState, error) {
	return r.setState(c, id)
}

func (r *contentRepositoryStub) InsertSet(c context.Context, id, lineage uuid.UUID, version int, v SetInput) error {
	return r.insertSet(c, id, lineage, version, v)
}

func (r *contentRepositoryStub) Template(c context.Context, id uuid.UUID, b bool) (StrokeTemplateDetail, error) {
	return r.template(c, id, b)
}

func (r *contentRepositoryStub) UpdateTemplate(c context.Context, v StrokeTemplateDetail) error {
	return r.updateTemplate(c, v)
}

func validReadingSet(lesson uuid.UUID) SetInput {
	stimulus := "a"
	return SetInput{Kind: "practice", LessonID: &lesson, Title: "Reading", Skill: "reading", Questions: []QuestionInput{{QuestionType: "multiple_choice", PromptText: "Which?", StimulusText: &stimulus, Points: 1, Options: []OptionInput{{Label: "a", IsCorrect: true}, {Label: "b"}}}}}
}

func TestQuestionSetVersionKeepsLineageAndDeletesOriginalDraft(t *testing.T) {
	original, lineage, lesson := uuid.New(), uuid.New(), uuid.New()
	v := validReadingSet(lesson)
	raw, _ := json.Marshal(v)
	var created uuid.UUID
	var calls []string
	active := &contentRepositoryStub{
		setState: func(context.Context, uuid.UUID) (QuestionSetState, error) {
			return QuestionSetState{ContentState: ContentState{Published: true}, LineageID: lineage, Version: 4, Kind: v.Kind, Skill: &v.Skill, LessonID: v.LessonID}, nil
		},
		draft: func(context.Context, string, uuid.UUID) (json.RawMessage, error) { return raw, nil },
		published: func(_ context.Context, _ string, id uuid.UUID, value bool) error {
			if !value && id != original {
				t.Fatal("disabled wrong version")
			}
			if value && id != created {
				t.Fatal("published wrong version")
			}
			calls = append(calls, fmt.Sprint(value))
			return nil
		},
		insertSet: func(_ context.Context, id, gotLineage uuid.UUID, version int, _ SetInput) error {
			if id == original || gotLineage != lineage || version != 5 {
				t.Fatal("lineage/version changed")
			}
			created = id
			return nil
		},
		deleteDraft: func(_ context.Context, _ string, id uuid.UUID) error {
			if id != original {
				t.Fatal("deleted new version's draft")
			}
			calls = append(calls, "draft")
			return nil
		},
	}
	result, err := newStubService(&contentRepositoryStub{active: active}).PublishQuestionSet(context.Background(), original)
	if err != nil || result.ID == nil || *result.ID != created || fmt.Sprint(calls) != "[false true draft]" {
		t.Fatalf("%+v %v %v", result, calls, err)
	}
}

func TestQuestionSetAttemptAndPlacementRestrictions(t *testing.T) {
	lesson := uuid.New()
	v := validReadingSet(lesson)
	for _, tc := range []struct {
		name  string
		state QuestionSetState
		want  error
	}{
		{"attempted version", QuestionSetState{HasAttempts: true}, ErrConflict},
		{"published placement", QuestionSetState{ContentState: ContentState{Published: true}, Kind: "final_exam"}, ErrInvalidInput},
	} {
		t.Run(tc.name, func(t *testing.T) {
			active := &contentRepositoryStub{setState: func(context.Context, uuid.UUID) (QuestionSetState, error) { return tc.state, nil }}
			_, err := newStubService(active).UpdateQuestionSet(context.Background(), uuid.New(), v)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestRemoveSelectsArchiveForHistoryOrBlockingRelations(t *testing.T) {
	for _, tc := range []struct {
		name                          string
		history, deleted, wantArchive bool
	}{
		{"history", true, false, true}, {"delete", false, true, false}, {"blocked relation", false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var archived, cleaned bool
			active := &contentRepositoryStub{
				state:   func(context.Context, string, uuid.UUID) (ContentState, error) { return ContentState{}, nil },
				history: func(context.Context, string, uuid.UUID) (bool, error) { return tc.history, nil },
				deleteContent: func(context.Context, string, uuid.UUID) (bool, error) {
					if tc.history {
						t.Fatal("deleted used content")
					}
					return tc.deleted, nil
				},
				archive:     func(context.Context, string, uuid.UUID) error { archived = true; return nil },
				deleteDraft: func(context.Context, string, uuid.UUID) error { cleaned = true; return nil },
			}
			result, err := newStubService(&contentRepositoryStub{active: active}).RemoveQuestionSet(context.Background(), uuid.New())
			if err != nil || archived != tc.wantArchive || result.Archived != tc.wantArchive || !cleaned {
				t.Fatalf("%+v %v", result, err)
			}
		})
	}
}

func TestTemplateMetadataEditPreservesGeometry(t *testing.T) {
	id := uuid.New()
	var data kanastroke.Data
	if err := json.Unmarshal([]byte(`{"strokes":[{"path":"M0 0 L1 1"}]}`), &data); err != nil {
		t.Fatal(err)
	}
	stored := StrokeTemplateDetail{StrokeTemplate: StrokeTemplate{ID: id, Script: "hiragana", Character: "あ", Romaji: "a", SourceVersion: "v1"}, StrokeData: data}
	saved := false
	active := &contentRepositoryStub{
		template: func(_ context.Context, got uuid.UUID, lock bool) (StrokeTemplateDetail, error) {
			if got != id || !lock {
				t.Fatal("missing lock")
			}
			return stored, nil
		},
		updateTemplate: func(_ context.Context, v StrokeTemplateDetail) error {
			if v.ID != id || v.Romaji != "aa" || v.StrokeData.Strokes[0].Path != stored.StrokeData.Strokes[0].Path {
				t.Fatal("changed geometry")
			}
			saved = true
			return nil
		},
	}
	input := StrokeTemplateInput{Script: "hiragana", Character: "あ", Romaji: "aa", SourceVersion: "v2"}
	_, err := newStubService(active).UpdateTemplate(context.Background(), id, input)
	if err != nil || !saved {
		t.Fatal(err)
	}
	input.Character = "い"
	saved = false
	_, err = newStubService(active).UpdateTemplate(context.Background(), id, input)
	if !errors.Is(err, ErrInvalidInput) || saved {
		t.Fatalf("identity changed without SVG: %v", err)
	}
}

func TestServiceUsesInjectedTransactionRunner(t *testing.T) {
	token := &transactionToken{}
	runner := &transactionRunnerStub{tx: token}
	active := &contentRepositoryStub{}
	repository := &contentRepositoryStub{active: active}
	svc := NewService(runner, repository).(*service)
	failure := errors.New("callback failed")
	err := svc.execTx(context.Background(), func(r ContentRepository) error {
		if r != active {
			t.Fatal("callback did not receive the transaction repository")
		}
		return failure
	})
	if !errors.Is(err, failure) || runner.calls != 1 || repository.boundTx != token {
		t.Fatalf("runner calls=%d transaction=%v error=%v", runner.calls, repository.boundTx, err)
	}
}

func TestServiceClassifiesSharedTransactionErrors(t *testing.T) {
	for _, commit := range []bool{false, true} {
		t.Run(fmt.Sprint(commit), func(t *testing.T) {
			original := &pgconn.PgError{Code: "23505"}
			runner := &transactionRunnerStub{}
			if commit {
				runner.commitErr = original
			} else {
				runner.beginErr = original
			}
			repository := &contentRepositoryStub{}
			called := false
			err := NewService(runner, repository).(*service).execTx(context.Background(), func(ContentRepository) error { called = true; return nil })
			if !errors.Is(err, ErrConflict) || !errors.Is(err, original) || called != commit {
				t.Fatalf("called=%t error=%v", called, err)
			}
		})
	}
}
