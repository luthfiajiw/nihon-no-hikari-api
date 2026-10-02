-- Setiap lesson bergantung pada lesson tepat sebelumnya di module yang sama.
-- Lesson pertama pada setiap module tidak memiliki prerequisite.
WITH ordered_lessons AS (
  SELECT
    "id" AS "lesson_id",
    LAG("id") OVER (
      PARTITION BY "module_id"
      ORDER BY "order_index", "id"
    ) AS "required_lesson_id"
  FROM "public"."lessons"
)
INSERT INTO "public"."lesson_prerequisites" (
  "lesson_id",
  "required_lesson_id"
)
SELECT
  "lesson_id",
  "required_lesson_id"
FROM ordered_lessons
WHERE "required_lesson_id" IS NOT NULL
ON CONFLICT ("lesson_id", "required_lesson_id") DO NOTHING;
