package question

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

type serviceRepositoryStub struct {
	ContentRepository
	detail             *QuestionSetDetail
	startAttemptResult *Attempt
	abandonErr         error
	abandonCalled      bool
	abandonedAttemptID uuid.UUID
	abandonedUserID    uuid.UUID
}

func (r *serviceRepositoryStub) ListQuestionSets(context.Context, uuid.UUID, uuid.UUID) ([]QuestionSet, error) {
	return nil, nil
}

func (r *serviceRepositoryStub) ListModuleExams(context.Context, uuid.UUID, uuid.UUID) ([]QuestionSet, error) {
	return nil, nil
}

func (r *serviceRepositoryStub) GetQuestionSetDetail(context.Context, uuid.UUID, uuid.UUID) (*QuestionSetDetail, error) {
	return r.detail, nil
}

func (r *serviceRepositoryStub) StartAttempt(context.Context, uuid.UUID, uuid.UUID) (*Attempt, error) {
	return r.startAttemptResult, nil
}

func (r *serviceRepositoryStub) AbandonAttempt(_ context.Context, _, attemptID, userID uuid.UUID) error {
	r.abandonCalled = true
	r.abandonedAttemptID = attemptID
	r.abandonedUserID = userID
	return r.abandonErr
}

func (r *serviceRepositoryStub) SubmitAttempt(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, SubmitAttemptRequest) (*AttemptResult, error) {
	return nil, nil
}

func TestGetQuestionSetDetail(t *testing.T) {
	questionSetID := uuid.New()
	questionID := uuid.New()
	optionID := uuid.New()
	repository := &serviceRepositoryStub{
		detail: &QuestionSetDetail{
			QuestionSet: QuestionSet{ID: questionSetID, Title: "Latihan membaca"},
			Questions: []Question{{
				ID:    questionID,
				Skill: SkillReading,
				Options: []QuestionOption{{
					ID:    optionID,
					Label: "a",
				}},
			}},
		},
	}
	service := NewService(nil, repository)

	response, err := service.GetQuestionSetDetail(context.Background(), questionSetID, uuid.New())
	if err != nil {
		t.Fatalf("GetQuestionSetDetail() error = %v", err)
	}
	if response.Data.ID != questionSetID || len(response.Data.Questions) != 1 {
		t.Fatalf("GetQuestionSetDetail() data = %#v", response.Data)
	}
	if len(response.Data.Questions[0].Options) != 1 || response.Data.Questions[0].Options[0].ID != optionID {
		t.Fatalf("GetQuestionSetDetail() options = %#v", response.Data.Questions[0].Options)
	}
}

func TestAbandonAttempt(t *testing.T) {
	repository := &serviceRepositoryStub{}
	service := NewService(nil, repository)
	attemptID := uuid.New()
	userID := uuid.New()

	response, err := service.AbandonAttempt(context.Background(), uuid.New(), attemptID, userID)
	if err != nil {
		t.Fatalf("AbandonAttempt() error = %v", err)
	}
	if !repository.abandonCalled || repository.abandonedAttemptID != attemptID || repository.abandonedUserID != userID {
		t.Fatal("AbandonAttempt() did not forward the attempt and user IDs")
	}
	if !response.Success || response.Message != "attempt berhasil ditinggalkan" {
		t.Fatalf("AbandonAttempt() response = %#v", response)
	}
}

func TestStartAttempt(t *testing.T) {
	questionSetID := uuid.New()
	attemptID := uuid.New()
	userID := uuid.New()
	trueVal := true
	explanation := "Penjelasan kata"

	stubAttempt := &Attempt{
		ID: attemptID,
		QuestionSet: QuestionSet{
			ID:   questionSetID,
			Kind: QuestionSetKindPractice,
		},
		Questions: []Question{
			{
				ID:          uuid.New(),
				Explanation: &explanation,
				Options: []QuestionOption{
					{
						ID:        uuid.New(),
						Label:     "A",
						IsCorrect: &trueVal,
					},
				},
			},
		},
	}

	repo := &serviceRepositoryStub{startAttemptResult: stubAttempt}
	svc := NewService(nil, repo)

	resp, err := svc.StartAttempt(context.Background(), questionSetID, userID)
	if err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}
	if !resp.Success || resp.Data.ID != attemptID {
		t.Fatalf("StartAttempt() response = %#v", resp)
	}
	if resp.Data.Questions[0].Explanation == nil || *resp.Data.Questions[0].Explanation != explanation {
		t.Fatalf("StartAttempt() expected explanation %q, got %#v", explanation, resp.Data.Questions[0].Explanation)
	}
	if resp.Data.Questions[0].Options[0].IsCorrect == nil || !*resp.Data.Questions[0].Options[0].IsCorrect {
		t.Fatalf("StartAttempt() expected is_correct true, got %#v", resp.Data.Questions[0].Options[0].IsCorrect)
	}
}
