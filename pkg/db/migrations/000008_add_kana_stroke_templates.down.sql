BEGIN;
-- Existing writing questions must be removed before downgrading.
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM public.questions WHERE stroke_template_id IS NOT NULL) THEN
    RAISE EXCEPTION 'remove writing questions before downgrading migration 000008';
  END IF;
END $$;
ALTER TABLE public.questions DROP CONSTRAINT questions_stroke_template_type_check;
ALTER TABLE public.questions DROP CONSTRAINT questions_stimulus_present_check;
ALTER TABLE public.questions ADD CONSTRAINT questions_stimulus_present_check CHECK (
  NULLIF(BTRIM(stimulus_text), '') IS NOT NULL OR NULLIF(BTRIM(stimulus_media_url), '') IS NOT NULL
);
ALTER TABLE public.questions DROP COLUMN stroke_template_id;
DROP TABLE public.kana_stroke_templates;
DROP FUNCTION public.prevent_kana_stroke_template_mutation();
CREATE OR REPLACE FUNCTION public.validate_exam_question(p_question_id uuid)
RETURNS void LANGUAGE plpgsql AS $$
DECLARE
  question_record record;
  option_count integer;
  correct_option_count integer;
BEGIN
  SELECT q.id, q.question_type, q.skill, q.stimulus_text,
    qs.skill AS question_set_skill, qs.is_published
  INTO question_record
  FROM public.questions q JOIN public.question_sets qs ON qs.id = q.question_set_id
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
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = FORMAT('multiple-choice question %s must have at least two options and exactly one correct option', question_record.id);
      END IF;
    WHEN 'stroke_writing'::public.question_type THEN
      IF option_count <> 0 OR NULLIF(BTRIM(question_record.stimulus_text), '') IS NULL THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = FORMAT('stroke-writing question %s must have text stimulus and no options', question_record.id);
      END IF;
      RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = FORMAT('stroke-writing question %s cannot be published before a server-side evaluator is available', question_record.id);
  END CASE;
END;
$$;
COMMIT;
