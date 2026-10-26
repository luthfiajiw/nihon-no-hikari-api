package admin

import (
	"context"

	"github.com/google/uuid"
)

func (s *service) Reorder(ctx context.Context, kind string, input ReorderRequest) (StatusResponse, error) {
	if err := input.Validate(kind); err != nil {
		return StatusResponse{}, invalidInput(err)
	}
	err := s.execTx(ctx, func(r Repository) error {
		siblings, err := r.LockSiblings(ctx, kind, input.ParentID)
		if err != nil {
			return err
		}
		if len(siblings) != len(input.OrderedIDs) {
			return ErrStaleOrder
		}
		requested := make(map[uuid.UUID]bool, len(input.OrderedIDs))
		for _, id := range input.OrderedIDs {
			requested[id] = true
		}
		for _, id := range siblings {
			if !requested[id] {
				return ErrStaleOrder
			}
		}
		return r.ApplyOrder(ctx, kind, input)
	})
	return StatusResponse{Success: true}, err
}
