package course

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"nihon-no-hikari-api/internal/course/model"
	"nihon-no-hikari-api/internal/question"
	"nihon-no-hikari-api/pkg/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type lessonProgressRepositoryStub struct {
	Repository
	upsertCalled       bool
	lesson             *model.LessonDetail
	upsertLessonResult *model.LessonProgress
	upsertLessonErr    error
	upsertModuleResult *model.ModuleProgress
	upsertModuleErr    error
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

func (r *lessonProgressRepositoryStub) GetLessonDetail(context.Context, uuid.UUID) (*model.LessonDetail, error) {
	return r.lesson, nil
}

type questionRepositoryStub struct {
	question.CatalogReader
	sets             []question.QuestionSet
	receivedLessonID uuid.UUID
	receivedUserID   uuid.UUID
}

func (r *questionRepositoryStub) ListQuestionSets(_ context.Context, lessonID, userID uuid.UUID) ([]question.QuestionSet, error) {
	r.receivedLessonID = lessonID
	r.receivedUserID = userID
	return r.sets, nil
}

func (r *questionRepositoryStub) ListModuleExams(context.Context, uuid.UUID, uuid.UUID) ([]question.QuestionSet, error) {
	return nil, nil
}

func (r *questionRepositoryStub) GetQuestionSetDetail(context.Context, uuid.UUID, uuid.UUID) (*question.QuestionSetDetail, error) {
	return nil, nil
}

func (r *questionRepositoryStub) StartAttempt(context.Context, uuid.UUID, uuid.UUID) (*question.Attempt, error) {
	return nil, nil
}

func (r *questionRepositoryStub) AbandonAttempt(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return nil
}

func (r *questionRepositoryStub) SubmitAttempt(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, question.SubmitAttemptRequest) (*question.AttemptResult, error) {
	return nil, nil
}

func (r *lessonProgressRepositoryStub) UpsertModuleProgress(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, model.UpsertModuleProgressRequest) (*model.ModuleProgress, error) {
	if r.upsertModuleErr != nil {
		return nil, r.upsertModuleErr
	}
	if r.upsertModuleResult != nil {
		return r.upsertModuleResult, nil
	}
	return nil, nil
}

func (r *lessonProgressRepositoryStub) UpsertLessonProgress(context.Context, uuid.UUID, uuid.UUID, model.UpsertLessonProgressRequest) (*model.LessonProgress, error) {
	r.upsertCalled = true
	if r.upsertLessonErr != nil {
		return nil, r.upsertLessonErr
	}
	if r.upsertLessonResult != nil {
		return r.upsertLessonResult, nil
	}
	return &model.LessonProgress{}, nil
}

func TestUpsertLessonProgressDelegatesCompletionPolicy(t *testing.T) {
	repository := &lessonProgressRepositoryStub{}
	service := NewService(nil, repository, nil)
	status := model.StatusCompleted

	_, err := service.UpsertLessonProgress(context.Background(), uuid.New(), uuid.New(), model.UpsertLessonProgressRequest{Status: &status})
	if err != nil {
		t.Fatalf("UpsertLessonProgress() error = %v", err)
	}
	if !repository.upsertCalled {
		t.Fatal("repository must decide whether the lesson has a quiz")
	}
}

func TestGetLessonDetailIncludesQuestionSets(t *testing.T) {
	lessonID := uuid.New()
	userID := uuid.New()
	setID := uuid.New()
	courseRepository := &lessonProgressRepositoryStub{
		lesson: &model.LessonDetail{ID: lessonID, Slug: "hiragana-a", Title: "Hiragana A"},
	}
	questionRepository := &questionRepositoryStub{
		sets: []question.QuestionSet{{ID: setID, LessonID: &lessonID, Title: "Latihan"}},
	}
	service := NewService(nil, courseRepository, questionRepository)

	response, err := service.GetLessonDetail(context.Background(), lessonID, userID)
	if err != nil {
		t.Fatalf("GetLessonDetail() error = %v", err)
	}
	if len(response.Data.QuestionSets) != 1 || response.Data.QuestionSets[0].ID != setID {
		t.Fatalf("GetLessonDetail() question sets = %#v", response.Data.QuestionSets)
	}
	if questionRepository.receivedLessonID != lessonID || questionRepository.receivedUserID != userID {
		t.Fatal("GetLessonDetail() did not forward lesson and user IDs")
	}
}

func TestUpsertLessonProgressIncludesModuleSyncFields(t *testing.T) {
	lessonID := uuid.New()
	moduleID := uuid.New()
	userID := uuid.New()
	status := model.StatusCompleted
	repository := &lessonProgressRepositoryStub{
		upsertLessonResult: &model.LessonProgress{
			UserID:       userID,
			LessonID:     lessonID,
			ModuleID:     moduleID,
			Status:       model.StatusCompleted,
			ModuleStatus: model.StatusCompleted,
		},
	}
	service := NewService(nil, repository, nil)

	res, err := service.UpsertLessonProgress(context.Background(), lessonID, userID, model.UpsertLessonProgressRequest{Status: &status})
	if err != nil {
		t.Fatalf("UpsertLessonProgress() error = %v", err)
	}
	if res.Data.ModuleID != moduleID || res.Data.ModuleStatus != model.StatusCompleted {
		t.Fatalf("unexpected module sync fields: %+v", res.Data)
	}

	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	data := decoded["data"].(map[string]any)
	if data["module_id"] != moduleID.String() || data["module_status"] != string(model.StatusCompleted) {
		t.Fatalf("unexpected wire JSON: %s", raw)
	}
}

func TestProgressHandlersMapModuleSynchronizationConflicts(t *testing.T) {
	userID := uuid.New()

	t.Run("upsertLessonProgress maps ErrModulePrerequisiteNotCompleted to 409", func(t *testing.T) {
		repo := &lessonProgressRepositoryStub{upsertLessonErr: utils.ErrModulePrerequisiteNotCompleted}
		h := &Handler{service: NewService(nil, repo, nil)}
		app := fiber.New()
		app.Use(func(c fiber.Ctx) error {
			c.Locals(utils.UserIDKey, userID)
			return c.Next()
		})
		app.Put("/lessons/:lessonId/progress", h.upsertLessonProgress)

		req := httptest.NewRequest("PUT", "/lessons/"+uuid.NewString()+"/progress", strings.NewReader(`{"status":"in_progress"}`))
		req.Header.Set("Content-Type", "application/json")
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		var body utils.ErrorRes
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != fiber.StatusConflict || body.Error != utils.ErrModulePrerequisiteNotCompleted.Error() {
			t.Fatalf("status=%d body=%+v; want 409/%q", res.StatusCode, body, utils.ErrModulePrerequisiteNotCompleted.Error())
		}
	})

	t.Run("upsertModuleProgress maps ErrModuleLessonsNotCompleted to 409", func(t *testing.T) {
		repo := &lessonProgressRepositoryStub{upsertModuleErr: utils.ErrModuleLessonsNotCompleted}
		h := &Handler{service: NewService(nil, repo, nil)}
		app := fiber.New()
		app.Use(func(c fiber.Ctx) error {
			c.Locals(utils.UserIDKey, userID)
			return c.Next()
		})
		app.Put("/courses/:courseId/modules/:moduleId/progress", h.upsertModuleProgress)

		req := httptest.NewRequest("PUT", "/courses/"+uuid.NewString()+"/modules/"+uuid.NewString()+"/progress", strings.NewReader(`{"status":"completed"}`))
		req.Header.Set("Content-Type", "application/json")
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		var body utils.ErrorRes
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != fiber.StatusConflict || body.Error != utils.ErrModuleLessonsNotCompleted.Error() {
			t.Fatalf("status=%d body=%+v; want 409/%q", res.StatusCode, body, utils.ErrModuleLessonsNotCompleted.Error())
		}
	})
}
