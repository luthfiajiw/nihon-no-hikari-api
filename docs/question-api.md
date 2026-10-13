# Lesson question API

Semua endpoint membutuhkan header `Authorization: Bearer <access-token>`.

## Model soal

Question hanya memiliki dua tipe:

- `multiple_choice`
- `stroke_writing`

Konten question dipisahkan menjadi:

- `prompt_text`: instruksi, misalnya `Bagaimana cara membaca karakter ini?`
- `stimulus_text`: teks yang ditonjolkan, misalnya `あ` atau `saya mau pergi ke sekolah`
- `stimulus_media_url`: alternatif stimulus berupa gambar atau media
- `options`: pilihan jawaban yang dapat berisi teks Jepang maupun Indonesia
- `writing_guide`: untuk `stroke_writing`, berisi `character`, `romaji`,
  `view_box` (109), dan daftar `strokes` berurutan dengan `path` SVG serta
  32 titik acuan `{x,y}` per stroke
- `show_writing_guide`: untuk `stroke_writing`, menentukan apakah kanvas
  menampilkan jalur, nomor, dan arah stroke. Jika `false`, soal memakai mode
  menulis bebas: isi tulisan tidak dicocokkan dengan template.

Set writing Hiragana baris A berisi 25 soal berurutan: masing-masing lima soal
untuk あ, い, う, え, dan お. Tiga soal pertama per kana menampilkan panduan;
dua soal berikutnya memakai kanvas kosong.

Setiap question wajib mempunyai `prompt_text`. Pilihan ganda membutuhkan
`stimulus_text` atau `stimulus_media_url`; writing membutuhkan template stroke.

## 1. Daftar question set

```http
GET /api/v1/question-sets?lessonId={lessonId}
```

Respons berisi `kind`, konfigurasi kelulusan, jumlah soal efektif, `order_index`,
`is_passed`, dan `attempts_used` untuk pengguna aktif. Nilai `kind` pada endpoint lesson ini adalah `practice`.
Question set diurutkan
berdasarkan `order_index`. Hanya question set `practice` yang published yang
dikembalikan.

## 2. Detail question set

```http
GET /api/v1/question-sets/{questionSetId}
```

Respons berisi metadata question set, seluruh `questions`, stimulus, dan
`options` pada setiap question. Field internal `question_options.is_correct`
tidak dikirim.

Contoh question:

```json
{
  "id": "<uuid>",
  "question_type": "multiple_choice",
  "skill": "reading",
  "prompt_text": "Bagaimana cara membaca karakter ini?",
  "stimulus_text": "あ",
  "stimulus_media_url": null,
  "points": 1,
  "order_index": 0,
  "options": [
    { "id": "<uuid>", "label": "a", "media_url": null, "order_index": 0 },
    { "id": "<uuid>", "label": "i", "media_url": null, "order_index": 1 }
  ]
}
```

## 3. Mulai attempt

```http
POST /api/v1/question-sets/{questionSetId}/attempts
```

Endpoint membuat attempt dan mengembalikan soal beserta stimulus dan opsi tanpa
`is_correct`. Urutan soal mengikuti `shuffle_questions`, sedangkan jumlahnya
mengikuti `total_questions`. `expires_at` tersedia jika question set memiliki
batas waktu.

## 4. Submit attempt

```http
POST /api/v1/question-sets/{questionSetId}/attempts/{attemptId}/submit
Content-Type: application/json

{
  "answers": [
    {
      "question_id": "<uuid>",
      "selected_option_id": "<uuid>"
    }
  ]
}
```

Setiap jawaban yang diisi menggunakan salah satu dari `selected_option_id` atau
`stroke_input`, dan `question_id` tidak boleh duplikat. `answers` boleh kosong,
hanya memuat soal yang sudah dijawab, atau memuat soal tanpa jawaban. Soal yang
tidak dijawab mendapat 0 poin dan tetap masuk ke total bobot penilaian.
`stroke_input` harus berupa JSON valid dengan ukuran maksimal 64 KiB.
Untuk soal writing, bentuknya adalah array stroke berisi titik `{x,y}` pada
bidang 0–109. Pada soal berpanduan, tiap stroke memiliki 3–256 titik, jumlah
stroke harus sama dengan panduan, dan server menilai ulang urutan, arah,
panjang, serta kedekatan jalur. Persentase kecocokan disimpan dalam
`attempt_answers.accuracy`; goresan yang tidak sesuai mendapat 0 poin.
Pada soal tanpa panduan, satu atau lebih stroke bebas dengan 2–256 titik per
stroke (maksimal 16 stroke) mendapat poin penuh tanpa penilaian bentuk.
`accuracy` dibiarkan kosong karena tidak ada kecocokan yang dihitung.
Soal tanpa tulisan tetap mendapat 0 poin. Input yang tidak mengikuti format
menghasilkan HTTP 400.

## 5. Abandon attempt

```http
POST /api/v1/question-sets/{questionSetId}/attempts/{attemptId}/abandon
```

Endpoint ini dipanggil ketika pengguna meninggalkan halaman test sebelum submit.
Attempt harus dimiliki oleh pengguna yang sedang aktif dan masih berstatus
`in_progress`. Pemanggilan ulang untuk attempt yang sudah `abandoned` tetap
berhasil (idempotent), sedangkan attempt yang sudah `submitted` menghasilkan
`409 Conflict`. Attempt abandoned tidak dihitung dalam `attempts_used`, batas
`max_attempts`, maupun cooldown.

## Aturan penilaian

- Hanya skill `reading` dan `writing` yang diterima untuk kelulusan lesson.
- Pilihan ganda dinilai dari `question_options.is_correct`.
- Setiap pilihan ganda published wajib mempunyai minimal dua opsi dan tepat
  satu opsi benar.
- Nilai menggunakan bobot `questions.points`.
- Nilai total dan setiap skill yang muncul harus mencapai
  `question_sets.passing_score`.
- Jika lesson mempunyai beberapa question set published, pengguna harus pernah
  lulus semuanya.
- `max_attempts`, `cooldown_minutes`, dan `time_limit_seconds` diterapkan oleh
  server.
- Kegagalan attempt tidak menurunkan lesson yang sebelumnya sudah completed.
- `PUT .../progress` tidak menerima status `completed`; completion hanya dapat
  berasal dari penilaian attempt.

Skor akurasi tidak diterima dari client. Panduan stroke Hiragana baris A berasal
dari [KanjiVG r20260714](https://github.com/KanjiVG/kanjivg/releases/tag/r20260714),
© Ulrich Apel dan kontributor, [CC BY-SA 3.0](https://creativecommons.org/licenses/by-sa/3.0/).
SVG asli dan konversi titik acuan tersedia di `pkg/db/stroke_sources`.
