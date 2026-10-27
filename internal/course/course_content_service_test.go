package course

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"nihon-no-hikari-api/internal/course/model"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Embedding an unimplemented repository makes unexpected persistence calls fail.
type contentRepositoryStub struct {
	Repository
	active        ContentRepository
	boundTx       pgx.Tx
	state         func(context.Context, string, uuid.UUID) (model.ContentState, error)
	draft         func(context.Context, string, uuid.UUID) (json.RawMessage, error)
	saveCore      func(context.Context, string, uuid.UUID, model.CoreInput, bool) error
	saveDraft     func(context.Context, string, uuid.UUID, json.RawMessage) error
	deleteDraft   func(context.Context, string, uuid.UUID) error
	published     func(context.Context, string, uuid.UUID, bool) error
	history       func(context.Context, string, uuid.UUID) (bool, error)
	deleteContent func(context.Context, string, uuid.UUID) (bool, error)
	archive       func(context.Context, string, uuid.UUID) error
	siblings      func(context.Context, string, uuid.UUID) ([]uuid.UUID, error)
	order         func(context.Context, string, model.ReorderRequest) error
	prerequisites func(context.Context, string, uuid.UUID, []uuid.UUID) (int, bool, error)
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
	return NewService(&transactionRunnerStub{}, repository, nil)
}

func (r *contentRepositoryStub) UseTx(tx pgx.Tx) ContentRepository {
	r.boundTx = tx
	if r.active != nil {
		return r.active
	}
	return r
}

func (r *contentRepositoryStub) ContentState(c context.Context, k string, id uuid.UUID) (model.ContentState, error) {
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

func (r *contentRepositoryStub) SaveCore(c context.Context, k string, id uuid.UUID, v model.CoreInput, b bool) error {
	return r.saveCore(c, k, id, v, b)
}

func (r *contentRepositoryStub) LockSiblings(c context.Context, k string, id uuid.UUID) ([]uuid.UUID, error) {
	return r.siblings(c, k, id)
}

func (r *contentRepositoryStub) ApplyOrder(c context.Context, k string, v model.ReorderRequest) error {
	return r.order(c, k, v)
}

func (r *contentRepositoryStub) PrerequisiteState(c context.Context, k string, id uuid.UUID, v []uuid.UUID) (int, bool, error) {
	return r.prerequisites(c, k, id, v)
}

func TestCoreUpdateSelectsLiveOrDraftStorage(t *testing.T) {
	for _, published := range []bool{false, true} {
		t.Run(fmt.Sprint(published), func(t *testing.T) {
			id := uuid.New()
			input := model.CoreInput{LevelID: uuid.New(), Slug: "course", Title: "Course"}
			active := &contentRepositoryStub{}
			active.state = func(context.Context, string, uuid.UUID) (model.ContentState, error) {
				return model.ContentState{Published: published}, nil
			}
			saved := false
			active.saveCore = func(_ context.Context, _ string, got uuid.UUID, v model.CoreInput, create bool) error {
				if published || create || got != id || v.Title != "Course" {
					t.Fatal("incorrect live update")
				}
				saved = true
				return nil
			}
			active.saveDraft = func(_ context.Context, _ string, got uuid.UUID, raw json.RawMessage) error {
				var v model.CoreInput
				if err := json.Unmarshal(raw, &v); err != nil {
					t.Fatal(err)
				}
				if !published || got != id || v.Title != "Course" {
					t.Fatal("incorrect draft update")
				}
				saved = true
				return nil
			}
			service := newStubService(&contentRepositoryStub{active: active})
			result, err := service.UpdateAdminContent(context.Background(), "course", id, input)
			if err != nil || !saved || result.Draft != published {
				t.Fatalf("%+v %v", result, err)
			}
		})
	}
}

func TestServiceValidationRunsBeforePersistence(t *testing.T) {
	_, err := NewService(nil, nil, nil).CreateAdminContent(context.Background(), "course", model.CoreInput{})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected input error: %v", err)
	}
}

func TestPublishCoreUsesTransactionRepositoryAndStopsOnFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			var calls []string
			injected := errors.New("storage failed")
			v := model.CoreInput{LevelID: uuid.New(), Slug: "course", Title: "Draft"}
			raw, _ := json.Marshal(v)
			active := &contentRepositoryStub{
				state: func(context.Context, string, uuid.UUID) (model.ContentState, error) {
					return model.ContentState{Published: true}, nil
				},
				draft: func(context.Context, string, uuid.UUID) (json.RawMessage, error) { return raw, nil },
				saveCore: func(context.Context, string, uuid.UUID, model.CoreInput, bool) error {
					calls = append(calls, "save")
					if fail {
						return injected
					}
					return nil
				},
				published:   func(context.Context, string, uuid.UUID, bool) error { calls = append(calls, "publish"); return nil },
				deleteDraft: func(context.Context, string, uuid.UUID) error { calls = append(calls, "delete draft"); return nil },
			}
			_, err := newStubService(&contentRepositoryStub{active: active}).PublishAdminContent(context.Background(), "course", uuid.New())
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
			result, err := newStubService(&contentRepositoryStub{active: active}).RemoveAdminContent(context.Background(), "lesson", uuid.New())
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
		active := &contentRepositoryStub{
			siblings: func(context.Context, string, uuid.UUID) ([]uuid.UUID, error) { return []uuid.UUID{a, b}, nil },
			order:    func(context.Context, string, model.ReorderRequest) error { applied = true; return nil },
		}
		_, err := newStubService(active).ReorderAdminContent(context.Background(), "module", model.ReorderRequest{ParentID: parent, OrderedIDs: ids})
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
		active := &contentRepositoryStub{prerequisites: func(context.Context, string, uuid.UUID, []uuid.UUID) (int, bool, error) {
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

func TestServiceUsesInjectedTransactionRunner(t *testing.T) {
	token := &transactionToken{}
	runner := &transactionRunnerStub{tx: token}
	active := &contentRepositoryStub{}
	repository := &contentRepositoryStub{active: active}
	svc := NewService(runner, repository, nil).(*service)
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
			err := NewService(runner, repository, nil).(*service).execTx(context.Background(), func(ContentRepository) error { called = true; return nil })
			if !errors.Is(err, ErrConflict) || !errors.Is(err, original) || called != commit {
				t.Fatalf("called=%t error=%v", called, err)
			}
		})
	}
}
