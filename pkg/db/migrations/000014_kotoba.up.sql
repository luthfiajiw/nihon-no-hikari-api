WITH missing AS (
    SELECT code, row_number() OVER (ORDER BY position) AS offset_index
    FROM (VALUES ('N5', 1), ('N4', 2), ('N3', 3), ('N2', 4), ('N1', 5)) AS jlpt(code, position)
    WHERE NOT EXISTS (SELECT 1 FROM public.levels l WHERE l.code = jlpt.code)
)
INSERT INTO public.levels(code, name, order_index)
SELECT code, code, (SELECT coalesce(max(order_index), -1) FROM public.levels) + offset_index FROM missing;

CREATE TABLE public.kotoba_media (
    id uuid PRIMARY KEY,
    kind text NOT NULL CHECK (kind IN ('cover', 'audio')),
    bucket text NOT NULL,
    object_path text NOT NULL,
    mime_type text NOT NULL,
    size_bytes bigint NOT NULL CHECK (size_bytes > 0),
    uploaded_by uuid NOT NULL REFERENCES public.users(id),
    status text NOT NULL DEFAULT 'uploading' CHECK (status IN ('uploading', 'ready', 'deleting')),
    cleanup_after timestamptz DEFAULT (now() + interval '24 hours'),
    retry_after timestamptz,
    attempts integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(bucket, object_path),
    CHECK ((kind = 'cover' AND mime_type IN ('image/png', 'image/jpeg') AND size_bytes <= 2097152)
        OR (kind = 'audio' AND mime_type = 'audio/mpeg' AND size_bytes <= 1048576))
);

CREATE TABLE public.kotoba_topics (
    id uuid PRIMARY KEY,
    level_id uuid NOT NULL REFERENCES public.levels(id),
    title varchar(160) NOT NULL CHECK (length(btrim(title)) > 0),
    category text NOT NULL CHECK (category IN ('Dasar', 'Populer', 'Pemula')),
    description varchar(200) NOT NULL CHECK (length(btrim(description)) > 0),
    order_index integer NOT NULL CHECK (order_index > 0),
    is_active boolean NOT NULL DEFAULT false,
    is_visible boolean NOT NULL DEFAULT true,
    cover_media_id uuid NOT NULL REFERENCES public.kotoba_media(id),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE public.kotoba_vocabularies (
    id uuid PRIMARY KEY,
    topic_id uuid NOT NULL REFERENCES public.kotoba_topics(id) ON DELETE CASCADE,
    kanji text NOT NULL CHECK (length(btrim(kanji)) > 0),
    hiragana text NOT NULL CHECK (length(btrim(hiragana)) > 0),
    furigana jsonb NOT NULL CHECK (jsonb_typeof(furigana) = 'array'),
    romaji text NOT NULL CHECK (length(btrim(romaji)) > 0),
    translation text NOT NULL CHECK (length(btrim(translation)) > 0),
    example_japanese text NOT NULL CHECK (length(btrim(example_japanese)) > 0),
    example_indonesian text NOT NULL CHECK (length(btrim(example_indonesian)) > 0),
    type text NOT NULL CHECK (type IN ('Kata Benda', 'Kata Kerja', 'Kata Sifat', 'Lainnya')),
    order_index integer NOT NULL CHECK (order_index > 0),
    audio_media_id uuid REFERENCES public.kotoba_media(id),
    UNIQUE(topic_id, order_index)
);

CREATE INDEX idx_kotoba_topics_catalog ON public.kotoba_topics(level_id, is_active, is_visible, order_index, id);
CREATE INDEX idx_kotoba_topics_cover ON public.kotoba_topics(cover_media_id);
CREATE INDEX idx_kotoba_vocabularies_audio ON public.kotoba_vocabularies(audio_media_id);
CREATE INDEX idx_kotoba_media_cleanup ON public.kotoba_media(cleanup_after, retry_after);
