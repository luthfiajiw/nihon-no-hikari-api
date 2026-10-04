package question

import (
	"context"
	"fmt"

	"nihon-no-hikari-api/pkg/utils"

	"github.com/google/uuid"
)

type Service interface {
	ListQuestionSets(ctx context.Context, courseID, lessonID, userID uuid.UUID) (*ListQuestionSetsResponse, error)
	GetQuestionSetDetail(ctx context.Context, courseID, lessonID, questionSetID, userID uuid.UUID) (*QuestionSetDetailResponse, error)
	StartAttempt(ctx context.Context, courseID, lessonID, questionSetID, userID uuid.UUID) (*StartAttemptResponse, error)
	SubmitAttempt(ctx context.Context, courseID, lessonID, questionSetID, attemptID, userID uuid.UUID, req SubmitAttemptRequest) (*SubmitAttemptResponse, error)
}

func (s *service) GetQuestionSetDetail(ctx context.Context, courseID, lessonID, questionSetID, userID uuid.UUID) (*QuestionSetDetailResponse, error) {
	detail, err := s.repository.GetQuestionSetDetail(ctx, courseID, lessonID, questionSetID, userID)
	if err != nil {
		return nil, err
	}
	return &QuestionSetDetailResponse{
		Success: true,
		Message: "berhasil mendapatkan detail question set",
		Data:    *detail,
	}, nil
}

type service struct {
	repository Repository
}

func NewService(repository Repository) Service {
	return &service{repository: repository}
}

func (s *service) ListQuestionSets(ctx context.Context, courseID, lessonID, userID uuid.UUID) (*ListQuestionSetsResponse, error) {
	sets, err := s.repository.ListQuestionSets(ctx, courseID, lessonID, userID)
	if err != nil {
		return nil, err
	}
	return &ListQuestionSetsResponse{
		Success: true,
		Message: "berhasil mendapatkan question set pelajaran",
		Data:    sets,
	}, nil
}

func (s *service) StartAttempt(ctx context.Context, courseID, lessonID, questionSetID, userID uuid.UUID) (*StartAttemptResponse, error) {
	attempt, err := s.repository.StartAttempt(ctx, courseID, lessonID, questionSetID, userID)
	if err != nil {
		return nil, err
	}
	return &StartAttemptResponse{
		Success: true,
		Message: "attempt berhasil dimulai",
		Data:    *attempt,
	}, nil
}

func (s *service) SubmitAttempt(ctx context.Context, courseID, lessonID, questionSetID, attemptID, userID uuid.UUID, req SubmitAttemptRequest) (*SubmitAttemptResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", utils.ErrInvalidAttempt, err)
	}
	result, err := s.repository.SubmitAttempt(ctx, courseID, lessonID, questionSetID, attemptID, userID, req)
	if err != nil {
		return nil, err
	}
	message := "attempt selesai, nilai belum memenuhi syarat kelulusan"
	if result.LessonStatus == "completed" {
		message = "selamat, lesson berhasil diselesaikan"
	} else if result.IsPassed {
		message = "attempt lulus, selesaikan question set lainnya"
	}
	return &SubmitAttemptResponse{
		Success: true,
		Message: message,
		Data:    *result,
	}, nil
}
