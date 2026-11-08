ALTER TABLE public.levels
    ADD COLUMN points jsonb NOT NULL DEFAULT '[]'::jsonb
    CONSTRAINT levels_points_array_check CHECK (jsonb_typeof(points) = 'array');

UPDATE public.levels
SET points = CASE code
    WHEN 'N5' THEN '["Modul Hiragana & Katakana lengkap", "800+ Flashcards audio penutur asli", "15 Bab tata bahasa dasar & 100 Kanji", "3x Simulasi Ujian JLPT N5", "AI Sensei Tutor 24/7"]'::jsonb
    WHEN 'N4' THEN '["14 Materi percakapan kontekstual", "Bank pola tata bahasa (Bunpo) N4", "1.500 Kosakata & 300 Kanji SRS", "5x Simulasi Ujian JLPT N4", "Penjadwalan belajar AI adaptif"]'::jsonb
    WHEN 'N3' THEN '["650 Kanji & 3.750 Kosakata lengkap", "Dokkai & Choukai intensif standar ujian", "Analisis kelemahan AI otomatis", "8x Simulasi Tryout N3 dengan timer", "Diskusi komunitas & bimbingan Sensei"]'::jsonb
    WHEN 'N2' THEN '["Artikel berita & ekspresi bisnis formal", "Pelatihan Choukai tempo asli Jepang", "1.000 Kanji & 6.000 Kosakata bisnis", "10x Tryout N2 resmi dengan pembahasan", "Feedback AI Essay & Dokkai mendalam"]'::jsonb
    WHEN 'N1' THEN '["Nuansa bahasa idiomatis tingkat tinggi", "Materi debat, opini & editorial koran", "2.000 Kanji & 10.000 Kosakata komplit", "Tryout N1 tanpa batas & simulasi nyata", "Simulasi wawancara kerja & beasiswa AI"]'::jsonb
    ELSE points
END
WHERE points = '[]'::jsonb;
