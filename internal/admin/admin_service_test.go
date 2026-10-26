package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"nihon-no-hikari-api/internal/admin/model"
	"nihon-no-hikari-api/pkg/kanastroke"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Embedding an unimplemented repository makes unexpected persistence calls fail.
type serviceRepositoryStub struct {
	Repository
	active         Repository
	boundTx        pgx.Tx
	state          func(context.Context, string, uuid.UUID) (model.ContentState, error)
	draft          func(context.Context, string, uuid.UUID) (json.RawMessage, error)
	saveCore       func(context.Context, string, uuid.UUID, coreInput, bool) error
	saveDraft      func(context.Context, string, uuid.UUID, json.RawMessage) error
	deleteDraft    func(context.Context, string, uuid.UUID) error
	published      func(context.Context, string, uuid.UUID, bool) error
	setState       func(context.Context, uuid.UUID) (model.QuestionSetState, error)
	insertSet      func(context.Context, uuid.UUID, uuid.UUID, int, setInput) error
	history        func(context.Context, string, uuid.UUID) (bool, error)
	deleteContent  func(context.Context, string, uuid.UUID) (bool, error)
	archive        func(context.Context, string, uuid.UUID) error
	siblings       func(context.Context, string, uuid.UUID) ([]uuid.UUID, error)
	order          func(context.Context, string, ReorderRequest) error
	prerequisites  func(context.Context, string, uuid.UUID, []uuid.UUID) (int, bool, error)
	template       func(context.Context, uuid.UUID, bool) (StrokeTemplateDetail, error)
	updateTemplate func(context.Context, StrokeTemplateDetail) error
}

func (r *serviceRepositoryStub) UseTx(tx pgx.Tx) Repository {
	r.boundTx = tx
	if r.active != nil {
		return r.active
	}
	return r
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

func (r *serviceRepositoryStub) ContentState(c context.Context, k string, id uuid.UUID) (model.ContentState, error) {
	return r.state(c, k, id)
}

func (r *serviceRepositoryStub) Draft(c context.Context, k string, id uuid.UUID) (json.RawMessage, error) {
	return r.draft(c, k, id)
}

func (r *serviceRepositoryStub) SaveCore(c context.Context, k string, id uuid.UUID, v coreInput, b bool) error {
	return r.saveCore(c, k, id, v, b)
}

func (r *serviceRepositoryStub) SaveDraft(c context.Context, k string, id uuid.UUID, v json.RawMessage) error {
	return r.saveDraft(c, k, id, v)
}

func (r *serviceRepositoryStub) DeleteDraft(c context.Context, k string, id uuid.UUID) error {
	return r.deleteDraft(c, k, id)
}

func (r *serviceRepositoryStub) SetPublished(c context.Context, k string, id uuid.UUID, b bool) error {
	return r.published(c, k, id, b)
}

func (r *serviceRepositoryStub) QuestionSetState(c context.Context, id uuid.UUID) (model.QuestionSetState, error) {
	return r.setState(c, id)
}

func (r *serviceRepositoryStub) InsertSet(c context.Context, id, lineage uuid.UUID, version int, v setInput) error {
	return r.insertSet(c, id, lineage, version, v)
}

func (r *serviceRepositoryStub) HasHistory(c context.Context, k string, id uuid.UUID) (bool, error) {
	return r.history(c, k, id)
}

func (r *serviceRepositoryStub) DeleteContent(c context.Context, k string, id uuid.UUID) (bool, error) {
	return r.deleteContent(c, k, id)
}

func (r *serviceRepositoryStub) Archive(c context.Context, k string, id uuid.UUID) error {
	return r.archive(c, k, id)
}

func (r *serviceRepositoryStub) LockSiblings(c context.Context, k string, id uuid.UUID) ([]uuid.UUID, error) {
	return r.siblings(c, k, id)
}

func (r *serviceRepositoryStub) ApplyOrder(c context.Context, k string, v ReorderRequest) error {
	return r.order(c, k, v)
}

func (r *serviceRepositoryStub) PrerequisiteState(c context.Context, k string, id uuid.UUID, v []uuid.UUID) (int, bool, error) {
	return r.prerequisites(c, k, id, v)
}

func (r *serviceRepositoryStub) Template(c context.Context, id uuid.UUID, b bool) (StrokeTemplateDetail, error) {
	return r.template(c, id, b)
}

func (r *serviceRepositoryStub) UpdateTemplate(c context.Context, v StrokeTemplateDetail) error {
	return r.updateTemplate(c, v)
}

func TestCoreUpdateSelectsLiveOrDraftStorage(t *testing.T) {
	for _, published := range []bool{false, true} {
		t.Run(fmt.Sprint(published), func(t *testing.T) {
			id := uuid.New()
			input := coreInput{LevelID: uuid.New(), Slug: "course", Title: "Course"}
			active := &serviceRepositoryStub{}
			active.state = func(context.Context, string, uuid.UUID) (model.ContentState, error) {
				return model.ContentState{Published: published}, nil
			}
			saved := false
			active.saveCore = func(_ context.Context, _ string, got uuid.UUID, v coreInput, create bool) error {
				if published || create || got != id || v.Title != "Course" {
					t.Fatal("incorrect live update")
				}
				saved = true
				return nil
			}
			active.saveDraft = func(_ context.Context, _ string, got uuid.UUID, raw json.RawMessage) error {
				var v coreInput
				if err := json.Unmarshal(raw, &v); err != nil {
					t.Fatal(err)
				}
				if !published || got != id || v.Title != "Course" {
					t.Fatal("incorrect draft update")
				}
				saved = true
				return nil
			}
			service := newStubService(&serviceRepositoryStub{active: active})
			result, err := service.UpdateContent(context.Background(), "course", id, input)
			if err != nil || !saved || result.Draft != published {
				t.Fatalf("%+v %v", result, err)
			}
		})
	}
}

func TestServiceValidationRunsBeforePersistence(t *testing.T) {
	_, err := NewService(nil, nil).CreateContent(context.Background(), "course", coreInput{})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected input error: %v", err)
	}
}

func TestPublishCoreUsesTransactionRepositoryAndStopsOnFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			var calls []string
			injected := errors.New("storage failed")
			v := coreInput{LevelID: uuid.New(), Slug: "course", Title: "Draft"}
			raw, _ := json.Marshal(v)
			active := &serviceRepositoryStub{
				state: func(context.Context, string, uuid.UUID) (model.ContentState, error) {
					return model.ContentState{Published: true}, nil
				},
				draft: func(context.Context, string, uuid.UUID) (json.RawMessage, error) { return raw, nil },
				saveCore: func(context.Context, string, uuid.UUID, coreInput, bool) error {
					calls = append(calls, "save")
					if fail {
						return injected
					}
					return nil
				},
				published:   func(context.Context, string, uuid.UUID, bool) error { calls = append(calls, "publish"); return nil },
				deleteDraft: func(context.Context, string, uuid.UUID) error { calls = append(calls, "delete draft"); return nil },
			}
			_, err := newStubService(&serviceRepositoryStub{active: active}).PublishContent(context.Background(), "course", uuid.New())
			if fail {
				if !errors.Is(err, injected) || fmt.Sprint(calls) != "[save]" {
					t.Fatalf("%v %v", calls, err)
				}
			} else if err != nil || fmt.Sprint(calls) != "[save publish delete draft]" {
				t.Fatalf("%v %v", calls, err)
			}
		})
	}
}

func validReadingSet(lesson uuid.UUID) setInput {
	stimulus := "a"
	return setInput{Kind: "practice", LessonID: &lesson, Title: "Reading", Skill: "reading", Questions: []questionInput{{QuestionType: "multiple_choice", PromptText: "Which?", StimulusText: &stimulus, Points: 1, Options: []optionInput{{Label: "a", IsCorrect: true}, {Label: "b"}}}}}
}

func TestQuestionSetVersionKeepsLineageAndDeletesOriginalDraft(t *testing.T) {
	original, lineage, lesson := uuid.New(), uuid.New(), uuid.New()
	v := validReadingSet(lesson)
	raw, _ := json.Marshal(v)
	var created uuid.UUID
	var calls []string
	active := &serviceRepositoryStub{
		setState: func(context.Context, uuid.UUID) (model.QuestionSetState, error) {
			return model.QuestionSetState{ContentState: model.ContentState{Published: true}, LineageID: lineage, Version: 4, Kind: v.Kind, Skill: &v.Skill, LessonID: v.LessonID}, nil
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
		insertSet: func(_ context.Context, id, gotLineage uuid.UUID, version int, _ setInput) error {
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
	result, err := newStubService(&serviceRepositoryStub{active: active}).PublishContent(context.Background(), "question_set", original)
	if err != nil || result.ID == nil || *result.ID != created || fmt.Sprint(calls) != "[false true draft]" {
		t.Fatalf("%+v %v %v", result, calls, err)
	}
}

func TestQuestionSetAttemptAndPlacementRestrictions(t *testing.T) {
	lesson := uuid.New()
	v := validReadingSet(lesson)
	for _, tc := range []struct {
		name  string
		state model.QuestionSetState
		want  error
	}{
		{"attempted version", model.QuestionSetState{HasAttempts: true}, ErrConflict},
		{"published placement", model.QuestionSetState{ContentState: model.ContentState{Published: true}, Kind: "final_exam"}, ErrInvalidInput},
	} {
		t.Run(tc.name, func(t *testing.T) {
			active := &serviceRepositoryStub{setState: func(context.Context, uuid.UUID) (model.QuestionSetState, error) { return tc.state, nil }}
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
			active := &serviceRepositoryStub{
				state:   func(context.Context, string, uuid.UUID) (model.ContentState, error) { return model.ContentState{}, nil },
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
			result, err := newStubService(&serviceRepositoryStub{active: active}).RemoveContent(context.Background(), "lesson", uuid.New())
			if err != nil || archived != tc.wantArchive || result.Archived != tc.wantArchive || !cleaned {
				t.Fatalf("%+v %v", result, err)
			}
		})
	}
}

func TestReorderRejectsChangedMembershipBeforeWriting(t *testing.T) {
	parent, a, b := uuid.New(), uuid.New(), uuid.New()
	for _, ids := range [][]uuid.UUID{{a}, {a, uuid.New()}, {b, a}} {
		applied := false
		active := &serviceRepositoryStub{
			siblings: func(context.Context, string, uuid.UUID) ([]uuid.UUID, error) { return []uuid.UUID{a, b}, nil },
			order:    func(context.Context, string, ReorderRequest) error { applied = true; return nil },
		}
		_, err := newStubService(active).Reorder(context.Background(), "module", ReorderRequest{ParentID: parent, OrderedIDs: ids})
		valid := len(ids) == 2 && ids[0] == b
		if valid {
			if err != nil || !applied {
				t.Fatalf("%v %v", ids, err)
			}
		} else if !errors.Is(err, ErrStaleOrder) || applied {
			t.Fatalf("%v %v", ids, err)
		}
	}
}

func TestPrerequisiteExistenceAndCycles(t *testing.T) {
	for _, tc := range []struct {
		count   int
		cycle   bool
		message string
	}{{0, false, "konten yang dipilih tidak ditemukan"}, {1, true, "hubungan prasyarat tidak boleh melingkar"}, {1, false, ""}} {
		active := &serviceRepositoryStub{prerequisites: func(context.Context, string, uuid.UUID, []uuid.UUID) (int, bool, error) {
			return tc.count, tc.cycle, nil
		}}
		err := validatePrerequisites(context.Background(), active, "lesson", uuid.New(), []uuid.UUID{uuid.New()})
		if tc.message == "" {
			if err != nil {
				t.Fatal(err)
			}
		} else if !errors.Is(err, errInvalidPrerequisites) || err.Error() != "prasyarat tidak valid: "+tc.message {
			t.Fatal(err)
		}
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
	active := &serviceRepositoryStub{
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
	active := &serviceRepositoryStub{}
	repository := &serviceRepositoryStub{active: active}
	svc := NewService(runner, repository).(*service)
	failure := errors.New("callback failed")
	err := svc.execTx(context.Background(), func(r Repository) error {
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
			repository := &serviceRepositoryStub{}
			called := false
			err := NewService(runner, repository).(*service).execTx(context.Background(), func(Repository) error { called = true; return nil })
			if !errors.Is(err, ErrConflict) || !errors.Is(err, original) || called != commit {
				t.Fatalf("called=%t error=%v", called, err)
			}
		})
	}
}
