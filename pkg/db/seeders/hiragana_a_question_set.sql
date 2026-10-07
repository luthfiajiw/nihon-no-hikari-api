-- Question set lengkap untuk lesson "Baris A (あ)".
-- Jalankan setelah migration 000005_harden_exam_question_schema.
--
-- Komposisi:
--   5 karakter (あ, い, う, え, お)
--   x 3 variasi Hiragana -> romaji (reading)
--   x 3 variasi romaji -> Hiragana (reading/recognition)
--   = 30 soal multiple choice, masing-masing dengan 4 opsi.

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

WITH characters (position, kana, romaji) AS (
  VALUES
    (1, 'あ', 'a'),
    (2, 'い', 'i'),
    (3, 'う', 'u'),
    (4, 'え', 'e'),
    (5, 'お', 'o')
),
prompt_variants (variant, direction, prompt_text, skill) AS (
  VALUES
    (
      1,
      'kana_to_romaji',
      'Bagaimana cara membaca karakter Hiragana ini?',
      'reading'::"public"."skill_type"
    ),
    (
      2,
      'kana_to_romaji',
      'Pilih pelafalan yang tepat untuk karakter berikut.',
      'reading'::"public"."skill_type"
    ),
    (
      3,
      'kana_to_romaji',
      'Karakter Hiragana berikut dibaca sebagai apa?',
      'reading'::"public"."skill_type"
    ),
    (
      4,
      'romaji_to_kana',
      'Pilih karakter Hiragana yang sesuai dengan bunyi berikut.',
      'reading'::"public"."skill_type"
    ),
    (
      5,
      'romaji_to_kana',
      'Manakah penulisan Hiragana yang tepat untuk bunyi ini?',
      'reading'::"public"."skill_type"
    ),
    (
      6,
      'romaji_to_kana',
      'Ubah bunyi berikut ke dalam bentuk Hiragana.',
      'reading'::"public"."skill_type"
    )
),
question_data AS (
  SELECT
    MD5(
      'nihon-no-hikari:hiragana-a:'
      || characters.position::text
      || ':'
      || prompt_variants.variant::text
    )::uuid AS id,
    ((characters.position - 1) * 6 + prompt_variants.variant - 1)::int2 AS order_index,
    characters.kana,
    characters.romaji,
    prompt_variants.direction,
    prompt_variants.prompt_text,
    prompt_variants.skill
  FROM characters
  CROSS JOIN prompt_variants
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
  question_data.skill,
  question_data.prompt_text,
  CASE
    WHEN question_data.direction = 'kana_to_romaji' THEN question_data.kana
    ELSE question_data.romaji
  END,
  NULL,
  CASE
    WHEN question_data.direction = 'kana_to_romaji'
      THEN FORMAT(
        'Karakter %s dibaca "%s".',
        question_data.kana,
        question_data.romaji
      )
    ELSE FORMAT(
      'Bunyi "%s" ditulis %s dalam Hiragana.',
      question_data.romaji,
      question_data.kana
    )
  END,
  1,
  question_data.order_index
FROM question_data
ON CONFLICT ("id") DO NOTHING;

WITH characters (position, kana, romaji) AS (
  VALUES
    (1, 'あ', 'a'),
    (2, 'い', 'i'),
    (3, 'う', 'u'),
    (4, 'え', 'e'),
    (5, 'お', 'o')
),
prompt_variants (variant, direction) AS (
  VALUES
    (1, 'kana_to_romaji'),
    (2, 'kana_to_romaji'),
    (3, 'kana_to_romaji'),
    (4, 'romaji_to_kana'),
    (5, 'romaji_to_kana'),
    (6, 'romaji_to_kana')
),
question_data AS (
  SELECT
    MD5(
      'nihon-no-hikari:hiragana-a:'
      || characters.position::text
      || ':'
      || prompt_variants.variant::text
    )::uuid AS question_id,
    characters.position,
    prompt_variants.variant,
    prompt_variants.direction
  FROM characters
  CROSS JOIN prompt_variants
),
option_offsets (offset_value) AS (
  VALUES (0), (1), (2), (3)
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
      WHEN question_data.direction = 'kana_to_romaji' THEN candidate.romaji
      ELSE candidate.kana
    END AS label,
    (option_offsets.offset_value = 0) AS is_correct,
    (
      (option_offsets.offset_value + question_data.variant - 1) % 4
    )::int2 AS order_index
  FROM question_data
  CROSS JOIN option_offsets
  JOIN characters AS candidate
    ON candidate.position = (
      (question_data.position - 1 + option_offsets.offset_value) % 5
    ) + 1
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
ON CONFLICT ("id") DO NOTHING;

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
  ) THEN
    RAISE EXCEPTION
      'setiap soal harus memiliki empat opsi dan tepat satu jawaban benar';
  END IF;

  PERFORM "public"."validate_exam_question_set"(target_question_set_id);
END;
$$;

COMMIT;
