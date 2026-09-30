package course

import (
	"context"
	"nihon-no-hikari-api/internal/course/model"

	"github.com/google/uuid"
)

type Service interface {
	GetList(ctx context.Context) (*model.ListCourseResponse, error)
	GetLessons(ctx context.Context, courseID uuid.UUID) (*model.ListLessonResponse, error)
	GetLessonDetail(ctx context.Context, courseID, lessonID uuid.UUID) (*model.LessonDetailResponse, error)
}

func (s *service) GetLessonDetail(ctx context.Context, courseID, lessonID uuid.UUID) (*model.LessonDetailResponse, error) {
	lesson, err := s.repository.GetLessonDetail(ctx, courseID, lessonID)
	if err != nil {
		return nil, err
	}

	return &model.LessonDetailResponse{
		Success: true,
		Message: "berhasil mendapatkan detail pelajaran",
		Data:    *lesson,
	}, nil
}

func (s *service) GetLessons(ctx context.Context, courseID uuid.UUID) (*model.ListLessonResponse, error) {
	lessons, err := s.repository.ListLessons(ctx, courseID)
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
	repository Repository
}

func NewService(repository Repository) Service {
	return &service{repository: repository}
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
