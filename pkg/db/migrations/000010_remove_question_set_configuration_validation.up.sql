BEGIN;

-- Question sets and their contents may be assembled across separate transactions.
-- Keep table constraints and foreign keys; remove the deferred content validation
-- that rejects published sets before their questions and options are inserted.
DROP TRIGGER IF EXISTS "question_sets_validate_exam_configuration"
ON "public"."question_sets";
DROP TRIGGER IF EXISTS "questions_validate_exam_configuration"
ON "public"."questions";
DROP TRIGGER IF EXISTS "question_options_validate_exam_configuration"
ON "public"."question_options";

DROP FUNCTION IF EXISTS "public"."trigger_validate_exam_question_set"();
DROP FUNCTION IF EXISTS "public"."trigger_validate_exam_question"();
DROP FUNCTION IF EXISTS "public"."trigger_validate_exam_option"();
DROP FUNCTION IF EXISTS "public"."validate_exam_question_set"(uuid);
DROP FUNCTION IF EXISTS "public"."validate_exam_question"(uuid);

COMMIT;
