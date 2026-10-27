package course

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"nihon-no-hikari-api/internal/course/model"
	"nihon-no-hikari-api/pkg/contenthtml"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ContentService interface {
	AdminCatalog(context.Context, *uuid.UUID) (model.CatalogResponse, error)
	AdminLevels(context.Context) (model.LevelsResponse, error)
	PreviewLessonContent(context.Context, json.RawMessage) (model.PreviewResponse, error)
	ListAdminContent(context.Context, string) (model.ContentListResponse, error)
	AdminContentDetail(context.Context, string, uuid.UUID) (model.ContentDetailResponse, error)
	CreateAdminContent(context.Context, string, model.CoreInput) (model.CreateResponse, error)
	UpdateAdminContent(context.Context, string, uuid.UUID, model.CoreInput) (model.UpdateResponse, error)
	PublishAdminContent(context.Context, string, uuid.UUID) (model.PublishResponse, error)
	RemoveAdminContent(context.Context, string, uuid.UUID) (model.RemoveResponse, error)
	RestoreAdminContent(context.Context, string, uuid.UUID) (model.StatusResponse, error)
	ReorderAdminContent(context.Context, string, model.ReorderRequest) (model.StatusResponse, error)
}

// execTx adapts the shared transaction runner to a transaction-bound repository.
func (s *service) execTx(ctx context.Context, fn func(ContentRepository) error) error {
	err := s.tx.Exec(ctx, func(tx pgx.Tx) error {
		return fn(s.repository.UseTx(tx))
	})
	// Commit-time constraints must retain the same application error mapping.
	return databaseError(err)
}

func validKind(kind string) bool {
	switch kind {
	case "course", "module", "lesson":
		return true
	}
	return false
}

func (s *service) AdminCatalog(ctx context.Context, courseID *uuid.UUID) (model.CatalogResponse, error) {
	data, err := s.repository.Catalog(ctx, courseID)
	if err == nil && courseID == nil {
		data.QuestionSets, err = s.questionRepository.CatalogQuestionSets(ctx)
	}
	return model.CatalogResponse{Success: true, Data: data}, err
}

func (s *service) AdminLevels(ctx context.Context) (model.LevelsResponse, error) {
	items, err := s.repository.Levels(ctx)
	return model.LevelsResponse{Success: true, Data: items}, err
}

func (s *service) PreviewLessonContent(ctx context.Context, raw json.RawMessage) (model.PreviewResponse, error) {
	clean, err := contenthtml.JSON(raw)
	if err != nil {
		return model.PreviewResponse{}, inputMessage("konten lesson harus berupa objek dengan content dan tips")
	}
	return model.PreviewResponse{Success: true, Data: json.RawMessage(clean)}, nil
}

func (s *service) ListAdminContent(ctx context.Context, kind string) (model.ContentListResponse, error) {
	if !validKind(kind) {
		return model.ContentListResponse{}, inputMessage("jenis konten tidak valid")
	}
	items, err := s.repository.ListContent(ctx, kind)
	return model.ContentListResponse{Success: true, Data: items}, err
}

func (s *service) AdminContentDetail(ctx context.Context, kind string, id uuid.UUID) (model.ContentDetailResponse, error) {
	if !validKind(kind) {
		return model.ContentDetailResponse{}, inputMessage("jenis konten tidak valid")
	}
	live, err := s.repository.Content(ctx, kind, id)
	if err != nil {
		return model.ContentDetailResponse{}, err
	}
	if kind == "lesson" {
		var fields map[string]json.RawMessage
		if json.Unmarshal(live, &fields) == nil {
			if clean, err := contenthtml.JSON(fields["content"]); err == nil {
				fields["content"] = clean
				live, _ = json.Marshal(fields)
			}
		}
	}
	draft, err := s.repository.Draft(ctx, kind, id)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return model.ContentDetailResponse{}, err
	}
	return model.ContentDetailResponse{Success: true, Data: live, Draft: draft}, nil
}

func (s *service) CreateAdminContent(ctx context.Context, kind string, v model.CoreInput) (model.CreateResponse, error) {
	if !validKind(kind) {
		return model.CreateResponse{}, inputMessage("jenis konten tidak valid")
	}
	if err := v.Validate(kind); err != nil {
		return model.CreateResponse{}, invalidInput(err)
	}
	id := uuid.New()
	err := s.execTx(ctx, func(r ContentRepository) error {
		if err := validatePrerequisites(ctx, r, kind, id, v.Prerequisites(kind)); err != nil {
			return err
		}
		return r.SaveCore(ctx, kind, id, v, true)
	})
	return model.CreateResponse{Success: true, ID: id}, err
}

func (s *service) UpdateAdminContent(ctx context.Context, kind string, id uuid.UUID, v model.CoreInput) (model.UpdateResponse, error) {
	if !validKind(kind) {
		return model.UpdateResponse{}, inputMessage("jenis konten tidak valid")
	}
	if err := v.Validate(kind); err != nil {
		return model.UpdateResponse{}, invalidInput(err)
	}
	var published bool
	err := s.execTx(ctx, func(r ContentRepository) error {
		state, err := r.ContentState(ctx, kind, id)
		if err != nil {
			return err
		}
		published = state.Published
		if err := validatePrerequisites(ctx, r, kind, id, v.Prerequisites(kind)); err != nil {
			return err
		}
		if published {
			raw, err := json.Marshal(v)
			if err != nil {
				return err
			}
			return r.SaveDraft(ctx, kind, id, raw)
		}
		return r.SaveCore(ctx, kind, id, v, false)
	})
	return model.UpdateResponse{Success: true, Draft: published}, err
}

func (s *service) PublishAdminContent(ctx context.Context, kind string, id uuid.UUID) (model.PublishResponse, error) {
	if !validKind(kind) {
		return model.PublishResponse{}, inputMessage("jenis konten tidak valid")
	}
	err := s.execTx(ctx, func(r ContentRepository) error {
		state, err := r.ContentState(ctx, kind, id)
		if err != nil {
			return err
		}
		if state.Archived {
			return inputMessage("pulihkan konten sebelum diterbitkan")
		}
		raw, err := r.Draft(ctx, kind, id)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		if err == nil {
			v, err := parseCore(kind, raw)
			if err != nil {
				return invalidInput(err)
			}
			if err := validatePrerequisites(ctx, r, kind, id, v.Prerequisites(kind)); err != nil {
				return err
			}
			if err := r.SaveCore(ctx, kind, id, v, false); err != nil {
				return err
			}
		}
		if err := r.SetPublished(ctx, kind, id, true); err != nil {
			return err
		}
		return r.DeleteDraft(ctx, kind, id)
	})
	return model.PublishResponse{Success: true}, err
}

func (s *service) RemoveAdminContent(ctx context.Context, kind string, id uuid.UUID) (model.RemoveResponse, error) {
	if !validKind(kind) {
		return model.RemoveResponse{}, inputMessage("jenis konten tidak valid")
	}
	var archived bool
	err := s.execTx(ctx, func(r ContentRepository) error {
		if _, err := r.ContentState(ctx, kind, id); err != nil {
			return err
		}
		var err error
		archived, err = r.HasHistory(ctx, kind, id)
		if err != nil {
			return err
		}
		if !archived {
			deleted, err := r.DeleteContent(ctx, kind, id)
			if err != nil {
				return err
			}
			archived = !deleted
		}
		if archived {
			if err := r.Archive(ctx, kind, id); err != nil {
				return err
			}
		}
		return r.DeleteDraft(ctx, kind, id)
	})
	return model.RemoveResponse{Success: true, Archived: archived}, err
}

func (s *service) RestoreAdminContent(ctx context.Context, kind string, id uuid.UUID) (model.StatusResponse, error) {
	if !validKind(kind) {
		return model.StatusResponse{}, inputMessage("jenis konten tidak valid")
	}
	err := s.repository.RestoreContent(ctx, kind, id)
	return model.StatusResponse{Success: true}, err
}

func (s *service) ReorderAdminContent(ctx context.Context, kind string, input model.ReorderRequest) (model.StatusResponse, error) {
	if err := input.Validate(kind); err != nil {
		return model.StatusResponse{}, invalidInput(err)
	}
	err := s.execTx(ctx, func(r ContentRepository) error {
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
	return model.StatusResponse{Success: true}, err
}

func parseCore(kind string, raw []byte) (model.CoreInput, error) {
	var v model.CoreInput
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, decodeError(err)
	}
	err := v.Validate(kind)
	return v, err
}

func validatePrerequisites(ctx context.Context, r ContentRepository, kind string, id uuid.UUID, required []uuid.UUID) error {
	if len(required) == 0 {
		return nil
	}
	if kind != "module" && kind != "lesson" {
		return errInvalidPrerequisites
	}
	seen := make(map[uuid.UUID]bool, len(required))
	for _, requiredID := range required {
		if requiredID == uuid.Nil || requiredID == id || seen[requiredID] {
			return fmt.Errorf("%w: pilih konten lain tanpa duplikasi", errInvalidPrerequisites)
		}
		seen[requiredID] = true
	}
	count, cycle, err := r.PrerequisiteState(ctx, kind, id, required)
	if err != nil {
		return err
	}
	if count != len(required) {
		return fmt.Errorf("%w: konten yang dipilih tidak ditemukan", errInvalidPrerequisites)
	}
	if cycle {
		return fmt.Errorf("%w: hubungan prasyarat tidak boleh melingkar", errInvalidPrerequisites)
	}
	return nil
}
