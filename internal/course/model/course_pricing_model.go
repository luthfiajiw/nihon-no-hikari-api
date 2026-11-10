package model

import (
	"github.com/google/uuid"
)

type PricingPlan struct {
	ID          uuid.UUID `json:"id" format:"uuid"`
	Code        string    `json:"code" example:"N5"`
	Name        string    `json:"name" example:"JLPT N5"`
	Tag         *string   `json:"tag,omitempty" example:"Favorit Pemula"`
	Description *string   `json:"description"`
	Price       int32     `json:"price" example:"49000"`
	Points      []string  `json:"points"`
	OrderIndex  int16     `json:"order_index"`
} // @name internal_course.PricingPlan

type PricingLesson struct {
	ID         uuid.UUID `json:"id" format:"uuid"`
	Slug       string    `json:"slug" example:"pengenalan-huruf-hiragana"`
	Title      string    `json:"title" example:"Pengenalan Huruf Hiragana & Aturan Penulisan"`
	OrderIndex int16     `json:"order_index"`
} // @name internal_course.PricingLesson

type PricingModule struct {
	ID               uuid.UUID       `json:"id" format:"uuid"`
	Slug             string          `json:"slug" example:"fondasi-aksara-jepang"`
	Title            string          `json:"title" example:"Fondasi Aksara Jepang"`
	Description      *string         `json:"description"`
	EstimatedMinutes int32           `json:"estimated_minutes"`
	LessonCount      int32           `json:"lesson_count"`
	OrderIndex       int16           `json:"order_index"`
	Lessons          []PricingLesson `json:"lessons"`
} // @name internal_course.PricingModule

type PricingDetail struct {
	ID           uuid.UUID       `json:"id" format:"uuid"`
	Code         string          `json:"code" example:"N5"`
	Name         string          `json:"name" example:"JLPT N5"`
	Tag          *string         `json:"tag,omitempty" example:"Favorit Pemula"`
	Description  *string         `json:"description"`
	Price        int32           `json:"price" example:"49000"`
	Points       []string        `json:"points"`
	OrderIndex   int16           `json:"order_index"`
	TotalModules int32           `json:"total_modules"`
	TotalLessons int32           `json:"total_lessons"`
	Modules      []PricingModule `json:"modules"`
} // @name internal_course.PricingDetail

type PricingListResponse struct {
	Success bool          `json:"success" example:"true"`
	Data    []PricingPlan `json:"data"`
} // @name internal_course.PricingListResponse

type PricingDetailResponse struct {
	Success bool          `json:"success" example:"true"`
	Data    PricingDetail `json:"data"`
} // @name internal_course.PricingDetailResponse
