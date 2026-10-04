package course

import (
	"context"
	"errors"
	"testing"

	"nihon-no-hikari-api/internal/course/model"
	"nihon-no-hikari-api/internal/question"
	"nihon-no-hikari-api/pkg/utils"

	"github.com/google/uuid"
)

type lessonProgressRepositoryStub struct {
	upsertCalled bool
	lesson       *model.LessonDetail
}

func (r *lessonProgressRepositoryStub) List(context.Context) ([]model.Course, error) {
	return nil, nil
}

func (r *lessonProgressRepositoryStub) GetCourseDetail(context.Context, uuid.UUID, uuid.UUID) (*model.CourseDetail, error) {
	return nil, nil
}

func (r *lessonProgressRepositoryStub) ListLessons(context.Context, uuid.UUID, uuid.UUID) ([]model.ModuleLesson, error) {
	return nil, nil
}

func (r *lessonProgressRepositoryStub) GetLessonDetail(context.Context, uuid.UUID, uuid.UUID) (*model.LessonDetail, error) {
	return r.lesson, nil
}

type questionRepositoryStub struct {
	sets             []question.QuestionSet
	receivedCourseID uuid.UUID
	receivedLessonID uuid.UUID
	receivedUserID   uuid.UUID
}

func (r *questionRepositoryStub) ListQuestionSets(_ context.Context, courseID, lessonID, userID uuid.UUID) ([]question.QuestionSet, error) {
	r.receivedCourseID = courseID
	r.receivedLessonID = lessonID
	r.receivedUserID = userID
	return r.sets, nil
}

func (r *questionRepositoryStub) GetQuestionSetDetail(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) (*question.QuestionSetDetail, error) {
	return nil, nil
}

func (r *questionRepositoryStub) StartAttempt(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) (*question.Attempt, error) {
	return nil, nil
}

func (r *questionRepositoryStub) SubmitAttempt(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, question.SubmitAttemptRequest) (*question.AttemptResult, error) {
	return nil, nil
}

func (r *lessonProgressRepositoryStub) UpsertModuleProgress(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, model.UpsertModuleProgressRequest) (*model.ModuleProgress, error) {
	return nil, nil
}

func (r *lessonProgressRepositoryStub) UpsertLessonProgress(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, model.UpsertLessonProgressRequest) (*model.LessonProgress, error) {
	r.upsertCalled = true
	return &model.LessonProgress{}, nil
}

func TestUpsertLessonProgressRejectsDirectCompletion(t *testing.T) {
	repository := &lessonProgressRepositoryStub{}
	service := NewService(repository, nil)
	status := model.StatusCompleted

	_, err := service.UpsertLessonProgress(context.Background(), uuid.New(), uuid.New(), uuid.New(), model.UpsertLessonProgressRequest{Status: &status})
	if !errors.Is(err, utils.ErrLessonCompletionRequiresQuiz) {
		t.Fatalf("UpsertLessonProgress() error = %v, want %v", err, utils.ErrLessonCompletionRequiresQuiz)
	}
	if repository.upsertCalled {
		t.Fatal("repository must not be called for direct completion")
	}
}

func TestGetLessonDetailIncludesQuestionSets(t *testing.T) {
	courseID := uuid.New()
	lessonID := uuid.New()
	userID := uuid.New()
	setID := uuid.New()
	courseRepository := &lessonProgressRepositoryStub{
		lesson: &model.LessonDetail{ID: lessonID, Slug: "hiragana-a", Title: "Hiragana A"},
	}
	questionRepository := &questionRepositoryStub{
		sets: []question.QuestionSet{{ID: setID, LessonID: lessonID, Title: "Latihan"}},
	}
	service := NewService(courseRepository, questionRepository)

	response, err := service.GetLessonDetail(context.Background(), courseID, lessonID, userID)
	if err != nil {
		t.Fatalf("GetLessonDetail() error = %v", err)
	}
	if len(response.Data.QuestionSets) != 1 || response.Data.QuestionSets[0].ID != setID {
		t.Fatalf("GetLessonDetail() question sets = %#v", response.Data.QuestionSets)
	}
	if questionRepository.receivedCourseID != courseID || questionRepository.receivedLessonID != lessonID || questionRepository.receivedUserID != userID {
		t.Fatal("GetLessonDetail() did not forward course, lesson, and user IDs")
	}
}
