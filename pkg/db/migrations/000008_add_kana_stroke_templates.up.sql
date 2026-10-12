BEGIN;

CREATE TABLE public.kana_stroke_templates (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  script text NOT NULL CHECK (script IN ('hiragana', 'katakana')),
  character varchar(4) NOT NULL,
  romaji varchar(8) NOT NULL,
  source_version text NOT NULL,
  stroke_data jsonb NOT NULL,
  CONSTRAINT kana_stroke_templates_version_key UNIQUE (script, character, source_version),
  CONSTRAINT kana_stroke_templates_data_check CHECK (
    jsonb_typeof(stroke_data) = 'object'
    AND stroke_data->>'view_box' = '109'
    AND jsonb_typeof(stroke_data->'strokes') = 'array'
    AND jsonb_array_length(stroke_data->'strokes') > 0
  )
);

CREATE FUNCTION public.prevent_kana_stroke_template_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION USING ERRCODE = '55000',
    MESSAGE = 'stroke templates are immutable; insert a new source version';
END;
$$;

CREATE TRIGGER kana_stroke_templates_immutable
BEFORE UPDATE OR DELETE ON public.kana_stroke_templates
FOR EACH ROW EXECUTE FUNCTION public.prevent_kana_stroke_template_mutation();

ALTER TABLE public.questions
  ADD COLUMN stroke_template_id uuid REFERENCES public.kana_stroke_templates(id) ON DELETE RESTRICT;

ALTER TABLE public.questions DROP CONSTRAINT questions_stimulus_present_check;
ALTER TABLE public.questions ADD CONSTRAINT questions_stimulus_present_check CHECK (
  NULLIF(BTRIM(stimulus_text), '') IS NOT NULL
  OR NULLIF(BTRIM(stimulus_media_url), '') IS NOT NULL
  OR (question_type = 'stroke_writing' AND stroke_template_id IS NOT NULL)
);
ALTER TABLE public.questions ADD CONSTRAINT questions_stroke_template_type_check CHECK (
  (question_type = 'stroke_writing' AND skill = 'writing' AND stroke_template_id IS NOT NULL)
  OR (question_type <> 'stroke_writing' AND stroke_template_id IS NULL)
);

CREATE OR REPLACE FUNCTION public.validate_exam_question(p_question_id uuid)
RETURNS void LANGUAGE plpgsql AS $$
DECLARE
  question_record record;
  option_count integer;
  correct_option_count integer;
BEGIN
  SELECT q.id, q.question_type, q.skill, q.stimulus_text, q.stroke_template_id,
    qs.skill AS question_set_skill, qs.is_published
  INTO question_record
  FROM public.questions q
  JOIN public.question_sets qs ON qs.id = q.question_set_id
  WHERE q.id = p_question_id;

  IF NOT FOUND OR NOT question_record.is_published THEN RETURN; END IF;
  IF question_record.skill NOT IN ('reading'::public.skill_type, 'writing'::public.skill_type) THEN
    RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = FORMAT('published question %s uses unsupported skill', question_record.id);
  END IF;
  IF question_record.question_set_skill IS NOT NULL AND question_record.skill <> question_record.question_set_skill THEN
    RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = FORMAT('question %s skill does not match its question set skill', question_record.id);
  END IF;

  SELECT COUNT(*)::integer, COUNT(*) FILTER (WHERE is_correct)::integer
  INTO option_count, correct_option_count
  FROM public.question_options WHERE question_id = question_record.id;

  CASE question_record.question_type
    WHEN 'multiple_choice'::public.question_type THEN
      IF option_count < 2 OR correct_option_count <> 1 THEN
        RAISE EXCEPTION USING ERRCODE = '23514',
          MESSAGE = FORMAT('multiple-choice question %s must have at least two options and exactly one correct option', question_record.id);
      END IF;
    WHEN 'stroke_writing'::public.question_type THEN
      IF question_record.skill <> 'writing' OR question_record.stroke_template_id IS NULL OR option_count <> 0 THEN
        RAISE EXCEPTION USING ERRCODE = '23514',
          MESSAGE = FORMAT('stroke-writing question %s needs a writing template and no options', question_record.id);
      END IF;
  END CASE;
END;
$$;

COMMIT;
