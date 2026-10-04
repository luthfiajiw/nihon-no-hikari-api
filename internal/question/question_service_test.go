package question

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

type serviceRepositoryStub struct {
	detail *QuestionSetDetail
}

func (r *serviceRepositoryStub) ListQuestionSets(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) ([]QuestionSet, error) {
	return nil, nil
}

func (r *serviceRepositoryStub) GetQuestionSetDetail(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) (*QuestionSetDetail, error) {
	return r.detail, nil
}

func (r *serviceRepositoryStub) StartAttempt(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) (*Attempt, error) {
	return nil, nil
}

func (r *serviceRepositoryStub) SubmitAttempt(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, SubmitAttemptRequest) (*AttemptResult, error) {
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
	service := NewService(repository)

	response, err := service.GetQuestionSetDetail(context.Background(), uuid.New(), uuid.New(), questionSetID, uuid.New())
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
