-- ----------------------------
-- Type structure for attempt_status
-- ----------------------------
DROP TYPE IF EXISTS "public"."attempt_status";
CREATE TYPE "public"."attempt_status" AS ENUM (
  'in_progress',
  'submitted',
  'abandoned'
);

-- ----------------------------
-- Type structure for kana_script
-- ----------------------------
DROP TYPE IF EXISTS "public"."kana_script";
CREATE TYPE "public"."kana_script" AS ENUM (
  'hiragana',
  'katakana'
);

-- ----------------------------
-- Type structure for kana_type
-- ----------------------------
DROP TYPE IF EXISTS "public"."kana_type";
CREATE TYPE "public"."kana_type" AS ENUM (
  'gojuon',
  'dakuten',
  'handakuten',
  'yoon'
);

-- ----------------------------
-- Type structure for progress_status
-- ----------------------------
DROP TYPE IF EXISTS "public"."progress_status";
CREATE TYPE "public"."progress_status" AS ENUM (
  'locked',
  'unlocked',
  'in_progress',
  'completed'
);

-- ----------------------------
-- Type structure for question_set_kind
-- ----------------------------
DROP TYPE IF EXISTS "public"."question_set_kind";
CREATE TYPE "public"."question_set_kind" AS ENUM (
  'practice',
  'final_exam'
);

-- ----------------------------
-- Type structure for question_type
-- ----------------------------
DROP TYPE IF EXISTS "public"."question_type";
CREATE TYPE "public"."question_type" AS ENUM (
  'kana_to_romaji',
  'romaji_to_kana',
  'audio_to_kana',
  'stroke_writing',
  'multiple_choice'
);

-- ----------------------------
-- Type structure for skill_type
-- ----------------------------
DROP TYPE IF EXISTS "public"."skill_type";
CREATE TYPE "public"."skill_type" AS ENUM (
  'reading',
  'writing',
  'listening'
);

-- ----------------------------
-- Table structure for attempt_answers
-- ----------------------------
DROP TABLE IF EXISTS "public"."attempt_answers";
CREATE TABLE "public"."attempt_answers" (
  "id" uuid NOT NULL DEFAULT gen_random_uuid(),
  "attempt_id" uuid NOT NULL,
  "question_id" uuid NOT NULL,
  "selected_option_id" uuid,
  "answer_text" varchar(255) COLLATE "pg_catalog"."default",
  "stroke_input" jsonb,
  "accuracy" numeric(5,2),
  "is_correct" bool NOT NULL DEFAULT false,
  "earned_points" int2 NOT NULL DEFAULT 0,
  "answered_at" timestamptz(6) NOT NULL DEFAULT now()
)
;

-- ----------------------------
-- Table structure for attempts
-- ----------------------------
DROP TABLE IF EXISTS "public"."attempts";
CREATE TABLE "public"."attempts" (
  "id" uuid NOT NULL DEFAULT gen_random_uuid(),
  "user_id" uuid NOT NULL,
  "question_set_id" uuid NOT NULL,
  "attempt_number" int2 NOT NULL,
  "status" "public"."attempt_status" NOT NULL DEFAULT 'in_progress'::attempt_status,
  "score" int2,
  "total_points" int2,
  "earned_points" int2,
  "is_passed" bool,
  "started_at" timestamptz(6) NOT NULL DEFAULT now(),
  "submitted_at" timestamptz(6)
)
;

-- ----------------------------
-- Table structure for courses
-- ----------------------------
DROP TABLE IF EXISTS "public"."courses";
CREATE TABLE "public"."courses" (
  "id" uuid NOT NULL DEFAULT gen_random_uuid(),
  "level_id" uuid NOT NULL,
  "slug" varchar(120) COLLATE "pg_catalog"."default" NOT NULL,
  "title" varchar(160) COLLATE "pg_catalog"."default" NOT NULL,
  "description" text COLLATE "pg_catalog"."default",
  "thumbnail_url" text COLLATE "pg_catalog"."default",
  "is_published" bool NOT NULL DEFAULT false,
  "created_at" timestamptz(6) NOT NULL DEFAULT now(),
  "updated_at" timestamptz(6) NOT NULL DEFAULT now()
)
;

-- ----------------------------
-- Table structure for enrollments
-- ----------------------------
DROP TABLE IF EXISTS "public"."enrollments";
CREATE TABLE "public"."enrollments" (
  "id" uuid NOT NULL DEFAULT gen_random_uuid(),
  "user_id" uuid NOT NULL,
  "course_id" uuid NOT NULL,
  "enrolled_at" timestamptz(6) NOT NULL DEFAULT now(),
  "completed_at" timestamptz(6)
)
;

-- ----------------------------
-- Table structure for kana_characters
-- ----------------------------
DROP TABLE IF EXISTS "public"."kana_characters";
CREATE TABLE "public"."kana_characters" (
  "id" uuid NOT NULL DEFAULT gen_random_uuid(),
  "script" "public"."kana_script" NOT NULL,
  "character" varchar(4) COLLATE "pg_catalog"."default" NOT NULL,
  "romaji" varchar(8) COLLATE "pg_catalog"."default" NOT NULL,
  "row_group" varchar(8) COLLATE "pg_catalog"."default" NOT NULL,
  "char_type" "public"."kana_type" NOT NULL DEFAULT 'gojuon'::kana_type,
  "stroke_count" int2 NOT NULL,
  "stroke_data" jsonb,
  "audio_url" text COLLATE "pg_catalog"."default",
  "mnemonic" text COLLATE "pg_catalog"."default",
  "order_index" int2 NOT NULL
)
;

-- ----------------------------
-- Table structure for lesson_characters
-- ----------------------------
DROP TABLE IF EXISTS "public"."lesson_characters";
CREATE TABLE "public"."lesson_characters" (
  "lesson_id" uuid NOT NULL,
  "character_id" uuid NOT NULL,
  "order_index" int2 NOT NULL DEFAULT 0
)
;

-- ----------------------------
-- Table structure for lessons
-- ----------------------------
DROP TABLE IF EXISTS "public"."lessons";
CREATE TABLE "public"."lessons" (
  "id" uuid NOT NULL DEFAULT gen_random_uuid(),
  "module_id" uuid NOT NULL,
  "slug" varchar(120) COLLATE "pg_catalog"."default" NOT NULL,
  "title" varchar(160) COLLATE "pg_catalog"."default" NOT NULL,
  "content" jsonb,
  "order_index" int2 NOT NULL,
  "is_published" bool NOT NULL DEFAULT false,
  "created_at" timestamptz(6) NOT NULL DEFAULT now(),
  "updated_at" timestamptz(6) NOT NULL DEFAULT now()
)
;

-- ----------------------------
-- Table structure for levels
-- ----------------------------
DROP TABLE IF EXISTS "public"."levels";
CREATE TABLE "public"."levels" (
  "id" uuid NOT NULL DEFAULT gen_random_uuid(),
  "code" varchar(8) COLLATE "pg_catalog"."default" NOT NULL,
  "name" varchar(100) COLLATE "pg_catalog"."default" NOT NULL,
  "description" text COLLATE "pg_catalog"."default",
  "order_index" int2 NOT NULL,
  "created_at" timestamptz(6) NOT NULL DEFAULT now(),
  "updated_at" timestamptz(6) NOT NULL DEFAULT now()
)
;

-- ----------------------------
-- Table structure for module_prerequisites
-- ----------------------------
DROP TABLE IF EXISTS "public"."module_prerequisites";
CREATE TABLE "public"."module_prerequisites" (
  "module_id" uuid NOT NULL,
  "required_module_id" uuid NOT NULL
)
;

-- ----------------------------
-- Table structure for modules
-- ----------------------------
DROP TABLE IF EXISTS "public"."modules";
CREATE TABLE "public"."modules" (
  "id" uuid NOT NULL DEFAULT gen_random_uuid(),
  "course_id" uuid NOT NULL,
  "slug" varchar(120) COLLATE "pg_catalog"."default" NOT NULL,
  "title" varchar(160) COLLATE "pg_catalog"."default" NOT NULL,
  "description" text COLLATE "pg_catalog"."default",
  "order_index" int2 NOT NULL,
  "is_mandatory" bool NOT NULL DEFAULT true,
  "is_entry" bool NOT NULL DEFAULT false,
  "estimated_minutes" int4,
  "is_published" bool NOT NULL DEFAULT false,
  "created_at" timestamptz(6) NOT NULL DEFAULT now(),
  "updated_at" timestamptz(6) NOT NULL DEFAULT now()
)
;

-- ----------------------------
-- Table structure for question_options
-- ----------------------------
DROP TABLE IF EXISTS "public"."question_options";
CREATE TABLE "public"."question_options" (
  "id" uuid NOT NULL DEFAULT gen_random_uuid(),
  "question_id" uuid NOT NULL,
  "label" varchar(120) COLLATE "pg_catalog"."default" NOT NULL,
  "media_url" text COLLATE "pg_catalog"."default",
  "is_correct" bool NOT NULL DEFAULT false,
  "order_index" int2 NOT NULL DEFAULT 0
)
;

-- ----------------------------
-- Table structure for question_sets
-- ----------------------------
DROP TABLE IF EXISTS "public"."question_sets";
CREATE TABLE "public"."question_sets" (
  "id" uuid NOT NULL DEFAULT gen_random_uuid(),
  "kind" "public"."question_set_kind" NOT NULL,
  "module_id" uuid,
  "lesson_id" uuid,
  "title" varchar(160) COLLATE "pg_catalog"."default" NOT NULL,
  "skill" "public"."skill_type",
  "passing_score" int2 NOT NULL DEFAULT 80,
  "total_questions" int2,
  "time_limit_seconds" int4,
  "max_attempts" int2,
  "cooldown_minutes" int4 NOT NULL DEFAULT 0,
  "shuffle_questions" bool NOT NULL DEFAULT true,
  "is_published" bool NOT NULL DEFAULT false,
  "created_at" timestamptz(6) NOT NULL DEFAULT now()
)
;

-- ----------------------------
-- Table structure for questions
-- ----------------------------
DROP TABLE IF EXISTS "public"."questions";
CREATE TABLE "public"."questions" (
  "id" uuid NOT NULL DEFAULT gen_random_uuid(),
  "question_set_id" uuid NOT NULL,
  "character_id" uuid,
  "question_type" "public"."question_type" NOT NULL,
  "skill" "public"."skill_type" NOT NULL,
  "prompt_text" text COLLATE "pg_catalog"."default",
  "prompt_media_url" text COLLATE "pg_catalog"."default",
  "correct_answer" varchar(64) COLLATE "pg_catalog"."default",
  "explanation" text COLLATE "pg_catalog"."default",
  "points" int2 NOT NULL DEFAULT 1,
  "order_index" int2 NOT NULL DEFAULT 0,
  "created_at" timestamptz(6) NOT NULL DEFAULT now()
)
;

-- ----------------------------
-- Table structure for user_character_mastery
-- ----------------------------
DROP TABLE IF EXISTS "public"."user_character_mastery";
CREATE TABLE "public"."user_character_mastery" (
  "user_id" uuid NOT NULL,
  "character_id" uuid NOT NULL,
  "correct_count" int4 NOT NULL DEFAULT 0,
  "wrong_count" int4 NOT NULL DEFAULT 0,
  "mastery_level" int2 NOT NULL DEFAULT 0,
  "last_seen_at" timestamptz(6),
  "next_review_at" timestamptz(6)
)
;

-- ----------------------------
-- Table structure for user_lesson_progress
-- ----------------------------
DROP TABLE IF EXISTS "public"."user_lesson_progress";
CREATE TABLE "public"."user_lesson_progress" (
  "user_id" uuid NOT NULL,
  "lesson_id" uuid NOT NULL,
  "status" "public"."progress_status" NOT NULL DEFAULT 'unlocked'::progress_status,
  "completed_at" timestamptz(6),
  "updated_at" timestamptz(6) NOT NULL DEFAULT now()
)
;

-- ----------------------------
-- Table structure for user_module_progress
-- ----------------------------
DROP TABLE IF EXISTS "public"."user_module_progress";
CREATE TABLE "public"."user_module_progress" (
  "user_id" uuid NOT NULL,
  "module_id" uuid NOT NULL,
  "status" "public"."progress_status" NOT NULL DEFAULT 'locked'::progress_status,
  "best_score" int2,
  "unlocked_at" timestamptz(6),
  "completed_at" timestamptz(6),
  "updated_at" timestamptz(6) NOT NULL DEFAULT now()
)
;

-- ----------------------------
-- Table structure for users
-- ----------------------------
DROP TABLE IF EXISTS "public"."users";
CREATE TABLE "public"."users" (
  "id" uuid NOT NULL DEFAULT gen_random_uuid(),
  "email" varchar(255) COLLATE "pg_catalog"."default" NOT NULL,
  "password_hash" text COLLATE "pg_catalog"."default",
  "display_name" varchar(120) COLLATE "pg_catalog"."default" NOT NULL,
  "avatar_url" text COLLATE "pg_catalog"."default",
  "created_at" timestamptz(6) NOT NULL DEFAULT now(),
  "updated_at" timestamptz(6) NOT NULL DEFAULT now()
)
;

-- ----------------------------
-- Function structure for armor
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."armor"(bytea);
CREATE FUNCTION "public"."armor"(bytea)
  RETURNS "pg_catalog"."text" AS '$libdir/pgcrypto', 'pg_armor'
  LANGUAGE c IMMUTABLE STRICT
  COST 1;

-- ----------------------------
-- Function structure for armor
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."armor"(bytea, _text, _text);
CREATE FUNCTION "public"."armor"(bytea, _text, _text)
  RETURNS "pg_catalog"."text" AS '$libdir/pgcrypto', 'pg_armor'
  LANGUAGE c IMMUTABLE STRICT
  COST 1;

-- ----------------------------
-- Function structure for crypt
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."crypt"(text, text);
CREATE FUNCTION "public"."crypt"(text, text)
  RETURNS "pg_catalog"."text" AS '$libdir/pgcrypto', 'pg_crypt'
  LANGUAGE c IMMUTABLE STRICT
  COST 1;

-- ----------------------------
-- Function structure for dearmor
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."dearmor"(text);
CREATE FUNCTION "public"."dearmor"(text)
  RETURNS "pg_catalog"."bytea" AS '$libdir/pgcrypto', 'pg_dearmor'
  LANGUAGE c IMMUTABLE STRICT
  COST 1;

-- ----------------------------
-- Function structure for decrypt
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."decrypt"(bytea, bytea, text);
CREATE FUNCTION "public"."decrypt"(bytea, bytea, text)
  RETURNS "pg_catalog"."bytea" AS '$libdir/pgcrypto', 'pg_decrypt'
  LANGUAGE c IMMUTABLE STRICT
  COST 1;

-- ----------------------------
-- Function structure for decrypt_iv
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."decrypt_iv"(bytea, bytea, bytea, text);
CREATE FUNCTION "public"."decrypt_iv"(bytea, bytea, bytea, text)
  RETURNS "pg_catalog"."bytea" AS '$libdir/pgcrypto', 'pg_decrypt_iv'
  LANGUAGE c IMMUTABLE STRICT
  COST 1;

-- ----------------------------
-- Function structure for digest
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."digest"(bytea, text);
CREATE FUNCTION "public"."digest"(bytea, text)
  RETURNS "pg_catalog"."bytea" AS '$libdir/pgcrypto', 'pg_digest'
  LANGUAGE c IMMUTABLE STRICT
  COST 1;

-- ----------------------------
-- Function structure for digest
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."digest"(text, text);
CREATE FUNCTION "public"."digest"(text, text)
  RETURNS "pg_catalog"."bytea" AS '$libdir/pgcrypto', 'pg_digest'
  LANGUAGE c IMMUTABLE STRICT
  COST 1;

-- ----------------------------
-- Function structure for encrypt
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."encrypt"(bytea, bytea, text);
CREATE FUNCTION "public"."encrypt"(bytea, bytea, text)
  RETURNS "pg_catalog"."bytea" AS '$libdir/pgcrypto', 'pg_encrypt'
  LANGUAGE c IMMUTABLE STRICT
  COST 1;

-- ----------------------------
-- Function structure for encrypt_iv
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."encrypt_iv"(bytea, bytea, bytea, text);
CREATE FUNCTION "public"."encrypt_iv"(bytea, bytea, bytea, text)
  RETURNS "pg_catalog"."bytea" AS '$libdir/pgcrypto', 'pg_encrypt_iv'
  LANGUAGE c IMMUTABLE STRICT
  COST 1;

-- ----------------------------
-- Function structure for gen_random_bytes
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."gen_random_bytes"(int4);
CREATE FUNCTION "public"."gen_random_bytes"(int4)
  RETURNS "pg_catalog"."bytea" AS '$libdir/pgcrypto', 'pg_random_bytes'
  LANGUAGE c VOLATILE STRICT
  COST 1;

-- ----------------------------
-- Function structure for gen_random_uuid
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."gen_random_uuid"();
CREATE FUNCTION "public"."gen_random_uuid"()
  RETURNS "pg_catalog"."uuid" AS '$libdir/pgcrypto', 'pg_random_uuid'
  LANGUAGE c VOLATILE
  COST 1;

-- ----------------------------
-- Function structure for gen_salt
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."gen_salt"(text, int4);
CREATE FUNCTION "public"."gen_salt"(text, int4)
  RETURNS "pg_catalog"."text" AS '$libdir/pgcrypto', 'pg_gen_salt_rounds'
  LANGUAGE c VOLATILE STRICT
  COST 1;

-- ----------------------------
-- Function structure for gen_salt
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."gen_salt"(text);
CREATE FUNCTION "public"."gen_salt"(text)
  RETURNS "pg_catalog"."text" AS '$libdir/pgcrypto', 'pg_gen_salt'
  LANGUAGE c VOLATILE STRICT
  COST 1;

-- ----------------------------
-- Function structure for hmac
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."hmac"(text, text, text);
CREATE FUNCTION "public"."hmac"(text, text, text)
  RETURNS "pg_catalog"."bytea" AS '$libdir/pgcrypto', 'pg_hmac'
  LANGUAGE c IMMUTABLE STRICT
  COST 1;

-- ----------------------------
-- Function structure for hmac
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."hmac"(bytea, bytea, text);
CREATE FUNCTION "public"."hmac"(bytea, bytea, text)
  RETURNS "pg_catalog"."bytea" AS '$libdir/pgcrypto', 'pg_hmac'
  LANGUAGE c IMMUTABLE STRICT
  COST 1;

-- ----------------------------
-- Function structure for pgp_armor_headers
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."pgp_armor_headers"(text, OUT "key" text, OUT "value" text);
CREATE FUNCTION "public"."pgp_armor_headers"(IN text, OUT "key" text, OUT "value" text)
  RETURNS SETOF "pg_catalog"."record" AS '$libdir/pgcrypto', 'pgp_armor_headers'
  LANGUAGE c IMMUTABLE STRICT
  COST 1
  ROWS 1000;

-- ----------------------------
-- Function structure for pgp_key_id
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."pgp_key_id"(bytea);
CREATE FUNCTION "public"."pgp_key_id"(bytea)
  RETURNS "pg_catalog"."text" AS '$libdir/pgcrypto', 'pgp_key_id_w'
  LANGUAGE c IMMUTABLE STRICT
  COST 1;

-- ----------------------------
-- Function structure for pgp_pub_decrypt
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."pgp_pub_decrypt"(bytea, bytea, text);
CREATE FUNCTION "public"."pgp_pub_decrypt"(bytea, bytea, text)
  RETURNS "pg_catalog"."text" AS '$libdir/pgcrypto', 'pgp_pub_decrypt_text'
  LANGUAGE c IMMUTABLE STRICT
  COST 1;

-- ----------------------------
-- Function structure for pgp_pub_decrypt
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."pgp_pub_decrypt"(bytea, bytea);
CREATE FUNCTION "public"."pgp_pub_decrypt"(bytea, bytea)
  RETURNS "pg_catalog"."text" AS '$libdir/pgcrypto', 'pgp_pub_decrypt_text'
  LANGUAGE c IMMUTABLE STRICT
  COST 1;

-- ----------------------------
-- Function structure for pgp_pub_decrypt
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."pgp_pub_decrypt"(bytea, bytea, text, text);
CREATE FUNCTION "public"."pgp_pub_decrypt"(bytea, bytea, text, text)
  RETURNS "pg_catalog"."text" AS '$libdir/pgcrypto', 'pgp_pub_decrypt_text'
  LANGUAGE c IMMUTABLE STRICT
  COST 1;

-- ----------------------------
-- Function structure for pgp_pub_decrypt_bytea
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."pgp_pub_decrypt_bytea"(bytea, bytea, text);
CREATE FUNCTION "public"."pgp_pub_decrypt_bytea"(bytea, bytea, text)
  RETURNS "pg_catalog"."bytea" AS '$libdir/pgcrypto', 'pgp_pub_decrypt_bytea'
  LANGUAGE c IMMUTABLE STRICT
  COST 1;

-- ----------------------------
-- Function structure for pgp_pub_decrypt_bytea
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."pgp_pub_decrypt_bytea"(bytea, bytea, text, text);
CREATE FUNCTION "public"."pgp_pub_decrypt_bytea"(bytea, bytea, text, text)
  RETURNS "pg_catalog"."bytea" AS '$libdir/pgcrypto', 'pgp_pub_decrypt_bytea'
  LANGUAGE c IMMUTABLE STRICT
  COST 1;

-- ----------------------------
-- Function structure for pgp_pub_decrypt_bytea
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."pgp_pub_decrypt_bytea"(bytea, bytea);
CREATE FUNCTION "public"."pgp_pub_decrypt_bytea"(bytea, bytea)
  RETURNS "pg_catalog"."bytea" AS '$libdir/pgcrypto', 'pgp_pub_decrypt_bytea'
  LANGUAGE c IMMUTABLE STRICT
  COST 1;

-- ----------------------------
-- Function structure for pgp_pub_encrypt
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."pgp_pub_encrypt"(text, bytea);
CREATE FUNCTION "public"."pgp_pub_encrypt"(text, bytea)
  RETURNS "pg_catalog"."bytea" AS '$libdir/pgcrypto', 'pgp_pub_encrypt_text'
  LANGUAGE c VOLATILE STRICT
  COST 1;

-- ----------------------------
-- Function structure for pgp_pub_encrypt
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."pgp_pub_encrypt"(text, bytea, text);
CREATE FUNCTION "public"."pgp_pub_encrypt"(text, bytea, text)
  RETURNS "pg_catalog"."bytea" AS '$libdir/pgcrypto', 'pgp_pub_encrypt_text'
  LANGUAGE c VOLATILE STRICT
  COST 1;

-- ----------------------------
-- Function structure for pgp_pub_encrypt_bytea
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."pgp_pub_encrypt_bytea"(bytea, bytea, text);
CREATE FUNCTION "public"."pgp_pub_encrypt_bytea"(bytea, bytea, text)
  RETURNS "pg_catalog"."bytea" AS '$libdir/pgcrypto', 'pgp_pub_encrypt_bytea'
  LANGUAGE c VOLATILE STRICT
  COST 1;

-- ----------------------------
-- Function structure for pgp_pub_encrypt_bytea
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."pgp_pub_encrypt_bytea"(bytea, bytea);
CREATE FUNCTION "public"."pgp_pub_encrypt_bytea"(bytea, bytea)
  RETURNS "pg_catalog"."bytea" AS '$libdir/pgcrypto', 'pgp_pub_encrypt_bytea'
  LANGUAGE c VOLATILE STRICT
  COST 1;

-- ----------------------------
-- Function structure for pgp_sym_decrypt
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."pgp_sym_decrypt"(bytea, text);
CREATE FUNCTION "public"."pgp_sym_decrypt"(bytea, text)
  RETURNS "pg_catalog"."text" AS '$libdir/pgcrypto', 'pgp_sym_decrypt_text'
  LANGUAGE c IMMUTABLE STRICT
  COST 1;

-- ----------------------------
-- Function structure for pgp_sym_decrypt
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."pgp_sym_decrypt"(bytea, text, text);
CREATE FUNCTION "public"."pgp_sym_decrypt"(bytea, text, text)
  RETURNS "pg_catalog"."text" AS '$libdir/pgcrypto', 'pgp_sym_decrypt_text'
  LANGUAGE c IMMUTABLE STRICT
  COST 1;

-- ----------------------------
-- Function structure for pgp_sym_decrypt_bytea
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."pgp_sym_decrypt_bytea"(bytea, text, text);
CREATE FUNCTION "public"."pgp_sym_decrypt_bytea"(bytea, text, text)
  RETURNS "pg_catalog"."bytea" AS '$libdir/pgcrypto', 'pgp_sym_decrypt_bytea'
  LANGUAGE c IMMUTABLE STRICT
  COST 1;

-- ----------------------------
-- Function structure for pgp_sym_decrypt_bytea
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."pgp_sym_decrypt_bytea"(bytea, text);
CREATE FUNCTION "public"."pgp_sym_decrypt_bytea"(bytea, text)
  RETURNS "pg_catalog"."bytea" AS '$libdir/pgcrypto', 'pgp_sym_decrypt_bytea'
  LANGUAGE c IMMUTABLE STRICT
  COST 1;

-- ----------------------------
-- Function structure for pgp_sym_encrypt
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."pgp_sym_encrypt"(text, text, text);
CREATE FUNCTION "public"."pgp_sym_encrypt"(text, text, text)
  RETURNS "pg_catalog"."bytea" AS '$libdir/pgcrypto', 'pgp_sym_encrypt_text'
  LANGUAGE c VOLATILE STRICT
  COST 1;

-- ----------------------------
-- Function structure for pgp_sym_encrypt
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."pgp_sym_encrypt"(text, text);
CREATE FUNCTION "public"."pgp_sym_encrypt"(text, text)
  RETURNS "pg_catalog"."bytea" AS '$libdir/pgcrypto', 'pgp_sym_encrypt_text'
  LANGUAGE c VOLATILE STRICT
  COST 1;

-- ----------------------------
-- Function structure for pgp_sym_encrypt_bytea
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."pgp_sym_encrypt_bytea"(bytea, text, text);
CREATE FUNCTION "public"."pgp_sym_encrypt_bytea"(bytea, text, text)
  RETURNS "pg_catalog"."bytea" AS '$libdir/pgcrypto', 'pgp_sym_encrypt_bytea'
  LANGUAGE c VOLATILE STRICT
  COST 1;

-- ----------------------------
-- Function structure for pgp_sym_encrypt_bytea
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."pgp_sym_encrypt_bytea"(bytea, text);
CREATE FUNCTION "public"."pgp_sym_encrypt_bytea"(bytea, text)
  RETURNS "pg_catalog"."bytea" AS '$libdir/pgcrypto', 'pgp_sym_encrypt_bytea'
  LANGUAGE c VOLATILE STRICT
  COST 1;

-- ----------------------------
-- Uniques structure for table attempt_answers
-- ----------------------------
ALTER TABLE "public"."attempt_answers" ADD CONSTRAINT "attempt_answers_attempt_id_question_id_key" UNIQUE ("attempt_id", "question_id");

-- ----------------------------
-- Primary Key structure for table attempt_answers
-- ----------------------------
ALTER TABLE "public"."attempt_answers" ADD CONSTRAINT "attempt_answers_pkey" PRIMARY KEY ("id");

-- ----------------------------
-- Indexes structure for table attempts
-- ----------------------------
CREATE INDEX "idx_attempts_user_set" ON "public"."attempts" USING btree (
  "user_id" "pg_catalog"."uuid_ops" ASC NULLS LAST,
  "question_set_id" "pg_catalog"."uuid_ops" ASC NULLS LAST,
  "submitted_at" "pg_catalog"."timestamptz_ops" DESC NULLS FIRST
);

-- ----------------------------
-- Uniques structure for table attempts
-- ----------------------------
ALTER TABLE "public"."attempts" ADD CONSTRAINT "attempts_user_id_question_set_id_attempt_number_key" UNIQUE ("user_id", "question_set_id", "attempt_number");

-- ----------------------------
-- Primary Key structure for table attempts
-- ----------------------------
ALTER TABLE "public"."attempts" ADD CONSTRAINT "attempts_pkey" PRIMARY KEY ("id");

-- ----------------------------
-- Indexes structure for table courses
-- ----------------------------
CREATE INDEX "idx_courses_level" ON "public"."courses" USING btree (
  "level_id" "pg_catalog"."uuid_ops" ASC NULLS LAST
);

-- ----------------------------
-- Uniques structure for table courses
-- ----------------------------
ALTER TABLE "public"."courses" ADD CONSTRAINT "courses_slug_key" UNIQUE ("slug");

-- ----------------------------
-- Primary Key structure for table courses
-- ----------------------------
ALTER TABLE "public"."courses" ADD CONSTRAINT "courses_pkey" PRIMARY KEY ("id");

-- ----------------------------
-- Uniques structure for table enrollments
-- ----------------------------
ALTER TABLE "public"."enrollments" ADD CONSTRAINT "enrollments_user_id_course_id_key" UNIQUE ("user_id", "course_id");

-- ----------------------------
-- Primary Key structure for table enrollments
-- ----------------------------
ALTER TABLE "public"."enrollments" ADD CONSTRAINT "enrollments_pkey" PRIMARY KEY ("id");

-- ----------------------------
-- Indexes structure for table kana_characters
-- ----------------------------
CREATE INDEX "idx_kana_script_order" ON "public"."kana_characters" USING btree (
  "script" "pg_catalog"."enum_ops" ASC NULLS LAST,
  "order_index" "pg_catalog"."int2_ops" ASC NULLS LAST
);

-- ----------------------------
-- Uniques structure for table kana_characters
-- ----------------------------
ALTER TABLE "public"."kana_characters" ADD CONSTRAINT "kana_characters_script_character_key" UNIQUE ("script", "character");

-- ----------------------------
-- Primary Key structure for table kana_characters
-- ----------------------------
ALTER TABLE "public"."kana_characters" ADD CONSTRAINT "kana_characters_pkey" PRIMARY KEY ("id");

-- ----------------------------
-- Primary Key structure for table lesson_characters
-- ----------------------------
ALTER TABLE "public"."lesson_characters" ADD CONSTRAINT "lesson_characters_pkey" PRIMARY KEY ("lesson_id", "character_id");

-- ----------------------------
-- Indexes structure for table lessons
-- ----------------------------
CREATE INDEX "idx_lessons_module" ON "public"."lessons" USING btree (
  "module_id" "pg_catalog"."uuid_ops" ASC NULLS LAST,
  "order_index" "pg_catalog"."int2_ops" ASC NULLS LAST
);

-- ----------------------------
-- Uniques structure for table lessons
-- ----------------------------
ALTER TABLE "public"."lessons" ADD CONSTRAINT "lessons_module_id_slug_key" UNIQUE ("module_id", "slug");

-- ----------------------------
-- Primary Key structure for table lessons
-- ----------------------------
ALTER TABLE "public"."lessons" ADD CONSTRAINT "lessons_pkey" PRIMARY KEY ("id");

-- ----------------------------
-- Uniques structure for table levels
-- ----------------------------
ALTER TABLE "public"."levels" ADD CONSTRAINT "levels_code_key" UNIQUE ("code");
ALTER TABLE "public"."levels" ADD CONSTRAINT "levels_order_index_key" UNIQUE ("order_index");

-- ----------------------------
-- Primary Key structure for table levels
-- ----------------------------
ALTER TABLE "public"."levels" ADD CONSTRAINT "levels_pkey" PRIMARY KEY ("id");

-- ----------------------------
-- Indexes structure for table module_prerequisites
-- ----------------------------
CREATE INDEX "idx_prereq_required" ON "public"."module_prerequisites" USING btree (
  "required_module_id" "pg_catalog"."uuid_ops" ASC NULLS LAST
);

-- ----------------------------
-- Checks structure for table module_prerequisites
-- ----------------------------
ALTER TABLE "public"."module_prerequisites" ADD CONSTRAINT "module_prerequisites_check" CHECK (module_id <> required_module_id);

-- ----------------------------
-- Primary Key structure for table module_prerequisites
-- ----------------------------
ALTER TABLE "public"."module_prerequisites" ADD CONSTRAINT "module_prerequisites_pkey" PRIMARY KEY ("module_id", "required_module_id");

-- ----------------------------
-- Indexes structure for table modules
-- ----------------------------
CREATE INDEX "idx_modules_course" ON "public"."modules" USING btree (
  "course_id" "pg_catalog"."uuid_ops" ASC NULLS LAST,
  "order_index" "pg_catalog"."int2_ops" ASC NULLS LAST
);

-- ----------------------------
-- Uniques structure for table modules
-- ----------------------------
ALTER TABLE "public"."modules" ADD CONSTRAINT "modules_course_id_slug_key" UNIQUE ("course_id", "slug");
ALTER TABLE "public"."modules" ADD CONSTRAINT "modules_course_id_order_index_key" UNIQUE ("course_id", "order_index") DEFERRABLE INITIALLY DEFERRED;

-- ----------------------------
-- Primary Key structure for table modules
-- ----------------------------
ALTER TABLE "public"."modules" ADD CONSTRAINT "modules_pkey" PRIMARY KEY ("id");

-- ----------------------------
-- Indexes structure for table question_options
-- ----------------------------
CREATE INDEX "idx_options_question" ON "public"."question_options" USING btree (
  "question_id" "pg_catalog"."uuid_ops" ASC NULLS LAST
);

-- ----------------------------
-- Primary Key structure for table question_options
-- ----------------------------
ALTER TABLE "public"."question_options" ADD CONSTRAINT "question_options_pkey" PRIMARY KEY ("id");

-- ----------------------------
-- Indexes structure for table question_sets
-- ----------------------------
CREATE INDEX "idx_qsets_lesson" ON "public"."question_sets" USING btree (
  "lesson_id" "pg_catalog"."uuid_ops" ASC NULLS LAST
);
CREATE UNIQUE INDEX "uq_final_exam_per_module" ON "public"."question_sets" USING btree (
  "module_id" "pg_catalog"."uuid_ops" ASC NULLS LAST
) WHERE kind = 'final_exam'::question_set_kind;

-- ----------------------------
-- Checks structure for table question_sets
-- ----------------------------
ALTER TABLE "public"."question_sets" ADD CONSTRAINT "question_sets_passing_score_check" CHECK (passing_score >= 0 AND passing_score <= 100);
ALTER TABLE "public"."question_sets" ADD CONSTRAINT "question_sets_check" CHECK (kind = 'final_exam'::question_set_kind AND module_id IS NOT NULL AND lesson_id IS NULL OR kind = 'practice'::question_set_kind AND lesson_id IS NOT NULL AND module_id IS NULL);

-- ----------------------------
-- Primary Key structure for table question_sets
-- ----------------------------
ALTER TABLE "public"."question_sets" ADD CONSTRAINT "question_sets_pkey" PRIMARY KEY ("id");

-- ----------------------------
-- Indexes structure for table questions
-- ----------------------------
CREATE INDEX "idx_questions_set" ON "public"."questions" USING btree (
  "question_set_id" "pg_catalog"."uuid_ops" ASC NULLS LAST,
  "order_index" "pg_catalog"."int2_ops" ASC NULLS LAST
);

-- ----------------------------
-- Primary Key structure for table questions
-- ----------------------------
ALTER TABLE "public"."questions" ADD CONSTRAINT "questions_pkey" PRIMARY KEY ("id");

-- ----------------------------
-- Primary Key structure for table user_character_mastery
-- ----------------------------
ALTER TABLE "public"."user_character_mastery" ADD CONSTRAINT "user_character_mastery_pkey" PRIMARY KEY ("user_id", "character_id");

-- ----------------------------
-- Primary Key structure for table user_lesson_progress
-- ----------------------------
ALTER TABLE "public"."user_lesson_progress" ADD CONSTRAINT "user_lesson_progress_pkey" PRIMARY KEY ("user_id", "lesson_id");

-- ----------------------------
-- Indexes structure for table user_module_progress
-- ----------------------------
CREATE INDEX "idx_ump_user_status" ON "public"."user_module_progress" USING btree (
  "user_id" "pg_catalog"."uuid_ops" ASC NULLS LAST,
  "status" "pg_catalog"."enum_ops" ASC NULLS LAST
);

-- ----------------------------
-- Primary Key structure for table user_module_progress
-- ----------------------------
ALTER TABLE "public"."user_module_progress" ADD CONSTRAINT "user_module_progress_pkey" PRIMARY KEY ("user_id", "module_id");

-- ----------------------------
-- Uniques structure for table users
-- ----------------------------
ALTER TABLE "public"."users" ADD CONSTRAINT "users_email_key" UNIQUE ("email");

-- ----------------------------
-- Primary Key structure for table users
-- ----------------------------
ALTER TABLE "public"."users" ADD CONSTRAINT "users_pkey" PRIMARY KEY ("id");

-- ----------------------------
-- Foreign Keys structure for table attempt_answers
-- ----------------------------
ALTER TABLE "public"."attempt_answers" ADD CONSTRAINT "attempt_answers_attempt_id_fkey" FOREIGN KEY ("attempt_id") REFERENCES "public"."attempts" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;
ALTER TABLE "public"."attempt_answers" ADD CONSTRAINT "attempt_answers_question_id_fkey" FOREIGN KEY ("question_id") REFERENCES "public"."questions" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;
ALTER TABLE "public"."attempt_answers" ADD CONSTRAINT "attempt_answers_selected_option_id_fkey" FOREIGN KEY ("selected_option_id") REFERENCES "public"."question_options" ("id") ON DELETE SET NULL ON UPDATE NO ACTION;

-- ----------------------------
-- Foreign Keys structure for table attempts
-- ----------------------------
ALTER TABLE "public"."attempts" ADD CONSTRAINT "attempts_question_set_id_fkey" FOREIGN KEY ("question_set_id") REFERENCES "public"."question_sets" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;
ALTER TABLE "public"."attempts" ADD CONSTRAINT "attempts_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "public"."users" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;

-- ----------------------------
-- Foreign Keys structure for table courses
-- ----------------------------
ALTER TABLE "public"."courses" ADD CONSTRAINT "courses_level_id_fkey" FOREIGN KEY ("level_id") REFERENCES "public"."levels" ("id") ON DELETE RESTRICT ON UPDATE NO ACTION;

-- ----------------------------
-- Foreign Keys structure for table enrollments
-- ----------------------------
ALTER TABLE "public"."enrollments" ADD CONSTRAINT "enrollments_course_id_fkey" FOREIGN KEY ("course_id") REFERENCES "public"."courses" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;
ALTER TABLE "public"."enrollments" ADD CONSTRAINT "enrollments_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "public"."users" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;

-- ----------------------------
-- Foreign Keys structure for table lesson_characters
-- ----------------------------
ALTER TABLE "public"."lesson_characters" ADD CONSTRAINT "lesson_characters_character_id_fkey" FOREIGN KEY ("character_id") REFERENCES "public"."kana_characters" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;
ALTER TABLE "public"."lesson_characters" ADD CONSTRAINT "lesson_characters_lesson_id_fkey" FOREIGN KEY ("lesson_id") REFERENCES "public"."lessons" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;

-- ----------------------------
-- Foreign Keys structure for table lessons
-- ----------------------------
ALTER TABLE "public"."lessons" ADD CONSTRAINT "lessons_module_id_fkey" FOREIGN KEY ("module_id") REFERENCES "public"."modules" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;

-- ----------------------------
-- Foreign Keys structure for table module_prerequisites
-- ----------------------------
ALTER TABLE "public"."module_prerequisites" ADD CONSTRAINT "module_prerequisites_module_id_fkey" FOREIGN KEY ("module_id") REFERENCES "public"."modules" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;
ALTER TABLE "public"."module_prerequisites" ADD CONSTRAINT "module_prerequisites_required_module_id_fkey" FOREIGN KEY ("required_module_id") REFERENCES "public"."modules" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;

-- ----------------------------
-- Foreign Keys structure for table modules
-- ----------------------------
ALTER TABLE "public"."modules" ADD CONSTRAINT "modules_course_id_fkey" FOREIGN KEY ("course_id") REFERENCES "public"."courses" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;

-- ----------------------------
-- Foreign Keys structure for table question_options
-- ----------------------------
ALTER TABLE "public"."question_options" ADD CONSTRAINT "question_options_question_id_fkey" FOREIGN KEY ("question_id") REFERENCES "public"."questions" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;

-- ----------------------------
-- Foreign Keys structure for table question_sets
-- ----------------------------
ALTER TABLE "public"."question_sets" ADD CONSTRAINT "question_sets_lesson_id_fkey" FOREIGN KEY ("lesson_id") REFERENCES "public"."lessons" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;
ALTER TABLE "public"."question_sets" ADD CONSTRAINT "question_sets_module_id_fkey" FOREIGN KEY ("module_id") REFERENCES "public"."modules" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;

-- ----------------------------
-- Foreign Keys structure for table questions
-- ----------------------------
ALTER TABLE "public"."questions" ADD CONSTRAINT "questions_character_id_fkey" FOREIGN KEY ("character_id") REFERENCES "public"."kana_characters" ("id") ON DELETE SET NULL ON UPDATE NO ACTION;
ALTER TABLE "public"."questions" ADD CONSTRAINT "questions_question_set_id_fkey" FOREIGN KEY ("question_set_id") REFERENCES "public"."question_sets" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;

-- ----------------------------
-- Foreign Keys structure for table user_character_mastery
-- ----------------------------
ALTER TABLE "public"."user_character_mastery" ADD CONSTRAINT "user_character_mastery_character_id_fkey" FOREIGN KEY ("character_id") REFERENCES "public"."kana_characters" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;
ALTER TABLE "public"."user_character_mastery" ADD CONSTRAINT "user_character_mastery_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "public"."users" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;

-- ----------------------------
-- Foreign Keys structure for table user_lesson_progress
-- ----------------------------
ALTER TABLE "public"."user_lesson_progress" ADD CONSTRAINT "user_lesson_progress_lesson_id_fkey" FOREIGN KEY ("lesson_id") REFERENCES "public"."lessons" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;
ALTER TABLE "public"."user_lesson_progress" ADD CONSTRAINT "user_lesson_progress_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "public"."users" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;

-- ----------------------------
-- Foreign Keys structure for table user_module_progress
-- ----------------------------
ALTER TABLE "public"."user_module_progress" ADD CONSTRAINT "user_module_progress_module_id_fkey" FOREIGN KEY ("module_id") REFERENCES "public"."modules" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;
ALTER TABLE "public"."user_module_progress" ADD CONSTRAINT "user_module_progress_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "public"."users" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;
