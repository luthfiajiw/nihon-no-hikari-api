BEGIN;

-- Only the two question types supported by the redesigned exam UI may remain.
DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM "public"."questions"
    WHERE "question_type"::text NOT IN ('multiple_choice', 'stroke_writing')
  ) THEN
    RAISE EXCEPTION
      'questions with legacy question types must be converted or removed before this migration';
  END IF;
END;
$$;

-- Character data is now stored directly as the question stimulus. The old
-- character catalog, lesson mapping, and character-specific mastery table are
-- intentionally removed together so no orphaned relationship remains.
DROP TABLE "public"."user_character_mastery";
DROP TABLE "public"."lesson_characters";

ALTER TABLE "public"."questions"
DROP CONSTRAINT "questions_character_id_fkey",
DROP COLUMN "character_id";

DROP TABLE "public"."kana_characters";
DROP TYPE "public"."kana_script";
DROP TYPE "public"."kana_type";

CREATE TYPE "public"."question_type_v2" AS ENUM (
  'multiple_choice',
  'stroke_writing'
);

ALTER TABLE "public"."questions"
ALTER COLUMN "question_type" TYPE "public"."question_type_v2"
USING "question_type"::text::"public"."question_type_v2";

DROP TYPE "public"."question_type";
ALTER TYPE "public"."question_type_v2" RENAME TO "question_type";

-- prompt_text is the instruction, for example "Bagaimana cara membaca
-- karakter ini?". stimulus_text is the highlighted content, for example
-- "?" or "saya mau pergi ke sekolah".
ALTER TABLE "public"."questions"
RENAME COLUMN "prompt_media_url" TO "stimulus_media_url";

ALTER TABLE "public"."questions"
ADD COLUMN "stimulus_text" text COLLATE "pg_catalog"."default",
ALTER COLUMN "prompt_text" SET NOT NULL,
DROP COLUMN "correct_answer";

-- Free-text answers are no longer part of the supported question contract.
-- stroke_input remains available for the future server-side stroke evaluator.
ALTER TABLE "public"."attempt_answers"
DROP COLUMN "answer_text";

ALTER TABLE "public"."question_sets"
ADD COLUMN "order_index" int2;

WITH ranked_question_sets AS (
  SELECT
    "id",
    (ROW_NUMBER() OVER (
      PARTITION BY "lesson_id", "module_id"
      ORDER BY "created_at", "id"
    ) - 1)::int2 AS new_order_index
  FROM "public"."question_sets"
)
UPDATE "public"."question_sets" AS qs
SET "order_index" = ranked.new_order_index
FROM ranked_question_sets AS ranked
WHERE ranked."id" = qs."id";

ALTER TABLE "public"."question_sets"
ALTER COLUMN "order_index" SET NOT NULL;

-- Normalize existing order values before installing uniqueness constraints.
DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM "public"."questions"
    GROUP BY "question_set_id"
    HAVING COUNT(*) > 32768
  ) THEN
    RAISE EXCEPTION 'a question set cannot contain more than 32768 questions';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM "public"."question_options"
    GROUP BY "question_id"
    HAVING COUNT(*) > 32768
  ) THEN
    RAISE EXCEPTION 'a question cannot contain more than 32768 options';
  END IF;
END;
$$;

WITH ranked_questions AS (
  SELECT
    "id",
    (ROW_NUMBER() OVER (
      PARTITION BY "question_set_id"
      ORDER BY "order_index", "id"
    ) - 1)::int2 AS new_order_index
  FROM "public"."questions"
)
UPDATE "public"."questions" AS q
SET "order_index" = ranked.new_order_index
FROM ranked_questions AS ranked
WHERE ranked."id" = q."id"
  AND q."order_index" IS DISTINCT FROM ranked.new_order_index;

WITH ranked_options AS (
  SELECT
    "id",
    (ROW_NUMBER() OVER (
      PARTITION BY "question_id"
      ORDER BY "order_index", "id"
    ) - 1)::int2 AS new_order_index
  FROM "public"."question_options"
)
UPDATE "public"."question_options" AS qo
SET "order_index" = ranked.new_order_index
FROM ranked_options AS ranked
WHERE ranked."id" = qo."id"
  AND qo."order_index" IS DISTINCT FROM ranked.new_order_index;

ALTER TABLE "public"."questions"
ADD CONSTRAINT "questions_points_positive_check"
CHECK ("points" > 0),
ADD CONSTRAINT "questions_order_index_nonnegative_check"
CHECK ("order_index" >= 0),
ADD CONSTRAINT "questions_prompt_not_blank_check"
CHECK (NULLIF(BTRIM("prompt_text"), '') IS NOT NULL),
ADD CONSTRAINT "questions_stimulus_present_check"
CHECK (
  NULLIF(BTRIM("stimulus_text"), '') IS NOT NULL
  OR NULLIF(BTRIM("stimulus_media_url"), '') IS NOT NULL
);

ALTER TABLE "public"."question_options"
ADD CONSTRAINT "question_options_order_index_nonnegative_check"
CHECK ("order_index" >= 0),
ADD CONSTRAINT "question_options_label_not_blank_check"
CHECK (NULLIF(BTRIM("label"), '') IS NOT NULL);

ALTER TABLE "public"."question_sets"
ADD CONSTRAINT "question_sets_total_questions_positive_check"
CHECK ("total_questions" IS NULL OR "total_questions" > 0),
ADD CONSTRAINT "question_sets_time_limit_positive_check"
CHECK ("time_limit_seconds" IS NULL OR "time_limit_seconds" > 0),
ADD CONSTRAINT "question_sets_max_attempts_positive_check"
CHECK ("max_attempts" IS NULL OR "max_attempts" > 0),
ADD CONSTRAINT "question_sets_cooldown_nonnegative_check"
CHECK ("cooldown_minutes" >= 0),
ADD CONSTRAINT "question_sets_order_index_nonnegative_check"
CHECK ("order_index" >= 0);

CREATE UNIQUE INDEX "uq_question_sets_lesson_order"
ON "public"."question_sets" ("lesson_id", "order_index")
WHERE "lesson_id" IS NOT NULL;

CREATE UNIQUE INDEX "uq_question_sets_module_order"
ON "public"."question_sets" ("module_id", "order_index")
WHERE "module_id" IS NOT NULL;

DROP INDEX "public"."idx_questions_set";
DROP INDEX "public"."idx_options_question";

ALTER TABLE "public"."questions"
ADD CONSTRAINT "questions_question_set_order_key"
UNIQUE ("question_set_id", "order_index")
DEFERRABLE INITIALLY IMMEDIATE;

ALTER TABLE "public"."question_options"
ADD CONSTRAINT "question_options_question_order_key"
UNIQUE ("question_id", "order_index")
DEFERRABLE INITIALLY IMMEDIATE,
ADD CONSTRAINT "question_options_question_id_id_key"
UNIQUE ("question_id", "id");

-- The selected option must belong to the same question as the answer.
ALTER TABLE "public"."attempt_answers"
DROP CONSTRAINT "attempt_answers_selected_option_id_fkey",
ADD CONSTRAINT "attempt_answers_question_selected_option_fkey"
FOREIGN KEY ("question_id", "selected_option_id")
REFERENCES "public"."question_options" ("question_id", "id")
ON DELETE RESTRICT
ON UPDATE NO ACTION;

CREATE FUNCTION "public"."validate_exam_question"(p_question_id uuid)
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
  question_record record;
  option_count integer;
  correct_option_count integer;
BEGIN
  SELECT
    q."id",
    q."question_type",
    q."skill",
    q."stimulus_text",
    qs."skill" AS question_set_skill,
    qs."is_published"
  INTO question_record
  FROM "public"."questions" AS q
  JOIN "public"."question_sets" AS qs ON qs."id" = q."question_set_id"
  WHERE q."id" = p_question_id;

  IF NOT FOUND OR NOT question_record.is_published THEN
    RETURN;
  END IF;

  IF question_record.skill NOT IN (
    'reading'::"public"."skill_type",
    'writing'::"public"."skill_type"
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '23514',
      MESSAGE = FORMAT(
        'published question %s uses unsupported skill %s',
        question_record.id,
        question_record.skill
      );
  END IF;

  IF question_record.question_set_skill IS NOT NULL
    AND question_record.skill <> question_record.question_set_skill THEN
    RAISE EXCEPTION USING
      ERRCODE = '23514',
      MESSAGE = FORMAT(
        'question %s skill does not match its question set skill',
        question_record.id
      );
  END IF;

  SELECT
    COUNT(*)::integer,
    COUNT(*) FILTER (WHERE qo."is_correct")::integer
  INTO option_count, correct_option_count
  FROM "public"."question_options" AS qo
  WHERE qo."question_id" = question_record.id;

  CASE question_record.question_type
    WHEN 'multiple_choice'::"public"."question_type" THEN
      IF option_count < 2 OR correct_option_count <> 1 THEN
        RAISE EXCEPTION USING
          ERRCODE = '23514',
          MESSAGE = FORMAT(
            'multiple-choice question %s must have at least two options and exactly one correct option',
            question_record.id
          );
      END IF;

    WHEN 'stroke_writing'::"public"."question_type" THEN
      IF option_count <> 0 THEN
        RAISE EXCEPTION USING
          ERRCODE = '23514',
          MESSAGE = FORMAT(
            'stroke-writing question %s must not have selectable options',
            question_record.id
          );
      END IF;

      IF NULLIF(BTRIM(question_record.stimulus_text), '') IS NULL THEN
        RAISE EXCEPTION USING
          ERRCODE = '23514',
          MESSAGE = FORMAT(
            'stroke-writing question %s must define a text stimulus',
            question_record.id
          );
      END IF;

      RAISE EXCEPTION USING
        ERRCODE = '23514',
        MESSAGE = FORMAT(
          'stroke-writing question %s cannot be published before a server-side evaluator is available',
          question_record.id
        );
  END CASE;
END;
$$;

CREATE FUNCTION "public"."validate_exam_question_set"(p_question_set_id uuid)
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
  question_set_record record;
  available_question_count integer;
  current_question_id uuid;
BEGIN
  SELECT qs."id", qs."total_questions", qs."is_published"
  INTO question_set_record
  FROM "public"."question_sets" AS qs
  WHERE qs."id" = p_question_set_id;

  IF NOT FOUND OR NOT question_set_record.is_published THEN
    RETURN;
  END IF;

  SELECT COUNT(*)::integer
  INTO available_question_count
  FROM "public"."questions" AS q
  WHERE q."question_set_id" = question_set_record.id;

  IF available_question_count = 0 THEN
    RAISE EXCEPTION USING
      ERRCODE = '23514',
      MESSAGE = FORMAT(
        'published question set %s must contain at least one question',
        question_set_record.id
      );
  END IF;

  IF question_set_record.total_questions IS NOT NULL
    AND question_set_record.total_questions > available_question_count THEN
    RAISE EXCEPTION USING
      ERRCODE = '23514',
      MESSAGE = FORMAT(
        'question set %s requests %s questions but only %s are available',
        question_set_record.id,
        question_set_record.total_questions,
        available_question_count
      );
  END IF;

  FOR current_question_id IN
    SELECT q."id"
    FROM "public"."questions" AS q
    WHERE q."question_set_id" = question_set_record.id
  LOOP
    PERFORM "public"."validate_exam_question"(current_question_id);
  END LOOP;
END;
$$;

CREATE FUNCTION "public"."trigger_validate_exam_question"()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    PERFORM "public"."validate_exam_question_set"(OLD."question_set_id");
    RETURN OLD;
  END IF;

  IF TG_OP = 'UPDATE' AND OLD."question_set_id" <> NEW."question_set_id" THEN
    PERFORM "public"."validate_exam_question_set"(OLD."question_set_id");
  END IF;

  PERFORM "public"."validate_exam_question"(NEW."id");
  PERFORM "public"."validate_exam_question_set"(NEW."question_set_id");
  RETURN NEW;
END;
$$;

CREATE FUNCTION "public"."trigger_validate_exam_option"()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    PERFORM "public"."validate_exam_question"(OLD."question_id");
    RETURN OLD;
  END IF;

  IF TG_OP = 'UPDATE' AND OLD."question_id" <> NEW."question_id" THEN
    PERFORM "public"."validate_exam_question"(OLD."question_id");
  END IF;

  PERFORM "public"."validate_exam_question"(NEW."question_id");
  RETURN NEW;
END;
$$;

CREATE FUNCTION "public"."trigger_validate_exam_question_set"()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  PERFORM "public"."validate_exam_question_set"(NEW."id");
  RETURN NEW;
END;
$$;

CREATE CONSTRAINT TRIGGER "questions_validate_exam_configuration"
AFTER INSERT OR UPDATE OR DELETE ON "public"."questions"
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION "public"."trigger_validate_exam_question"();

CREATE CONSTRAINT TRIGGER "question_options_validate_exam_configuration"
AFTER INSERT OR UPDATE OR DELETE ON "public"."question_options"
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION "public"."trigger_validate_exam_option"();

CREATE CONSTRAINT TRIGGER "question_sets_validate_exam_configuration"
AFTER INSERT OR UPDATE ON "public"."question_sets"
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION "public"."trigger_validate_exam_question_set"();

CREATE FUNCTION "public"."prevent_attempted_question_mutation"()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM "public"."attempt_answers" AS aa
    WHERE aa."question_id" = OLD."id"
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = FORMAT(
        'question %s is immutable because it has already been assigned to an attempt',
        OLD."id"
      );
  END IF;

  IF TG_OP = 'DELETE' THEN
    RETURN OLD;
  END IF;
  RETURN NEW;
END;
$$;

CREATE FUNCTION "public"."prevent_attempted_option_mutation"()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  old_question_id uuid;
  new_question_id uuid;
BEGIN
  IF TG_OP <> 'INSERT' THEN
    old_question_id := OLD."question_id";
  END IF;
  IF TG_OP <> 'DELETE' THEN
    new_question_id := NEW."question_id";
  END IF;

  IF EXISTS (
    SELECT 1
    FROM "public"."attempt_answers" AS aa
    WHERE aa."question_id" = old_question_id
       OR aa."question_id" = new_question_id
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = FORMAT(
        'options for question %s are immutable because the question has already been assigned to an attempt',
        COALESCE(old_question_id, new_question_id)
      );
  END IF;

  IF TG_OP = 'DELETE' THEN
    RETURN OLD;
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER "questions_prevent_attempted_mutation"
BEFORE UPDATE OR DELETE ON "public"."questions"
FOR EACH ROW
EXECUTE FUNCTION "public"."prevent_attempted_question_mutation"();

CREATE TRIGGER "question_options_prevent_attempted_mutation"
BEFORE INSERT OR UPDATE OR DELETE ON "public"."question_options"
FOR EACH ROW
EXECUTE FUNCTION "public"."prevent_attempted_option_mutation"();

DO $$
DECLARE
  current_question_set_id uuid;
BEGIN
  FOR current_question_set_id IN
    SELECT qs."id"
    FROM "public"."question_sets" AS qs
    WHERE qs."is_published"
  LOOP
    PERFORM "public"."validate_exam_question_set"(current_question_set_id);
  END LOOP;
END;
$$;

COMMIT;
