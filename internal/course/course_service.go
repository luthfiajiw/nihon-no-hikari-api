package course

import (
	"context"
	"fmt"
	"nihon-no-hikari-api/internal/course/model"
	"nihon-no-hikari-api/internal/question"
	"nihon-no-hikari-api/pkg/utils"

	"github.com/google/uuid"
)

type Service interface {
	GetList(ctx context.Context) (*model.ListCourseResponse, error)
	GetDetail(ctx context.Context, courseID, userID uuid.UUID) (*model.CourseDetailResponse, error)
	GetLessons(ctx context.Context, courseID, userID uuid.UUID) (*model.ListLessonResponse, error)
	GetLessonDetail(ctx context.Context, courseID, lessonID, userID uuid.UUID) (*model.LessonDetailResponse, error)
	UpsertModuleProgress(ctx context.Context, courseID, moduleID, userID uuid.UUID, req model.UpsertModuleProgressRequest) (*model.ModuleProgressResponse, error)
	UpsertLessonProgress(ctx context.Context, courseID, lessonID, userID uuid.UUID, req model.UpsertLessonProgressRequest) (*model.LessonProgressResponse, error)
}

func (s *service) UpsertLessonProgress(ctx context.Context, courseID, lessonID, userID uuid.UUID, req model.UpsertLessonProgressRequest) (*model.LessonProgressResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", utils.ErrInvalidLessonProgress, err)
	}
	if req.Status != nil && *req.Status == model.StatusCompleted {
		return nil, utils.ErrLessonCompletionRequiresQuiz
	}

	progress, err := s.repository.UpsertLessonProgress(ctx, courseID, lessonID, userID, req)
	if err != nil {
		return nil, err
	}

	return &model.LessonProgressResponse{
		Success: true,
		Message: "progress pelajaran berhasil disimpan",
		Data:    *progress,
	}, nil
}

func (s *service) UpsertModuleProgress(ctx context.Context, courseID, moduleID, userID uuid.UUID, req model.UpsertModuleProgressRequest) (*model.ModuleProgressResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", utils.ErrInvalidModuleProgress, err)
	}

	progress, err := s.repository.UpsertModuleProgress(ctx, courseID, moduleID, userID, req)
	if err != nil {
		return nil, err
	}

	return &model.ModuleProgressResponse{
		Success: true,
		Message: "progress modul berhasil disimpan",
		Data:    *progress,
	}, nil
}

func (s *service) GetDetail(ctx context.Context, courseID, userID uuid.UUID) (*model.CourseDetailResponse, error) {
	course, err := s.repository.GetCourseDetail(ctx, courseID, userID)
	if err != nil {
		return nil, err
	}

	return &model.CourseDetailResponse{
		Success: true,
		Message: "berhasil mendapatkan detail kursus",
		Data:    *course,
	}, nil
}

func (s *service) GetLessonDetail(ctx context.Context, courseID, lessonID, userID uuid.UUID) (*model.LessonDetailResponse, error) {
	lesson, err := s.repository.GetLessonDetail(ctx, courseID, lessonID)
	if err != nil {
		return nil, err
	}
	questionSets, err := s.questionRepository.ListQuestionSets(ctx, courseID, lessonID, userID)
	if err != nil {
		return nil, err
	}
	lesson.QuestionSets = questionSets

	return &model.LessonDetailResponse{
		Success: true,
		Message: "berhasil mendapatkan detail pelajaran",
		Data:    *lesson,
	}, nil
}

func (s *service) GetLessons(ctx context.Context, courseID, userID uuid.UUID) (*model.ListLessonResponse, error) {
	lessons, err := s.repository.ListLessons(ctx, courseID, userID)
	if err != nil {
		return nil, err
	}

	return &model.ListLessonResponse{
		Success: true,
		Message: "berhasil mendapatkan daftar modul dan pelajaran",
		Data:    lessons,
	}, nil
}

type service struct {
	repository         Repository
	questionRepository question.Repository
}

func NewService(repository Repository, questionRepository question.Repository) Service {
	return &service{repository: repository, questionRepository: questionRepository}
}

func (s *service) GetList(ctx context.Context) (*model.ListCourseResponse, error) {
	courses, err := s.repository.List(ctx)
	if err != nil {
		return nil, err
	}

	return &model.ListCourseResponse{
		Success: true,
		Message: "berhasil mendapatkan daftar kursus",
		Data:    courses,
	}, nil
}
