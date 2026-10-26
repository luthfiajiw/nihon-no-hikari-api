package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"nihon-no-hikari-api/pkg/contenthtml"
	"nihon-no-hikari-api/pkg/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Service interface {
	Catalog(context.Context, *uuid.UUID) (CatalogResponse, error)
	Levels(context.Context) (LevelsResponse, error)
	Templates(context.Context) (TemplatesResponse, error)
	PreviewContent(context.Context, json.RawMessage) (PreviewResponse, error)
	ListContent(context.Context, string) (ContentListResponse, error)
	Detail(context.Context, string, uuid.UUID) (ContentDetailResponse, error)
	CreateContent(context.Context, string, coreInput) (CreateResponse, error)
	UpdateContent(context.Context, string, uuid.UUID, coreInput) (UpdateResponse, error)
	PublishContent(context.Context, string, uuid.UUID) (PublishResponse, error)
	RemoveContent(context.Context, string, uuid.UUID) (RemoveResponse, error)
	RestoreContent(context.Context, string, uuid.UUID) (StatusResponse, error)
	CreateQuestionSet(context.Context, setInput) (CreateResponse, error)
	UpdateQuestionSet(context.Context, uuid.UUID, setInput) (UpdateResponse, error)
	Reorder(context.Context, string, ReorderRequest) (StatusResponse, error)
	Template(context.Context, uuid.UUID) (StrokeTemplateResponse, error)
	PreviewTemplate(context.Context, StrokeTemplateInput) (StrokeTemplatePreviewResponse, error)
	CreateTemplate(context.Context, StrokeTemplateInput) (StrokeTemplateResponse, error)
	UpdateTemplate(context.Context, uuid.UUID, StrokeTemplateInput) (StrokeTemplateResponse, error)
}

type service struct {
	tx         db.WithTx
	repository Repository
}

func NewService(tx db.WithTx, repository Repository) Service {
	return &service{tx: tx, repository: repository}
}

// execTx adapts the shared transaction runner to a transaction-bound repository.
func (s *service) execTx(ctx context.Context, fn func(Repository) error) error {
	err := s.tx.Exec(ctx, func(tx pgx.Tx) error {
		return fn(s.repository.UseTx(tx))
	})
	// Commit-time constraints must retain the same application error mapping.
	return databaseError(err)
}

func validKind(kind string) bool {
	switch kind {
	case "course", "module", "lesson", "question_set":
		return true
	}
	return false
}

func (s *service) Catalog(ctx context.Context, courseID *uuid.UUID) (CatalogResponse, error) {
	data, err := s.repository.Catalog(ctx, courseID)
	return CatalogResponse{Success: true, Data: data}, err
}

func (s *service) Levels(ctx context.Context) (LevelsResponse, error) {
	items, err := s.repository.Levels(ctx)
	return LevelsResponse{Success: true, Data: items}, err
}

func (s *service) Templates(ctx context.Context) (TemplatesResponse, error) {
	items, err := s.repository.Templates(ctx)
	return TemplatesResponse{Success: true, Data: items}, err
}

func (s *service) PreviewContent(ctx context.Context, raw json.RawMessage) (PreviewResponse, error) {
	clean, err := contenthtml.JSON(raw)
	if err != nil {
		return PreviewResponse{}, inputMessage("konten lesson harus berupa objek dengan content dan tips")
	}
	return PreviewResponse{Success: true, Data: json.RawMessage(clean)}, nil
}

func (s *service) ListContent(ctx context.Context, kind string) (ContentListResponse, error) {
	if !validKind(kind) {
		return ContentListResponse{}, inputMessage("jenis konten tidak valid")
	}
	items, err := s.repository.ListContent(ctx, kind)
	return ContentListResponse{Success: true, Data: items}, err
}

func (s *service) Detail(ctx context.Context, kind string, id uuid.UUID) (ContentDetailResponse, error) {
	if !validKind(kind) {
		return ContentDetailResponse{}, inputMessage("jenis konten tidak valid")
	}
	live, err := s.repository.Content(ctx, kind, id)
	if err != nil {
		return ContentDetailResponse{}, err
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
	if kind == "question_set" {
		live, err = s.repository.WithQuestions(ctx, live, id)
		if err != nil {
			return ContentDetailResponse{}, err
		}
	}
	draft, err := s.repository.Draft(ctx, kind, id)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return ContentDetailResponse{}, err
	}
	return ContentDetailResponse{Success: true, Data: live, Draft: draft}, nil
}

func parseCore(kind string, raw []byte) (coreInput, error) {
	var v coreInput
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, decodeError(err)
	}
	err := v.Validate(kind)
	return v, err
}

func (s *service) CreateContent(ctx context.Context, kind string, v coreInput) (CreateResponse, error) {
	if !validKind(kind) || kind == "question_set" {
		return CreateResponse{}, inputMessage("jenis konten tidak valid")
	}
	if err := v.Validate(kind); err != nil {
		return CreateResponse{}, invalidInput(err)
	}
	id := uuid.New()
	err := s.execTx(ctx, func(r Repository) error {
		if err := validatePrerequisites(ctx, r, kind, id, v.Prerequisites(kind)); err != nil {
			return err
		}
		return r.SaveCore(ctx, kind, id, v, true)
	})
	return CreateResponse{Success: true, ID: id}, err
}

func (s *service) UpdateContent(ctx context.Context, kind string, id uuid.UUID, v coreInput) (UpdateResponse, error) {
	if !validKind(kind) || kind == "question_set" {
		return UpdateResponse{}, inputMessage("jenis konten tidak valid")
	}
	if err := v.Validate(kind); err != nil {
		return UpdateResponse{}, invalidInput(err)
	}
	var published bool
	err := s.execTx(ctx, func(r Repository) error {
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
	return UpdateResponse{Success: true, Draft: published}, err
}

func (s *service) PublishContent(ctx context.Context, kind string, id uuid.UUID) (PublishResponse, error) {
	if !validKind(kind) {
		return PublishResponse{}, inputMessage("jenis konten tidak valid")
	}
	if kind == "question_set" {
		return s.publishQuestionSet(ctx, id)
	}
	err := s.execTx(ctx, func(r Repository) error {
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
	return PublishResponse{Success: true}, err
}

func (s *service) RemoveContent(ctx context.Context, kind string, id uuid.UUID) (RemoveResponse, error) {
	if !validKind(kind) {
		return RemoveResponse{}, inputMessage("jenis konten tidak valid")
	}
	var archived bool
	err := s.execTx(ctx, func(r Repository) error {
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
	return RemoveResponse{Success: true, Archived: archived}, err
}

func (s *service) RestoreContent(ctx context.Context, kind string, id uuid.UUID) (StatusResponse, error) {
	if !validKind(kind) {
		return StatusResponse{}, inputMessage("jenis konten tidak valid")
	}
	err := s.repository.RestoreContent(ctx, kind, id)
	return StatusResponse{Success: true}, err
}

func validatePrerequisites(ctx context.Context, r Repository, kind string, id uuid.UUID, required []uuid.UUID) error {
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
