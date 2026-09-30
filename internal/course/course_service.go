package course

import (
	"context"
	"nihon-no-hikari-api/internal/course/models"
)

type Service interface {
	GetList(ctx context.Context) (*models.ListCoursesRes, error)
}

type service struct {
	repository Repository
}

func NewService(repository Repository) Service {
	return &service{repository: repository}
}

func (s *service) GetList(ctx context.Context) (*models.ListCoursesRes, error) {
	courses, err := s.repository.List(ctx)
	if err != nil {
		return nil, err
	}

	return &models.ListCoursesRes{
		Success: true,
		Message: "berhasil mendapatkan daftar kursus",
		Data:    courses,
	}, nil
}
