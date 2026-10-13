BEGIN;

CREATE FUNCTION "public"."validate_exam_question"(p_question_id uuid)
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

CREATE FUNCTION "public"."validate_exam_question_set"(p_question_set_id uuid)
RETURNS void LANGUAGE plpgsql AS $$
DECLARE
  question_set_record record;
  available_question_count integer;
  current_question_id uuid;
BEGIN
  SELECT qs.id, qs.total_questions, qs.is_published
  INTO question_set_record
  FROM public.question_sets qs
  WHERE qs.id = p_question_set_id;

  IF NOT FOUND OR NOT question_set_record.is_published THEN RETURN; END IF;

  SELECT COUNT(*)::integer INTO available_question_count
  FROM public.questions q
  WHERE q.question_set_id = question_set_record.id;

  IF available_question_count = 0 THEN
    RAISE EXCEPTION USING ERRCODE = '23514',
      MESSAGE = FORMAT('published question set %s must contain at least one question', question_set_record.id);
  END IF;

  IF question_set_record.total_questions IS NOT NULL
    AND question_set_record.total_questions > available_question_count THEN
    RAISE EXCEPTION USING ERRCODE = '23514',
      MESSAGE = FORMAT('question set %s requests %s questions but only %s are available',
        question_set_record.id, question_set_record.total_questions, available_question_count);
  END IF;

  FOR current_question_id IN
    SELECT q.id FROM public.questions q WHERE q.question_set_id = question_set_record.id
  LOOP
    PERFORM public.validate_exam_question(current_question_id);
  END LOOP;
END;
$$;

CREATE FUNCTION "public"."trigger_validate_exam_question"()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    PERFORM public.validate_exam_question_set(OLD.question_set_id);
    RETURN OLD;
  END IF;
  IF TG_OP = 'UPDATE' AND OLD.question_set_id <> NEW.question_set_id THEN
    PERFORM public.validate_exam_question_set(OLD.question_set_id);
  END IF;
  PERFORM public.validate_exam_question(NEW.id);
  PERFORM public.validate_exam_question_set(NEW.question_set_id);
  RETURN NEW;
END;
$$;

CREATE FUNCTION "public"."trigger_validate_exam_option"()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    PERFORM public.validate_exam_question(OLD.question_id);
    RETURN OLD;
  END IF;
  IF TG_OP = 'UPDATE' AND OLD.question_id <> NEW.question_id THEN
    PERFORM public.validate_exam_question(OLD.question_id);
  END IF;
  PERFORM public.validate_exam_question(NEW.question_id);
  RETURN NEW;
END;
$$;

CREATE FUNCTION "public"."trigger_validate_exam_question_set"()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  PERFORM public.validate_exam_question_set(NEW.id);
  RETURN NEW;
END;
$$;

CREATE CONSTRAINT TRIGGER "questions_validate_exam_configuration"
AFTER INSERT OR UPDATE OR DELETE ON public.questions
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION public.trigger_validate_exam_question();

CREATE CONSTRAINT TRIGGER "question_options_validate_exam_configuration"
AFTER INSERT OR UPDATE OR DELETE ON public.question_options
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION public.trigger_validate_exam_option();

CREATE CONSTRAINT TRIGGER "question_sets_validate_exam_configuration"
AFTER INSERT OR UPDATE ON public.question_sets
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION public.trigger_validate_exam_question_set();

COMMIT;
