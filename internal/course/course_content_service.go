package course

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"nihon-no-hikari-api/internal/course/model"
	"nihon-no-hikari-api/pkg/contenthtml"
	"nihon-no-hikari-api/pkg/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ContentService interface {
	AdminCatalog(context.Context, *uuid.UUID) (model.CatalogResponse, error)
	AdminLevels(context.Context) (model.LevelsResponse, error)
	AdminLevelsPaginated(context.Context, model.AdminListFilter) (model.LevelsResponse, error)
	PublicLevels(context.Context) (model.LevelsResponse, error)
	AdminLevelDetail(context.Context, uuid.UUID) (model.LevelResponse, error)
	CreateAdminLevel(context.Context, model.LevelInput) (model.LevelResponse, error)
	UpdateAdminLevel(context.Context, uuid.UUID, model.LevelInput) (model.LevelResponse, error)
	DeleteAdminLevel(context.Context, uuid.UUID) (model.StatusResponse, error)
	UpdateAdminLevelPrice(context.Context, uuid.UUID, model.UpdateLevelPriceRequest) (model.LevelResponse, error)
	PreviewLessonContent(context.Context, json.RawMessage) (model.PreviewResponse, error)
	ListAdminContent(context.Context, string) (model.ContentListResponse, error)
	ListAdminContentPaginated(context.Context, string, model.AdminListFilter) (model.ContentListResponse, error)
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
	if err != nil {
		return model.LevelsResponse{}, err
	}
	meta := utils.BuildPaginationMeta(utils.PaginationParams{Paginated: false}, len(items))
	return model.LevelsResponse{Success: true, Data: items, Meta: &meta}, nil
}

func (s *service) AdminLevelsPaginated(ctx context.Context, filter model.AdminListFilter) (model.LevelsResponse, error) {
	items, total, err := s.repository.LevelsPaginated(ctx, filter)
	if err != nil {
		return model.LevelsResponse{}, err
	}
	meta := utils.BuildPaginationMeta(filter.Pagination, total)
	return model.LevelsResponse{Success: true, Data: items, Meta: &meta}, nil
}

func (s *service) PublicLevels(ctx context.Context) (model.LevelsResponse, error) {
	return s.AdminLevels(ctx)
}

func validateLevelInput(req model.LevelInput) (model.LevelInput, error) {
	code := strings.TrimSpace(req.Code)
	if code == "" || len(code) > 8 {
		return req, inputMessage("kode level harus diisi dan maksimal 8 karakter")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 100 {
		return req, inputMessage("nama level harus diisi dan maksimal 100 karakter")
	}
	var tag *string
	if req.Tag != nil {
		t := strings.TrimSpace(*req.Tag)
		if t != "" {
			if len(t) > 40 {
				return req, inputMessage("tag level maksimal 40 karakter")
			}
			tag = &t
		}
	}
	var desc *string
	if req.Description != nil {
		d := strings.TrimSpace(*req.Description)
		if d != "" {
			desc = &d
		}
	}
	if req.OrderIndex == nil || *req.OrderIndex < 0 || *req.OrderIndex > 32767 {
		return req, inputMessage("urutan level harus antara 0 dan 32767")
	}
	if req.Price == nil || *req.Price < 0 || *req.Price > 100_000_000 {
		return req, inputMessage("harga level harus antara 0 dan 100000000")
	}
	points := make([]string, 0, len(req.Points))
	for _, p := range req.Points {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			if len(trimmed) > 160 {
				return req, inputMessage("setiap poin maksimal 160 karakter")
			}
			points = append(points, trimmed)
		}
	}
	if len(points) > 20 {
		return req, inputMessage("maksimal 20 poin per level")
	}
	req.Code = code
	req.Name = name
	req.Tag = tag
	req.Description = desc
	req.Points = points
	return req, nil
}

func (s *service) AdminLevelDetail(ctx context.Context, id uuid.UUID) (model.LevelResponse, error) {
	item, err := s.repository.Level(ctx, id)
	if err != nil {
		return model.LevelResponse{}, err
	}
	return model.LevelResponse{Success: true, Data: item}, nil
}

func (s *service) CreateAdminLevel(ctx context.Context, req model.LevelInput) (model.LevelResponse, error) {
	valid, err := validateLevelInput(req)
	if err != nil {
		return model.LevelResponse{}, err
	}
	id := uuid.New()
	item, err := s.repository.CreateLevel(ctx, id, valid)
	if err != nil {
		return model.LevelResponse{}, err
	}
	return model.LevelResponse{Success: true, Data: item}, nil
}

func (s *service) UpdateAdminLevel(ctx context.Context, id uuid.UUID, req model.LevelInput) (model.LevelResponse, error) {
	valid, err := validateLevelInput(req)
	if err != nil {
		return model.LevelResponse{}, err
	}
	item, err := s.repository.UpdateLevel(ctx, id, valid)
	if err != nil {
		return model.LevelResponse{}, err
	}
	return model.LevelResponse{Success: true, Data: item}, nil
}

func (s *service) DeleteAdminLevel(ctx context.Context, id uuid.UUID) (model.StatusResponse, error) {
	if err := s.repository.DeleteLevel(ctx, id); err != nil {
		return model.StatusResponse{}, err
	}
	return model.StatusResponse{Success: true}, nil
}

func (s *service) UpdateAdminLevelPrice(ctx context.Context, id uuid.UUID, req model.UpdateLevelPriceRequest) (model.LevelResponse, error) {
	if req.Price == nil || *req.Price < 0 || *req.Price > 100_000_000 {
		return model.LevelResponse{}, inputMessage("harga level harus antara 0 dan 100000000")
	}
	item, err := s.repository.UpdateLevelPrice(ctx, id, int32(*req.Price))
	if err != nil {
		return model.LevelResponse{}, err
	}
	return model.LevelResponse{Success: true, Data: item}, nil
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
	if err != nil {
		return model.ContentListResponse{}, err
	}
	meta := utils.BuildPaginationMeta(utils.PaginationParams{Paginated: false}, len(items))
	return model.ContentListResponse{Success: true, Data: items, Meta: &meta}, nil
}

func (s *service) ListAdminContentPaginated(ctx context.Context, kind string, filter model.AdminListFilter) (model.ContentListResponse, error) {
	if !validKind(kind) {
		return model.ContentListResponse{}, inputMessage("jenis konten tidak valid")
	}
	items, total, err := s.repository.ListContentPaginated(ctx, kind, filter)
	if err != nil {
		return model.ContentListResponse{}, err
	}
	meta := utils.BuildPaginationMeta(filter.Pagination, total)
	return model.ContentListResponse{Success: true, Data: items, Meta: &meta}, nil
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
