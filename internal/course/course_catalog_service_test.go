package course

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"nihon-no-hikari-api/internal/course/model"

	"github.com/google/uuid"
)

type catalogRepositoryStub struct {
	Repository
	courseID *uuid.UUID
}

func (r *catalogRepositoryStub) Catalog(_ context.Context, courseID *uuid.UUID) (model.CatalogData, error) {
	r.courseID = courseID
	return model.CatalogData{Courses: []json.RawMessage{}, Modules: []json.RawMessage{}, Lessons: []json.RawMessage{}, QuestionSets: []json.RawMessage{}}, nil
}

type catalogQuestionsStub struct {
	QuestionReader
	calls int
	err   error
}

func (q *catalogQuestionsStub) CatalogQuestionSets(context.Context) ([]json.RawMessage, error) {
	q.calls++
	return []json.RawMessage{json.RawMessage(`{"id":"question-set"}`)}, q.err
}

func TestAdminCatalogComposesQuestionProjectionOnlyWithoutCourseFilter(t *testing.T) {
	repo, questions := &catalogRepositoryStub{}, &catalogQuestionsStub{}
	svc := NewService(nil, repo, questions)
	all, err := svc.AdminCatalog(context.Background(), nil)
	if err != nil || questions.calls != 1 || len(all.Data.QuestionSets) != 1 {
		t.Fatalf("catalog=%+v calls=%d err=%v", all, questions.calls, err)
	}
	id := uuid.New()
	filtered, err := svc.AdminCatalog(context.Background(), &id)
	if err != nil || questions.calls != 1 || repo.courseID != &id {
		t.Fatalf("filtered catalog called the question reader: %+v %v", filtered, err)
	}
	for _, items := range [][]json.RawMessage{filtered.Data.Courses, filtered.Data.Modules, filtered.Data.Lessons, filtered.Data.QuestionSets} {
		if items == nil || len(items) != 0 {
			t.Fatalf("filtered empty array changed: %v", items)
		}
	}
	questions.err = errors.New("question projection unavailable")
	if _, err := svc.AdminCatalog(context.Background(), nil); !errors.Is(err, questions.err) {
		t.Fatalf("question reader failure lost: %v", err)
	}
}
