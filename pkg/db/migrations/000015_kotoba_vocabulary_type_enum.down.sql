ALTER TABLE public.kotoba_vocabularies ALTER COLUMN type DROP DEFAULT;

-- Lossy: types without a legacy label (adverb, question_word, family_member, counter,
-- day_date_month) collapse into 'Lainnya'.
ALTER TABLE public.kotoba_vocabularies
    ALTER COLUMN type TYPE text USING (
        CASE type::text
            WHEN 'noun' THEN 'Kata Benda'
            WHEN 'verb' THEN 'Kata Kerja'
            WHEN 'adjective' THEN 'Kata Sifat'
            ELSE 'Lainnya'
        END
    );

ALTER TABLE public.kotoba_vocabularies ADD CONSTRAINT kotoba_vocabularies_type_check
    CHECK (type IN ('Kata Benda', 'Kata Kerja', 'Kata Sifat', 'Lainnya'));

DROP TYPE IF EXISTS public.kotoba_vocabulary_type;
