ALTER TABLE public.levels ADD COLUMN tag varchar(40);

UPDATE public.levels
SET tag = CASE code
    WHEN 'N5' THEN 'Favorit Pemula'
    ELSE NULL
END
WHERE tag IS NULL;
