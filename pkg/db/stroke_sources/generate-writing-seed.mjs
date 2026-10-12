import { readFileSync, writeFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

// KanjiVG r20260714, CC BY-SA 3.0. Only the five pinned local SVGs are read.
const directory = dirname(fileURLToPath(import.meta.url));
const characters = [
  ['03042', 'あ', 'a'],
  ['03044', 'い', 'i'],
  ['03046', 'う', 'u'],
  ['03048', 'え', 'e'],
  ['0304a', 'お', 'o']
];

function samplePath(path) {
  const tokens = path.match(/[Mc]|-?(?:\d+\.?\d*|\.\d+)(?:e[-+]?\d+)?/gi);
  if (!tokens || tokens[0] !== 'M') throw new Error(`Unsupported path: ${path}`);
  let x = Number(tokens[1]);
  let y = Number(tokens[2]);
  let index = 3;
  const dense = [{ x, y }];
  while (index < tokens.length) {
    if (tokens[index] !== 'c') throw new Error(`Unsupported path command: ${tokens[index]}`);
    index++;
    while (index < tokens.length && tokens[index] !== 'c') {
      if (index + 5 >= tokens.length) throw new Error('Incomplete cubic path');
      const values = tokens.slice(index, index + 6).map(Number);
      const x1 = x + values[0], y1 = y + values[1];
      const x2 = x + values[2], y2 = y + values[3];
      const endX = x + values[4], endY = y + values[5];
      for (let step = 1; step <= 30; step++) {
        const t = step / 30, u = 1 - t;
        dense.push({
          x: u ** 3 * x + 3 * u ** 2 * t * x1 + 3 * u * t ** 2 * x2 + t ** 3 * endX,
          y: u ** 3 * y + 3 * u ** 2 * t * y1 + 3 * u * t ** 2 * y2 + t ** 3 * endY
        });
      }
      x = endX;
      y = endY;
      index += 6;
    }
  }
  const lengths = [0];
  for (let i = 1; i < dense.length; i++) {
    lengths.push(lengths[i - 1] + Math.hypot(dense[i].x - dense[i - 1].x, dense[i].y - dense[i - 1].y));
  }
  const total = lengths.at(-1);
  return Array.from({ length: 32 }, (_, sample) => {
    const target = total * sample / 31;
    let i = 1;
    while (i < lengths.length - 1 && lengths[i] < target) i++;
    const fraction = (target - lengths[i - 1]) / (lengths[i] - lengths[i - 1]);
    return {
      x: Number((dense[i - 1].x + fraction * (dense[i].x - dense[i - 1].x)).toFixed(3)),
      y: Number((dense[i - 1].y + fraction * (dense[i].y - dense[i - 1].y)).toFixed(3))
    };
  });
}

const escape = (value) => value.replaceAll("'", "''");
const guides = characters.map(([code, character, romaji]) => {
  const svg = readFileSync(join(directory, `${code}.svg`), 'utf8');
  const paths = [...svg.matchAll(/<path\s+id="[^"]+-s\d+"\s+d="([^"]+)"\s*\/>/g)].map((match) => match[1]);
  if (paths.length === 0) throw new Error(`No strokes found for ${character}`);
  const guide = { view_box: 109, strokes: paths.map((path) => ({ path, points: samplePath(path) })) };
  return { code, character, romaji, ...guide };
});
const rows = guides.map(({ code, character, romaji, view_box, strokes }) =>
  `  (md5('nnh:kana:${code}:r20260714')::uuid, 'hiragana', '${character}', '${romaji}', 'r20260714', '${escape(JSON.stringify({ view_box, strokes }))}'::jsonb)`
);
writeFileSync(join(directory, 'hiragana_a_guides.json'), `${JSON.stringify(guides, null, 2)}\n`);

const sql = `-- KanjiVG r20260714, © Ulrich Apel and contributors, CC BY-SA 3.0.
-- Source: https://github.com/KanjiVG/kanjivg/tree/r20260714/kanji
BEGIN;
INSERT INTO public.kana_stroke_templates (id, script, character, romaji, source_version, stroke_data)
VALUES\n${rows.join(',\n')}
ON CONFLICT (id) DO NOTHING;

DO $$
DECLARE
  target_lesson_uuid CONSTANT uuid := '6f92cb9b-d9ff-486f-b138-5a16a10afb09';
  target_set_uuid CONSTANT uuid := md5('nnh:hiragana-a:writing:r20260714')::uuid;
  existing_questions integer;
BEGIN
  IF NOT EXISTS (SELECT 1 FROM public.lessons AS l WHERE l.id = target_lesson_uuid) THEN
    RAISE EXCEPTION 'lesson Hiragana baris A tidak ditemukan';
  END IF;
  INSERT INTO public.question_sets (id, kind, module_id, lesson_id, title, skill, passing_score,
    total_questions, time_limit_seconds, max_attempts, cooldown_minutes, shuffle_questions, is_published, order_index)
  VALUES (target_set_uuid, 'practice', NULL, target_lesson_uuid, 'Latihan Menulis Hiragana Baris A', 'writing',
    100, 25, NULL, NULL, 0, false, false,
    (SELECT COALESCE(MAX(qs.order_index) + 1, 0)::int2 FROM public.question_sets AS qs WHERE qs.lesson_id = target_lesson_uuid))
  ON CONFLICT (id) DO NOTHING;

  SELECT COUNT(*) INTO existing_questions FROM public.questions AS q WHERE q.question_set_id = target_set_uuid;
  IF existing_questions = 5 THEN
    -- A one-time reset of the original five-question trial. Attempts must go
    -- before questions because their answers protect referenced questions.
    UPDATE public.user_lesson_progress AS progress
    SET status = 'in_progress', completed_at = NULL, updated_at = now()
    WHERE progress.lesson_id = target_lesson_uuid
      AND progress.user_id IN (
        SELECT DISTINCT a.user_id FROM public.attempts AS a WHERE a.question_set_id = target_set_uuid
      );
    DELETE FROM public.attempts AS a WHERE a.question_set_id = target_set_uuid;
    DELETE FROM public.questions AS q WHERE q.question_set_id = target_set_uuid;
  ELSIF existing_questions NOT IN (0, 25) THEN
    RAISE EXCEPTION 'jumlah soal writing lama tidak dikenali: %', existing_questions;
  END IF;

  UPDATE public.question_sets AS qs
  SET total_questions = 25, passing_score = 100, time_limit_seconds = NULL,
      shuffle_questions = false, is_published = false
  WHERE qs.id = target_set_uuid;
END $$;

INSERT INTO public.questions (id, question_set_id, stroke_template_id, question_type, skill,
  prompt_text, stimulus_text, stimulus_media_url, explanation, points, order_index, show_writing_guide)
SELECT md5('nnh:hiragana-a:writing:' || t.character || ':variant:' || variant.number)::uuid,
  md5('nnh:hiragana-a:writing:r20260714')::uuid, t.id, 'stroke_writing', 'writing',
  CASE WHEN variant.number <= 3
    THEN 'Telusuri huruf Hiragana untuk bunyi "' || t.romaji || '" sesuai panduan.'
    ELSE 'Tuliskan huruf Hiragana untuk bunyi "' || t.romaji || '" tanpa panduan.'
  END, NULL, NULL,
  'Karakter ' || t.character || ' dibaca "' || t.romaji || '".', 1,
  ((array_position(ARRAY['あ','い','う','え','お'], t.character) - 1) * 5 + variant.number - 1)::int2,
  variant.number <= 3
FROM public.kana_stroke_templates t
CROSS JOIN generate_series(1, 5) AS variant(number)
WHERE t.script = 'hiragana' AND t.character = ANY(ARRAY['あ','い','う','え','お']) AND t.source_version = 'r20260714'
ON CONFLICT (id) DO NOTHING;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM public.questions AS q
    WHERE q.question_set_id = md5('nnh:hiragana-a:writing:r20260714')::uuid
    GROUP BY q.stroke_template_id
    HAVING COUNT(*) <> 5 OR COUNT(*) FILTER (WHERE q.show_writing_guide) <> 3
  ) OR (SELECT COUNT(*) FROM public.questions
      WHERE question_set_id = md5('nnh:hiragana-a:writing:r20260714')::uuid) <> 25 THEN
    RAISE EXCEPTION 'question set writing baris A harus berisi 25 soal: tiga berpanduan dan dua tanpa panduan per kana';
  END IF;
END $$;

UPDATE public.question_sets SET is_published = true
WHERE id = md5('nnh:hiragana-a:writing:r20260714')::uuid;
COMMIT;
`;
writeFileSync(join(directory, '..', 'seeders', 'hiragana_a_writing_question_set.sql'), sql);
