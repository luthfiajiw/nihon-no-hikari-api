package admin

import (
	"context"
	"errors"
	"strings"

	"nihon-no-hikari-api/pkg/kanastroke"

	"github.com/google/uuid"
)

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
	err := s.execTx(ctx, func(r Repository) error {
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
