BEGIN;

DROP TRIGGER IF EXISTS "question_options_prevent_attempted_mutation"
ON "public"."question_options";
DROP TRIGGER IF EXISTS "questions_prevent_attempted_mutation"
ON "public"."questions";
DROP TRIGGER IF EXISTS "question_sets_validate_exam_configuration"
ON "public"."question_sets";
DROP TRIGGER IF EXISTS "question_options_validate_exam_configuration"
ON "public"."question_options";
DROP TRIGGER IF EXISTS "questions_validate_exam_configuration"
ON "public"."questions";

DROP FUNCTION IF EXISTS "public"."prevent_attempted_option_mutation"();
DROP FUNCTION IF EXISTS "public"."prevent_attempted_question_mutation"();
DROP FUNCTION IF EXISTS "public"."trigger_validate_exam_question_set"();
DROP FUNCTION IF EXISTS "public"."trigger_validate_exam_option"();
DROP FUNCTION IF EXISTS "public"."trigger_validate_exam_question"();
DROP FUNCTION IF EXISTS "public"."validate_exam_question_set"(uuid);
DROP FUNCTION IF EXISTS "public"."validate_exam_question"(uuid);

ALTER TABLE "public"."attempt_answers"
DROP CONSTRAINT IF EXISTS "attempt_answers_question_selected_option_fkey",
ADD CONSTRAINT "attempt_answers_selected_option_id_fkey"
FOREIGN KEY ("selected_option_id")
REFERENCES "public"."question_options" ("id")
ON DELETE SET NULL
ON UPDATE NO ACTION;

ALTER TABLE "public"."question_options"
DROP CONSTRAINT IF EXISTS "question_options_question_id_id_key",
DROP CONSTRAINT IF EXISTS "question_options_question_order_key",
DROP CONSTRAINT IF EXISTS "question_options_label_not_blank_check",
DROP CONSTRAINT IF EXISTS "question_options_order_index_nonnegative_check";

ALTER TABLE "public"."questions"
DROP CONSTRAINT IF EXISTS "questions_question_set_order_key",
DROP CONSTRAINT IF EXISTS "questions_stimulus_present_check",
DROP CONSTRAINT IF EXISTS "questions_prompt_not_blank_check",
DROP CONSTRAINT IF EXISTS "questions_order_index_nonnegative_check",
DROP CONSTRAINT IF EXISTS "questions_points_positive_check";

CREATE INDEX "idx_options_question" ON "public"."question_options"
USING btree ("question_id");

CREATE INDEX "idx_questions_set" ON "public"."questions"
USING btree ("question_set_id", "order_index");

DROP INDEX IF EXISTS "public"."uq_question_sets_module_order";
DROP INDEX IF EXISTS "public"."uq_question_sets_lesson_order";

ALTER TABLE "public"."question_sets"
DROP CONSTRAINT IF EXISTS "question_sets_order_index_nonnegative_check",
DROP CONSTRAINT IF EXISTS "question_sets_cooldown_nonnegative_check",
DROP CONSTRAINT IF EXISTS "question_sets_max_attempts_positive_check",
DROP CONSTRAINT IF EXISTS "question_sets_time_limit_positive_check",
DROP CONSTRAINT IF EXISTS "question_sets_total_questions_positive_check",
DROP COLUMN "order_index";

ALTER TABLE "public"."attempt_answers"
ADD COLUMN "answer_text" varchar(255) COLLATE "pg_catalog"."default";

ALTER TABLE "public"."questions"
ADD COLUMN "correct_answer" varchar(64) COLLATE "pg_catalog"."default",
ALTER COLUMN "prompt_text" DROP NOT NULL,
DROP COLUMN "stimulus_text";

ALTER TABLE "public"."questions"
RENAME COLUMN "stimulus_media_url" TO "prompt_media_url";

CREATE TYPE "public"."question_type_legacy" AS ENUM (
  'kana_to_romaji',
  'romaji_to_kana',
  'audio_to_kana',
  'stroke_writing',
  'multiple_choice'
);

ALTER TABLE "public"."questions"
ALTER COLUMN "question_type" TYPE "public"."question_type_legacy"
USING "question_type"::text::"public"."question_type_legacy";

DROP TYPE "public"."question_type";
ALTER TYPE "public"."question_type_legacy" RENAME TO "question_type";

CREATE TYPE "public"."kana_script" AS ENUM (
  'hiragana',
  'katakana'
);

CREATE TYPE "public"."kana_type" AS ENUM (
  'gojuon',
  'dakuten',
  'handakuten',
  'yoon'
);

CREATE TABLE "public"."kana_characters" (
  "id" uuid NOT NULL DEFAULT gen_random_uuid(),
  "script" "public"."kana_script" NOT NULL,
  "character" varchar(4) COLLATE "pg_catalog"."default" NOT NULL,
  "romaji" varchar(8) COLLATE "pg_catalog"."default" NOT NULL,
  "row_group" varchar(8) COLLATE "pg_catalog"."default" NOT NULL,
  "char_type" "public"."kana_type" NOT NULL DEFAULT 'gojuon'::"public"."kana_type",
  "stroke_count" int2 NOT NULL,
  "stroke_data" jsonb,
  "audio_url" text COLLATE "pg_catalog"."default",
  "mnemonic" text COLLATE "pg_catalog"."default",
  "order_index" int2 NOT NULL,
  CONSTRAINT "kana_characters_pkey" PRIMARY KEY ("id"),
  CONSTRAINT "kana_characters_script_character_key" UNIQUE ("script", "character")
);

CREATE INDEX "idx_kana_script_order" ON "public"."kana_characters"
USING btree ("script", "order_index");

CREATE TABLE "public"."lesson_characters" (
  "lesson_id" uuid NOT NULL,
  "character_id" uuid NOT NULL,
  "order_index" int2 NOT NULL DEFAULT 0,
  CONSTRAINT "lesson_characters_pkey" PRIMARY KEY ("lesson_id", "character_id"),
  CONSTRAINT "lesson_characters_character_id_fkey"
    FOREIGN KEY ("character_id") REFERENCES "public"."kana_characters" ("id")
    ON DELETE CASCADE ON UPDATE NO ACTION,
  CONSTRAINT "lesson_characters_lesson_id_fkey"
    FOREIGN KEY ("lesson_id") REFERENCES "public"."lessons" ("id")
    ON DELETE CASCADE ON UPDATE NO ACTION
);

CREATE TABLE "public"."user_character_mastery" (
  "user_id" uuid NOT NULL,
  "character_id" uuid NOT NULL,
  "correct_count" int4 NOT NULL DEFAULT 0,
  "wrong_count" int4 NOT NULL DEFAULT 0,
  "mastery_level" int2 NOT NULL DEFAULT 0,
  "last_seen_at" timestamptz(6),
  "next_review_at" timestamptz(6),
  CONSTRAINT "user_character_mastery_pkey" PRIMARY KEY ("user_id", "character_id"),
  CONSTRAINT "user_character_mastery_character_id_fkey"
    FOREIGN KEY ("character_id") REFERENCES "public"."kana_characters" ("id")
    ON DELETE CASCADE ON UPDATE NO ACTION,
  CONSTRAINT "user_character_mastery_user_id_fkey"
    FOREIGN KEY ("user_id") REFERENCES "public"."users" ("id")
    ON DELETE CASCADE ON UPDATE NO ACTION
);

ALTER TABLE "public"."questions"
ADD COLUMN "character_id" uuid,
ADD CONSTRAINT "questions_character_id_fkey"
FOREIGN KEY ("character_id") REFERENCES "public"."kana_characters" ("id")
ON DELETE SET NULL
ON UPDATE NO ACTION;

COMMIT;
