package admin

import (
	"context"
	"encoding/json"
	"errors"

	"nihon-no-hikari-api/internal/admin/model"

	"github.com/google/uuid"
)

func sameUUID(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func parseSet(raw []byte, publish bool) (setInput, error) {
	var v setInput
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, decodeError(err)
	}
	err := v.Validate(publish)
	return v, err
}

func samePlacement(v setInput, state model.QuestionSetState) bool {
	return v.Kind == state.Kind && (state.Skill == nil || v.Skill == *state.Skill) && sameUUID(v.LessonID, state.LessonID) && sameUUID(v.ModuleID, state.ModuleID)
}

func (s *service) CreateQuestionSet(ctx context.Context, v setInput) (CreateResponse, error) {
	if err := v.Validate(false); err != nil {
		return CreateResponse{}, invalidInput(err)
	}
	id := uuid.New()
	err := s.execTx(ctx, func(r Repository) error {
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

func (s *service) UpdateQuestionSet(ctx context.Context, id uuid.UUID, v setInput) (UpdateResponse, error) {
	if err := v.Validate(false); err != nil {
		return UpdateResponse{}, invalidInput(err)
	}
	var published bool
	err := s.execTx(ctx, func(r Repository) error {
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

func (s *service) publishQuestionSet(ctx context.Context, originalID uuid.UUID) (PublishResponse, error) {
	id := originalID
	err := s.execTx(ctx, func(r Repository) error {
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
