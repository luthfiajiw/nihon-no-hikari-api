# Lesson question API

Semua endpoint membutuhkan header `Authorization: Bearer <access-token>`.

## 1. Daftar question set

```http
GET /api/v1/courses/{courseId}/lessons/{lessonId}/question-sets
```

Respons berisi konfigurasi kelulusan, jumlah soal efektif, `is_passed`, dan `attempts_used` untuk pengguna aktif. Hanya question set `practice` yang published yang dikembalikan.

## 2. Detail question set

```http
GET /api/v1/courses/{courseId}/lessons/{lessonId}/question-sets/{questionSetId}
```

Respons berisi metadata question set, seluruh `questions`, dan `options` pada setiap question. Field internal penilaian seperti `questions.correct_answer` dan `question_options.is_correct` tidak dikirim.

## 3. Mulai attempt

```http
POST /api/v1/courses/{courseId}/lessons/{lessonId}/question-sets/{questionSetId}/attempts
```

Endpoint ini membuat attempt dan mengembalikan soal beserta opsi tanpa `is_correct` atau `correct_answer`. Urutan soal mengikuti `shuffle_questions`, sedangkan jumlahnya mengikuti `total_questions`. `expires_at` tersedia jika question set memiliki batas waktu.

## 4. Submit attempt

```http
POST /api/v1/courses/{courseId}/lessons/{lessonId}/question-sets/{questionSetId}/attempts/{attemptId}/submit
Content-Type: application/json

{
  "answers": [
    {
      "question_id": "<uuid>",
      "selected_option_id": "<uuid>"
    },
    {
      "question_id": "<uuid>",
      "answer_text": "あ"
    }
  ]
}
```

Setiap jawaban wajib menggunakan tepat satu dari `selected_option_id` atau `answer_text`. Semua soal pada attempt harus dijawab dan tidak boleh duplikat.

## Aturan penilaian

- Hanya skill `reading` dan `writing` yang diterima untuk kelulusan lesson.
- Opsi dinilai dari `question_options.is_correct`; teks dinilai case-insensitive setelah spasi awal/akhir dibuang dan Unicode dinormalisasi terhadap `questions.correct_answer`.
- Nilai menggunakan bobot `questions.points`.
- Nilai total dan setiap skill yang muncul harus mencapai `question_sets.passing_score`.
- Jika lesson mempunyai beberapa question set published, pengguna harus pernah lulus semuanya.
- `max_attempts`, `cooldown_minutes`, dan `time_limit_seconds` diterapkan oleh server.
- Kegagalan attempt tidak menurunkan lesson yang sebelumnya sudah completed.
- `PUT .../progress` tidak menerima status `completed`; completion hanya dapat berasal dari penilaian attempt.

`stroke_writing` belum dinilai oleh endpoint ini. Gunakan jawaban teks untuk writing sampai tersedia evaluator stroke server-side; menerima skor akurasi dari klien akan mudah dimanipulasi.
