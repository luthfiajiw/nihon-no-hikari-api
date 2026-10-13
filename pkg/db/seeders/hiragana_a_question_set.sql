-- Question set lengkap untuk lesson "Baris A (あ)".
-- Jalankan setelah migration 000005_harden_exam_question_schema.
--
-- Komposisi: 25 pasangan dari あ, い, う, え, お dan 5 rangkaian tiga kana.
-- Urutan campur, 15 soal Hiragana -> romaji dan 15 soal romaji -> Hiragana.
-- Setiap soal multiple choice memiliki 4 opsi.

BEGIN;

DO $$
DECLARE
  target_lesson_id constant uuid := '6f92cb9b-d9ff-486f-b138-5a16a10afb09';
  target_question_set_id constant uuid := '6f92cb9b-d9ff-486f-b138-5a16a10a0030';
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM "public"."lessons"
    WHERE "id" = target_lesson_id
  ) THEN
    RAISE EXCEPTION 'lesson % tidak ditemukan', target_lesson_id;
  END IF;

  IF EXISTS (
    SELECT 1
    FROM "public"."question_sets"
    WHERE "id" = target_question_set_id
      AND "lesson_id" IS DISTINCT FROM target_lesson_id
  ) THEN
    RAISE EXCEPTION
      'question set id % sudah digunakan oleh lesson lain',
      target_question_set_id;
  END IF;
END;
$$;

WITH next_order AS (
  SELECT COALESCE(MAX("order_index") + 1, 0)::int2 AS "order_index"
  FROM "public"."question_sets"
  WHERE "lesson_id" = '6f92cb9b-d9ff-486f-b138-5a16a10afb09'
    AND "id" <> '6f92cb9b-d9ff-486f-b138-5a16a10a0030'
)
INSERT INTO "public"."question_sets" (
  "id",
  "kind",
  "module_id",
  "lesson_id",
  "title",
  "skill",
  "passing_score",
  "total_questions",
  "time_limit_seconds",
  "max_attempts",
  "cooldown_minutes",
  "shuffle_questions",
  "is_published",
  "order_index"
)
SELECT
  '6f92cb9b-d9ff-486f-b138-5a16a10a0030',
  'practice'::"public"."question_set_kind",
  NULL,
  '6f92cb9b-d9ff-486f-b138-5a16a10afb09',
  'Latihan Hiragana Baris A',
  'reading'::"public"."skill_type",
  80,
  30,
  600,
  NULL,
  0,
  true,
  true,
  next_order."order_index"
FROM next_order
ON CONFLICT ("id") DO NOTHING;

WITH samples (order_index, romaji) AS (
  VALUES
    (0, 'aa'), (1, 'ii'), (2, 'uie'), (3, 'ia'), (4, 'eoa'),
    (5, 'eu'), (6, 'uu'), (7, 'ui'), (8, 'ua'), (9, 'iu'),
    (10, 'ao'), (11, 'oe'), (12, 'uo'), (13, 'ou'), (14, 'ie'),
    (15, 'ei'), (16, 'oa'), (17, 'oo'), (18, 'ae'), (19, 'aoi'),
    (20, 'iau'), (21, 'eo'), (22, 'ee'), (23, 'oie'), (24, 'ea'),
    (25, 'ue'), (26, 'au'), (27, 'oi'), (28, 'io'), (29, 'ai')
),
question_data AS (
  SELECT
    MD5(
      'nihon-no-hikari:hiragana-a:'
      || (samples.order_index / 6 + 1)::text
      || ':'
      || (samples.order_index % 6 + 1)::text
    )::uuid AS id,
    samples.order_index::int2,
    TRANSLATE(samples.romaji, 'aiueo', 'あいうえお') AS kana,
    samples.romaji
  FROM samples
)
INSERT INTO "public"."questions" (
  "id",
  "question_set_id",
  "question_type",
  "skill",
  "prompt_text",
  "stimulus_text",
  "stimulus_media_url",
  "explanation",
  "points",
  "order_index"
)
SELECT
  question_data.id,
  '6f92cb9b-d9ff-486f-b138-5a16a10a0030',
  'multiple_choice'::"public"."question_type",
  'reading'::"public"."skill_type",
  CASE
    WHEN question_data.order_index % 2 = 0
      THEN 'Bagaimana rangkaian Hiragana ini dibaca dalam romaji?'
    ELSE 'Pilih rangkaian Hiragana yang sesuai dengan romaji berikut.'
  END,
  CASE
    WHEN question_data.order_index % 2 = 0 THEN question_data.kana
    ELSE question_data.romaji
  END,
  NULL,
  CASE
    WHEN question_data.order_index % 2 = 0
      THEN FORMAT(
        'Rangkaian %s dibaca "%s".',
        question_data.kana,
        question_data.romaji
      )
    ELSE FORMAT(
      'Rangkaian "%s" ditulis %s dalam Hiragana.',
      question_data.romaji,
      question_data.kana
    )
  END,
  1,
  question_data.order_index
FROM question_data
ON CONFLICT ("id") DO UPDATE SET
  "prompt_text" = EXCLUDED."prompt_text",
  "stimulus_text" = EXCLUDED."stimulus_text",
  "explanation" = EXCLUDED."explanation"
WHERE (
  "questions"."prompt_text",
  "questions"."stimulus_text",
  "questions"."explanation"
) IS DISTINCT FROM (
  EXCLUDED."prompt_text",
  EXCLUDED."stimulus_text",
  EXCLUDED."explanation"
);

WITH samples (order_index, romaji) AS (
  VALUES
    (0, 'aa'), (1, 'ii'), (2, 'uie'), (3, 'ia'), (4, 'eoa'),
    (5, 'eu'), (6, 'uu'), (7, 'ui'), (8, 'ua'), (9, 'iu'),
    (10, 'ao'), (11, 'oe'), (12, 'uo'), (13, 'ou'), (14, 'ie'),
    (15, 'ei'), (16, 'oa'), (17, 'oo'), (18, 'ae'), (19, 'aoi'),
    (20, 'iau'), (21, 'eo'), (22, 'ee'), (23, 'oie'), (24, 'ea'),
    (25, 'ue'), (26, 'au'), (27, 'oi'), (28, 'io'), (29, 'ai')
),
question_data AS (
  SELECT
    MD5(
      'nihon-no-hikari:hiragana-a:'
      || (samples.order_index / 6 + 1)::text
      || ':'
      || (samples.order_index % 6 + 1)::text
    )::uuid AS question_id,
    samples.order_index,
    samples.romaji,
    (samples.order_index % LENGTH(samples.romaji) + 1) AS changed_position
  FROM samples
),
option_offsets (offset_value) AS (
  VALUES (0), (1), (2), (3)
),
characters (position, romaji) AS (
  VALUES (1, 'a'), (2, 'i'), (3, 'u'), (4, 'e'), (5, 'o')
),
option_data AS (
  SELECT
    MD5(
      question_data.question_id::text
      || ':option:'
      || option_offsets.offset_value::text
    )::uuid AS id,
    question_data.question_id,
    CASE
      WHEN question_data.order_index % 2 = 0 THEN candidate_answer.romaji
      ELSE TRANSLATE(candidate_answer.romaji, 'aiueo', 'あいうえお')
    END AS label,
    (option_offsets.offset_value = 0) AS is_correct,
    (
      (option_offsets.offset_value + question_data.order_index % 6) % 4
    )::int2 AS order_index
  FROM question_data
  CROSS JOIN option_offsets
  JOIN characters AS original
    ON original.romaji = SUBSTRING(
      question_data.romaji FROM question_data.changed_position FOR 1
    )
  JOIN characters AS candidate
    ON candidate.position = (
      (original.position - 1 + option_offsets.offset_value) % 5
    ) + 1
  CROSS JOIN LATERAL (
    SELECT OVERLAY(
      question_data.romaji PLACING candidate.romaji
      FROM question_data.changed_position FOR 1
    ) AS romaji
  ) AS candidate_answer
)
INSERT INTO "public"."question_options" (
  "id",
  "question_id",
  "label",
  "media_url",
  "is_correct",
  "order_index"
)
SELECT
  option_data.id,
  option_data.question_id,
  option_data.label,
  NULL,
  option_data.is_correct,
  option_data.order_index
FROM option_data
ON CONFLICT ("id") DO UPDATE SET
  "label" = EXCLUDED."label"
WHERE "question_options"."label" IS DISTINCT FROM EXCLUDED."label";

DO $$
DECLARE
  target_question_set_id constant uuid := '6f92cb9b-d9ff-486f-b138-5a16a10a0030';
  question_count integer;
  option_count integer;
BEGIN
  SELECT COUNT(*)::integer
  INTO question_count
  FROM "public"."questions"
  WHERE "question_set_id" = target_question_set_id;

  SELECT COUNT(*)::integer
  INTO option_count
  FROM "public"."question_options" AS qo
  JOIN "public"."questions" AS q ON q."id" = qo."question_id"
  WHERE q."question_set_id" = target_question_set_id;

  IF question_count <> 30 OR option_count <> 120 THEN
    RAISE EXCEPTION
      'seeder Hiragana baris A tidak lengkap: % questions, % options',
      question_count,
      option_count;
  END IF;

  IF EXISTS (
    SELECT 1
    FROM "public"."questions" AS q
    LEFT JOIN "public"."question_options" AS qo ON qo."question_id" = q."id"
    WHERE q."question_set_id" = target_question_set_id
    GROUP BY q."id"
    HAVING COUNT(qo."id") <> 4
      OR COUNT(qo."id") FILTER (WHERE qo."is_correct") <> 1
      OR COUNT(DISTINCT qo."label") <> 4
  ) THEN
    RAISE EXCEPTION
      'setiap soal harus memiliki empat opsi berbeda dan tepat satu jawaban benar';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM "public"."questions" AS q
    JOIN "public"."question_options" AS qo
      ON qo."question_id" = q."id"
    WHERE q."question_set_id" = target_question_set_id
      AND (
        LENGTH(q."stimulus_text") NOT BETWEEN 2 AND 3
        OR LENGTH(qo."label") <> LENGTH(q."stimulus_text")
        OR (
          qo."is_correct"
          AND qo."label" <> CASE
            WHEN q."order_index" % 2 = 0
              THEN TRANSLATE(q."stimulus_text", 'あいうえお', 'aiueo')
            ELSE TRANSLATE(q."stimulus_text", 'aiueo', 'あいうえお')
          END
        )
      )
  ) THEN
    RAISE EXCEPTION
      'rangkaian kana, romaji, atau jawaban benar tidak sesuai';
  END IF;

END;
$$;

COMMIT;
