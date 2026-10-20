BEGIN;
-- Old API versions grade using live templates. Do not switch active attempts.
DO $$ BEGIN
  IF EXISTS (
    SELECT 1 FROM public.attempts a
    JOIN public.attempt_answers aa ON aa.attempt_id = a.id
    JOIN public.questions q ON q.id = aa.question_id
    WHERE a.status = 'in_progress' AND q.question_type = 'stroke_writing'
  ) THEN
    RAISE EXCEPTION 'finish or abandon active writing attempts before downgrading migration 000013';
  END IF;
END $$;

CREATE OR REPLACE FUNCTION public.prevent_kana_stroke_template_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION USING ERRCODE = '55000',
    MESSAGE = 'stroke templates are immutable; insert a new source version';
END;
$$;
ALTER TABLE public.attempt_answers DROP COLUMN writing_guide_snapshot;
COMMIT;
