package admin

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
)

func (r *dbRepository) Template(ctx context.Context, id uuid.UUID, lock bool) (StrokeTemplateDetail, error) {
	query := `SELECT id,script,character,romaji,source_version,stroke_data FROM public.kana_stroke_templates WHERE id=$1`
	if lock {
		query += ` FOR UPDATE`
	}
	var item StrokeTemplateDetail
	err := r.QueryRow(ctx, query, id).Scan(&item.ID, &item.Script, &item.Character, &item.Romaji, &item.SourceVersion, &item.StrokeData)
	item.StrokeCount = len(item.StrokeData.Strokes)
	return item, err
}

func (r *dbRepository) InsertTemplate(ctx context.Context, item StrokeTemplateDetail) error {
	strokeJSON, err := json.Marshal(item.StrokeData)
	if err != nil {
		return err
	}
	_, err = r.Exec(ctx, `INSERT INTO public.kana_stroke_templates(id,script,character,romaji,source_version,stroke_data) VALUES($1,$2,$3,$4,$5,$6::jsonb)`,
		item.ID, item.Script, item.Character, item.Romaji, item.SourceVersion, string(strokeJSON))
	return err
}

func (r *dbRepository) UpdateTemplate(ctx context.Context, item StrokeTemplateDetail) error {
	strokeJSON, err := json.Marshal(item.StrokeData)
	if err != nil {
		return err
	}
	_, err = r.Exec(ctx, `UPDATE public.kana_stroke_templates SET script=$2,character=$3,romaji=$4,source_version=$5,stroke_data=$6::jsonb WHERE id=$1`,
		item.ID, item.Script, item.Character, item.Romaji, item.SourceVersion, string(strokeJSON))
	return err
}
