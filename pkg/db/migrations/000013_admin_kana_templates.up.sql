BEGIN;

-- Deploy with the old API stopped: backfill before templates become mutable.
ALTER TABLE public.attempt_answers ADD COLUMN writing_guide_snapshot jsonb;
UPDATE public.attempt_answers aa
SET writing_guide_snapshot = t.stroke_data || jsonb_build_object('character', t.character, 'romaji', t.romaji)
FROM public.questions q
JOIN public.kana_stroke_templates t ON t.id = q.stroke_template_id
WHERE aa.question_id = q.id AND q.question_type = 'stroke_writing';

ALTER TABLE public.attempt_answers ADD CONSTRAINT attempt_answers_writing_guide_snapshot_check CHECK (
  writing_guide_snapshot IS NULL OR (
    jsonb_typeof(writing_guide_snapshot) = 'object'
    AND writing_guide_snapshot->>'view_box' = '109'
    AND jsonb_typeof(writing_guide_snapshot->'strokes') = 'array'
    AND jsonb_array_length(writing_guide_snapshot->'strokes') > 0
    AND COALESCE(writing_guide_snapshot->>'character', '') <> ''
    AND COALESCE(writing_guide_snapshot->>'romaji', '') <> ''
  ) IS TRUE
);

CREATE OR REPLACE FUNCTION public.prevent_kana_stroke_template_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'stroke templates cannot be deleted';
  END IF;
  RETURN NEW;
END;
$$;

COMMIT;
