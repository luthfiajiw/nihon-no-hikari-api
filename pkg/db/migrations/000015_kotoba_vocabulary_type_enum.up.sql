CREATE TYPE public.kotoba_vocabulary_type AS ENUM (
    'verb',
    'adjective',
    'adverb',
    'question_word',
    'noun',
    'family_member',
    'counter',
    'day_date_month',
    'other'
);

ALTER TABLE public.kotoba_vocabularies DROP CONSTRAINT IF EXISTS kotoba_vocabularies_type_check;

-- Legacy Indonesian labels map onto their English enum values; anything else becomes 'other'.
ALTER TABLE public.kotoba_vocabularies
    ALTER COLUMN type TYPE public.kotoba_vocabulary_type USING (
        CASE type
            WHEN 'Kata Benda' THEN 'noun'
            WHEN 'Kata Kerja' THEN 'verb'
            WHEN 'Kata Sifat' THEN 'adjective'
            ELSE 'other'
        END
    )::public.kotoba_vocabulary_type,
    ALTER COLUMN type SET DEFAULT 'other';
