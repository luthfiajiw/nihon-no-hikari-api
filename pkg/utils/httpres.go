package utils

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"
)

type SuccessRes struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type ErrorRes struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
}

type PageContext struct {
	Page      int `json:"page"`
	TotalPage int `json:"total_page"`
	PageSize  int `json:"page_size"`
}

type PaginationParams struct {
	Page      int
	Limit     int
	Search    string
	Paginated bool
}

type PaginationMeta struct {
	Page       int `json:"page" example:"1"`
	Limit      int `json:"limit" example:"10"`
	Total      int `json:"total" example:"42"`
	TotalPages int `json:"total_pages" example:"5"`
}

func ParsePaginationParams(c fiber.Ctx) PaginationParams {
	rawPage := strings.TrimSpace(c.Query("page"))
	rawLimit := strings.TrimSpace(c.Query("limit"))
	search := strings.TrimSpace(c.Query("search"))
	if search == "" {
		search = strings.TrimSpace(c.Query("q"))
	}

	paginated := rawPage != "" || rawLimit != ""
	page := 1
	if p, err := strconv.Atoi(rawPage); err == nil && p > 0 {
		page = p
	}
	limit := 10
	if l, err := strconv.Atoi(rawLimit); err == nil && l > 0 {
		if l > 100 {
			l = 100
		}
		limit = l
	}

	return PaginationParams{
		Page:      page,
		Limit:     limit,
		Search:    search,
		Paginated: paginated,
	}
}

func (p PaginationParams) Offset() int {
	if !p.Paginated || p.Page <= 1 {
		return 0
	}
	return (p.Page - 1) * p.Limit
}

func BuildPaginationMeta(p PaginationParams, total int) PaginationMeta {
	if !p.Paginated {
		limit := total
		if limit == 0 {
			limit = 10
		}
		return PaginationMeta{
			Page:       1,
			Limit:      limit,
			Total:      total,
			TotalPages: 1,
		}
	}
	totalPages := (total + p.Limit - 1) / p.Limit
	if totalPages < 1 {
		totalPages = 1
	}
	return PaginationMeta{
		Page:       p.Page,
		Limit:      p.Limit,
		Total:      total,
		TotalPages: totalPages,
	}
}

