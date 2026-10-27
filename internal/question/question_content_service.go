package question

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"nihon-no-hikari-api/pkg/kanastroke"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ContentService interface {
	Templates(context.Context) (TemplatesResponse, error)
	CreateQuestionSet(context.Context, SetInput) (CreateResponse, error)
	UpdateQuestionSet(context.Context, uuid.UUID, SetInput) (UpdateResponse, error)
	Template(context.Context, uuid.UUID) (StrokeTemplateResponse, error)
	PreviewTemplate(context.Context, StrokeTemplateInput) (StrokeTemplatePreviewResponse, error)
	CreateTemplate(context.Context, StrokeTemplateInput) (StrokeTemplateResponse, error)
	UpdateTemplate(context.Context, uuid.UUID, StrokeTemplateInput) (StrokeTemplateResponse, error)
	ListAdminQuestionSets(context.Context) (ContentListResponse, error)
	AdminQuestionSetDetail(context.Context, uuid.UUID) (ContentDetailResponse, error)
	PublishQuestionSet(context.Context, uuid.UUID) (PublishResponse, error)
	RemoveQuestionSet(context.Context, uuid.UUID) (RemoveResponse, error)
	RestoreQuestionSet(context.Context, uuid.UUID) (StatusResponse, error)
}

// execTx adapts the shared transaction runner to a transaction-bound repository.
func (s *service) execTx(ctx context.Context, fn func(ContentRepository) error) error {
	err := s.tx.Exec(ctx, func(tx pgx.Tx) error {
		return fn(s.repository.UseTx(tx))
	})
	// Commit-time constraints must retain the same application error mapping.
	return databaseError(err)
}

func (s *service) Templates(ctx context.Context) (TemplatesResponse, error) {
	items, err := s.repository.Templates(ctx)
	return TemplatesResponse{Success: true, Data: items}, err
}

func (s *service) ListAdminQuestionSets(ctx context.Context) (ContentListResponse, error) {
	items, err := s.repository.ListContent(ctx, "question_set")
	return ContentListResponse{Success: true, Data: items}, err
}

func (s *service) AdminQuestionSetDetail(ctx context.Context, id uuid.UUID) (ContentDetailResponse, error) {
	live, err := s.repository.Content(ctx, "question_set", id)
	if err != nil {
		return ContentDetailResponse{}, err
	}
	live, err = s.repository.WithQuestions(ctx, live, id)
	if err != nil {
		return ContentDetailResponse{}, err
	}
	draft, err := s.repository.Draft(ctx, "question_set", id)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return ContentDetailResponse{}, err
	}
	return ContentDetailResponse{Success: true, Data: live, Draft: draft}, nil
}

func (s *service) RemoveQuestionSet(ctx context.Context, id uuid.UUID) (RemoveResponse, error) {
	var archived bool
	err := s.execTx(ctx, func(r ContentRepository) error {
		if _, err := r.ContentState(ctx, "question_set", id); err != nil {
			return err
		}
		var err error
		archived, err = r.HasHistory(ctx, "question_set", id)
		if err != nil {
			return err
		}
		if !archived {
			deleted, err := r.DeleteContent(ctx, "question_set", id)
			if err != nil {
				return err
			}
			archived = !deleted
		}
		if archived {
			if err := r.Archive(ctx, "question_set", id); err != nil {
				return err
			}
		}
		return r.DeleteDraft(ctx, "question_set", id)
	})
	return RemoveResponse{Success: true, Archived: archived}, err
}

func (s *service) RestoreQuestionSet(ctx context.Context, id uuid.UUID) (StatusResponse, error) {
	err := s.repository.RestoreContent(ctx, "question_set", id)
	return StatusResponse{Success: true}, err
}

func sameUUID(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func parseSet(raw []byte, publish bool) (SetInput, error) {
	var v SetInput
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, decodeError(err)
	}
	err := v.Validate(publish)
	return v, err
}

func samePlacement(v SetInput, state QuestionSetState) bool {
	return v.Kind == state.Kind && (state.Skill == nil || v.Skill == *state.Skill) && sameUUID(v.LessonID, state.LessonID) && sameUUID(v.ModuleID, state.ModuleID)
}

func (s *service) CreateQuestionSet(ctx context.Context, v SetInput) (CreateResponse, error) {
	if err := v.Validate(false); err != nil {
		return CreateResponse{}, invalidInput(err)
	}
	id := uuid.New()
	err := s.execTx(ctx, func(r ContentRepository) error {
		initial := v
		initial.Questions = nil
		if err := r.InsertSet(ctx, id, id, 1, initial); err != nil {
			return err
		}
		payload, err := json.Marshal(v)
		if err != nil {
			return err
		}
		return r.SaveDraft(ctx, "question_set", id, payload)
	})
	return CreateResponse{Success: true, ID: id}, err
}

func (s *service) UpdateQuestionSet(ctx context.Context, id uuid.UUID, v SetInput) (UpdateResponse, error) {
	if err := v.Validate(false); err != nil {
		return UpdateResponse{}, invalidInput(err)
	}
	var published bool
	err := s.execTx(ctx, func(r ContentRepository) error {
		state, err := r.QuestionSetState(ctx, id)
		if err != nil {
			return err
		}
		published = state.Published
		if published {
			if !samePlacement(v, state) {
				return inputMessage("jenis, skill, dan tempat question set yang sudah terbit tidak dapat diubah")
			}
		} else if state.HasAttempts {
			return &applicationError{kind: ErrConflict, message: "versi lama dengan attempt tidak dapat diubah"}
		}
		payload, err := json.Marshal(v)
		if err != nil {
			return err
		}
		return r.SaveDraft(ctx, "question_set", id, payload)
	})
	return UpdateResponse{Success: true, Draft: published}, err
}

func (s *service) PublishQuestionSet(ctx context.Context, originalID uuid.UUID) (PublishResponse, error) {
	id := originalID
	err := s.execTx(ctx, func(r ContentRepository) error {
		state, err := r.QuestionSetState(ctx, id)
		if err != nil {
			return err
		}
		if state.Archived {
			return inputMessage("pulihkan question set sebelum diterbitkan")
		}
		raw, err := r.Draft(ctx, "question_set", id)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		if errors.Is(err, ErrNotFound) {
			if state.Published {
				return nil
			}
			if state.HasAttempts {
				return r.SetPublished(ctx, "question_set", id, true)
			}
			live, err := r.Content(ctx, "question_set", id)
			if err != nil {
				return err
			}
			raw, err = r.WithQuestions(ctx, live, id)
			if err != nil {
				return err
			}
		}
		v, err := parseSet(raw, true)
		if err != nil {
			return invalidInput(err)
		}
		if state.Published {
			if !samePlacement(v, state) {
				return inputMessage("jenis, skill, dan tempat question set yang sudah terbit tidak dapat diubah")
			}
			if err := r.SetPublished(ctx, "question_set", id, false); err != nil {
				return err
			}
			id = uuid.New()
			if err := r.InsertSet(ctx, id, state.LineageID, state.Version+1, v); err != nil {
				return err
			}
		} else {
			if state.HasAttempts {
				return &applicationError{kind: ErrConflict, message: "versi lama dengan attempt tidak dapat diterbitkan ulang"}
			}
			if err := r.UpdateSet(ctx, id, v); err != nil {
				return err
			}
			if err := r.DeleteQuestions(ctx, id); err != nil {
				return err
			}
			if err := r.InsertQuestions(ctx, id, v); err != nil {
				return err
			}
		}
		if err := r.SetPublished(ctx, "question_set", id, true); err != nil {
			return err
		}
		return r.DeleteDraft(ctx, "question_set", originalID)
	})
	return PublishResponse{Success: true, ID: &id}, err
}

func templateValidationError(err error) error {
	if errors.Is(err, kanastroke.ErrTooLarge) {
		return &applicationError{kind: ErrPayloadTooLarge, message: err.Error()}
	}
	return invalidInput(err)
}

func convertTemplateInput(input StrokeTemplateInput) (StrokeTemplatePreview, error) {
	if input.SVGContent == nil || strings.TrimSpace(*input.SVGContent) == "" {
		return StrokeTemplatePreview{}, errors.New("file SVG KanjiVG wajib diunggah")
	}
	data, err := kanastroke.Convert(*input.SVGContent, input.Script, input.Character)
	return StrokeTemplatePreview{Script: input.Script, Character: input.Character, Romaji: input.Romaji,
		SourceVersion: input.SourceVersion, StrokeCount: len(data.Strokes), StrokeData: data}, err
}

func (s *service) Template(ctx context.Context, id uuid.UUID) (StrokeTemplateResponse, error) {
	item, err := s.repository.Template(ctx, id, false)
	return StrokeTemplateResponse{Success: true, Data: item}, err
}

func (s *service) PreviewTemplate(ctx context.Context, input StrokeTemplateInput) (StrokeTemplatePreviewResponse, error) {
	if err := input.Validate(); err != nil {
		return StrokeTemplatePreviewResponse{}, templateValidationError(err)
	}
	preview, err := convertTemplateInput(input)
	if err != nil {
		return StrokeTemplatePreviewResponse{}, templateValidationError(err)
	}
	return StrokeTemplatePreviewResponse{Success: true, Data: preview}, nil
}

func (s *service) CreateTemplate(ctx context.Context, input StrokeTemplateInput) (StrokeTemplateResponse, error) {
	if err := input.Validate(); err != nil {
		return StrokeTemplateResponse{}, templateValidationError(err)
	}
	preview, err := convertTemplateInput(input)
	if err != nil {
		return StrokeTemplateResponse{}, templateValidationError(err)
	}
	item := StrokeTemplateDetail{StrokeTemplate: StrokeTemplate{ID: uuid.New(), Script: input.Script, Character: input.Character, Romaji: input.Romaji, SourceVersion: input.SourceVersion, StrokeCount: preview.StrokeCount}, StrokeData: preview.StrokeData}
	err = s.repository.InsertTemplate(ctx, item)
	return StrokeTemplateResponse{Success: true, Data: item}, err
}

func (s *service) UpdateTemplate(ctx context.Context, id uuid.UUID, input StrokeTemplateInput) (StrokeTemplateResponse, error) {
	if err := input.Validate(); err != nil {
		return StrokeTemplateResponse{}, templateValidationError(err)
	}
	// Conversion runs before locking the row, as in the original implementation.
	var converted *kanastroke.Data
	if input.SVGContent != nil {
		preview, err := convertTemplateInput(input)
		if err != nil {
			return StrokeTemplateResponse{}, templateValidationError(err)
		}
		converted = &preview.StrokeData
	}
	var item StrokeTemplateDetail
	err := s.execTx(ctx, func(r ContentRepository) error {
		var err error
		item, err = r.Template(ctx, id, true)
		if err != nil {
			return err
		}
		if converted == nil && (input.Script != item.Script || input.Character != item.Character) {
			return inputMessage("unggah SVG pengganti saat script atau karakter berubah")
		}
		if converted != nil {
			item.StrokeData = *converted
		}
		item.Script, item.Character, item.Romaji, item.SourceVersion = input.Script, input.Character, input.Romaji, input.SourceVersion
		item.StrokeCount = len(item.StrokeData.Strokes)
		return r.UpdateTemplate(ctx, item)
	})
	return StrokeTemplateResponse{Success: true, Data: item}, err
}
