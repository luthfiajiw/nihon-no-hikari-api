package model

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type UpsertModuleProgressRequest struct {
	Status    *ModuleStatus `json:"status"`
	BestScore *int16        `json:"best_score"`
}

func (r UpsertModuleProgressRequest) Validate() error {
	if r.Status == nil && r.BestScore == nil {
		return errors.New("status atau best_score wajib diisi")
	}
	if r.Status != nil {
		switch *r.Status {
		case ModuleStatusLocked, ModuleStatusUnlocked, ModuleStatusInProgress, ModuleStatusCompleted:
		default:
			return errors.New("status harus locked, unlocked, in_progress, atau completed")
		}
	}
	if r.BestScore != nil && (*r.BestScore < 0 || *r.BestScore > 100) {
		return errors.New("best_score harus di antara 0 dan 100")
	}
	return nil
}

type ModuleProgress struct {
	UserID      uuid.UUID    `json:"user_id"`
	ModuleID    uuid.UUID    `json:"module_id"`
	Status      ModuleStatus `json:"status"`
	BestScore   *int16       `json:"best_score"`
	UnlockedAt  *time.Time   `json:"unlocked_at"`
	CompletedAt *time.Time   `json:"completed_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

type ModuleProgressResponse struct {
	Success bool           `json:"success"`
	Message string         `json:"message"`
	Data    ModuleProgress `json:"data"`
}
