UPDATE "public"."question_sets"
SET "time_limit_seconds" = NULL
WHERE "time_limit_seconds" = 0;

ALTER TABLE "public"."question_sets"
DROP CONSTRAINT IF EXISTS "question_sets_time_limit_positive_check",
ADD CONSTRAINT "question_sets_time_limit_positive_check"
CHECK ("time_limit_seconds" IS NULL OR "time_limit_seconds" > 0);
