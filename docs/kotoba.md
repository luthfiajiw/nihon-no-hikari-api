# Kotoba API and Supabase Storage

Kotoba is independent of courses, lessons, and assessments. This implementation
provides the backend contracts for the existing admin editor and student pages.
The frontend admin editor and student pages use these endpoints through their
authenticated SvelteKit proxy. Exercise counts are currently always 0.

## Configuration and deployment

Apply migration `000014_kotoba` with the existing migration runner before deploying
the API. It creates topics, vocabulary, and durable media metadata, and adds only
missing N5–N1 levels while preserving existing level names and order positions.
The down migration drops the three Kotoba tables and retains shared levels. Drain
media through normal topic deletion and cleanup before rolling back if remote
objects should also be removed; rolling back SQL alone does not delete Storage files.

Add these settings to the API environment (also passed through `compose.yml`):

```dotenv
SUPABASE_URL=https://your-project.supabase.co
SUPABASE_SERVICE_ROLE_KEY=your-server-only-service-role-key
SUPABASE_KOTOBA_COVER_BUCKET=kotoba-covers
SUPABASE_KOTOBA_AUDIO_BUCKET=kotoba-audio
```

The URL is the project root, without `/storage/v1`. Use a separate Supabase
project for development/testing. Never send the service-role key to the browser, 
commit it, or use the application's JWT as a Supabase access token.

Provision the two buckets once using the explicitly invoked command:

```sh
go run ./cmd/kotoba-storage
```

The command loads `.env`, creates missing buckets, and updates existing buckets
with the configured names to be public and enforce these restrictions. Existing
objects and their database references do not need to move:

| Bucket | MIME types | Maximum file size |
| --- | --- | --- |
| `kotoba-covers` | `image/png`, `image/jpeg` | 2 MiB (2,097,152 bytes) |
| `kotoba-audio` | `audio/mpeg` | 1 MiB (1,048,576 bytes) |

Use two distinct bucket names. Keep Storage RLS enabled and do not add anonymous
or authenticated listing/write policies for these buckets; uploads and deletes
use the backend's service-role key, while object reads use permanent public
URLs (`/storage/v1/object/public/...`). Bucket provisioning never runs on API
startup.

With a missing/invalid Supabase URL or a missing key, valid upload requests and
reads that need media URLs return 503. Empty lists and database-only operations
remain available. An upstream Storage upload failure returns 502; database
failures return 500. Responses never include service credentials or upstream
response bodies.

## Routes and access

All routes start with `/api/v1` and require `Authorization: Bearer <access-token>`.
`/admin/kotoba/*` additionally requires role `Admin`.

| Method | Path | Result |
| --- | --- | --- |
| POST | `/admin/kotoba/media` | Upload one file; 201 with media metadata |
| GET | `/admin/kotoba/topics` | Admin summaries, including inactive/hidden topics |
| POST | `/admin/kotoba/topics` | Create complete topic; 201 with saved detail |
| GET | `/admin/kotoba/topics/:id` | Full editor detail |
| PUT | `/admin/kotoba/topics/:id` | Replace complete form; 200 with saved detail |
| DELETE | `/admin/kotoba/topics/:id` | Delete topic and its words; 200 `{ "success": true }` |
| GET | `/kotoba/topics` | Student summaries; only active and visible topics |
| GET | `/kotoba/topics/:id` | Student detail; active hidden topics remain accessible by ID |

Inactive or missing student topics return 404. Unauthenticated requests return
401; non-admin writes/reads under `/admin` return 403. Invalid UUIDs return 400.
Responses are JSON with `success` and `data`, use snake_case fields and RFC3339
timestamps. Admin detail includes version, publication flags, and media IDs.
Student responses omit those admin fields while including permanent public media URLs.
List responses contain `data: []` when empty, without pagination. Summaries omit
the vocabulary array; detail includes vocabulary in its saved order.

`cover_url`, `audio_url`, and upload preview `url` are permanent public URLs
constructed locally from `SUPABASE_URL`, bucket name, and object path without
calling Supabase Storage on reads. `media_urls_expires_at` and `url_expires_at`
are deprecated and omitted from JSON responses. Do not persist generated URLs
in the database or treat them as file IDs. API responses still use
`Cache-Control: no-store`.

Both lists accept `level=N5|N4|N3|N2|N1` and `search` (literal, case-insensitive
substring in title/description). Admin additionally accepts
`status=all|active|inactive` and `sort=order|title` (default `order`). Students
accept `sort=newest|title|vocabulary|order` (default `newest`). Ties use topic UUID
ascending. Unknown filter values return 400. The student vocabulary search,
word-type filters, and word sorting can continue using the complete detail array.

## Upload then save

Upload each file separately with multipart fields `kind=cover|audio` and `file`.
The file part must declare its actual image or audio content type. `audio/mp3` is
accepted as an input alias and stored as `audio/mpeg`.

```sh
curl -X POST "$API_URL/api/v1/admin/kotoba/media" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -F 'kind=cover' -F 'file=@cover.png;type=image/png'
```

```json
{
  "success": true,
  "data": {
    "id": "11111111-1111-4111-8111-111111111111",
    "kind": "cover",
    "url": "https://your-project.supabase.co/storage/v1/object/public/kotoba-covers/11111111-1111-4111-8111-111111111111.png",
    "mime_type": "image/png",
    "size_bytes": 12345
  }
}
```

Cover names must end in `.png`, `.jpg`, or `.jpeg`. Audio names must end in `.mp3`.
The API checks actual bytes and MIME, decodes images, and validates complete MP3
frames plus decoded audio. Empty, corrupt, mismatched, and oversized files are
rejected. Images are additionally limited to 16 megapixels to bound decoding
memory. Each replacement gets a new UUID object path, avoiding overwrite/CDN
staleness. Recommended cover dimensions remain 1200 × 400, without enforcing an
aspect ratio. The current Fiber request limit is sufficient for a single 2 MiB
file plus multipart overhead; any reverse proxy must allow at least that much.

After uploading the cover and optional MP3s, POST a complete topic:

```json
{
  "title": "Sekolah",
  "level": "N5",
  "category": "Dasar",
  "description": "Kosakata lingkungan sekolah.",
  "order_index": 1,
  "is_active": true,
  "is_visible": true,
  "cover_media_id": "11111111-1111-4111-8111-111111111111",
  "vocabulary": [
    {
      "kanji": "学校",
      "hiragana": "がっこう",
      "furigana": [{ "text": "学校", "reading": "がっこう" }],
      "romaji": "gakkou",
      "translation": "sekolah",
      "example": {
        "japanese": "学校へ行きます。",
        "indonesian": "Saya pergi ke sekolah."
      },
      "type": "noun",
      "audio_media_id": null
    }
  ]
}
```

Create omits all vocabulary IDs. PUT sends the same complete form plus the latest
`version`. Existing words send their server IDs; new/duplicated words omit IDs.
The saved response supplies generated IDs, media URLs, counts, timestamps, and
the new version. Do not submit URL or object-path fields, response-only fields,
or browser `File` values in JSON. Unknown fields are rejected.

PUT replaces the complete vocabulary array atomically: omitted words are removed,
array position becomes `order_index` starting at 1, and existing IDs remain stable.
Sending `audio_media_id: null` (or omitting it in this full-replacement payload)
clears audio. A repeated media ID can be shared by duplicated words or other
topics. Media must be ready and match the required cover/audio kind; media IDs
and existing vocabulary IDs are checked on every save. Admins can reuse ready
media uploaded by another admin. A stale version returns 409; reload the topic
and reconcile changes before trying again.

Required values follow the editor: title up to 160 Unicode characters,
description up to 200, positive topic order, cover, at least one word, and all
word/example text fields. Category is `Dasar|Populer|Pemula`. Topic order can
tie. Audio is optional.

Word `type` is stored as the `kotoba_vocabulary_type` enum and uses English keys
in requests and responses. It is optional: a missing or blank value is saved as
`other`. Clients render the Indonesian label:

| `type` | Label |
|---|---|
| `verb` | Kata Kerja |
| `adjective` | Kata Sifat |
| `adverb` | Kata Keterangan |
| `question_word` | Kata Tanya |
| `noun` | Kata Benda |
| `family_member` | Anggota Keluarga |
| `counter` | Cara Menghitung |
| `day_date_month` | Hari, Tanggal & Bulan |
| `other` (default) | Lainnya |

Legacy labels such as `Kata Benda` are rejected with a `vocabulary.N.type` field error.

Furigana segments must concatenate exactly to the Japanese word. An empty array
generates one whole-word segment; kana-only words omit redundant readings.

Validation failures use index-based paths so the frontend can map errors to
its local word IDs:

```json
{
  "success": false,
  "error": "periksa kembali kolom yang ditandai",
  "field_errors": { "vocabulary.0.kanji": "Kolom ini wajib diisi." }
}
```

File size failures return 413. Invalid field values or media references return
400. Published content changes take effect immediately; no separate draft or
publish endpoint is added.

## Frontend field mapping

| Existing frontend state | API |
| --- | --- |
| `order`, `active`, `visible` | `order_index`, `is_active`, `is_visible` |
| `cover.file` / `word.audio.file` | Separate multipart upload; retain returned media ID |
| `cover.url` / `word.audio.url` | `cover_url` / `audio_url` |
| Media preview expiration | Deprecated; `media_urls_expires_at` / `url_expires_at` omitted for public URLs |
| Browser `new` / `new-word` / duplicate UUID | Omit topic ID on POST; omit IDs of new words |
| `createdAt` numeric value | Parse `created_at` RFC3339 timestamp |
| `vocabularyCount`, `exerciseCount` | `vocabulary_count`, `exercise_count` |

Retain the admin media IDs and version alongside editor state. Reconstruct media
previews with `{ url, file: null }`. Upload only new files, then save all topic
fields; leave the editor open and keep the unsaved form if upload/save fails.
The student integration uses `cover_url` for its banner and prefers
`audio_url` for individual/queued playback, with browser speech for words without
audio.

## Cleanup and monitoring

Media metadata is inserted with status `uploading` before contacting Supabase.
Only successfully uploaded objects become `ready`. Abandoned uploads and unused
media become eligible for removal after 24 hours. References clear that schedule;
detaching the final reference starts a new 24-hour retention period.

The API worker runs at startup and every five minutes, processing at most 100
eligible objects per run. It locks each claim with `SKIP LOCKED`, marks it
`deleting`, and persists a five-minute lease before calling Storage. Saves lock
the same media rows and reject deleting media, preventing reattachment races.
Shared covers/audio are never deleted while referenced. Missing remote objects
are treated as already removed; other failures retry after 5, 10, 20, 40, then
60 minutes. Leases and retry state survive restarts and multiple API instances.
Database transactions never remain open during remote uploads/deletions.

Observe structured log entries `kotoba media upload failed`, `kotoba media
cleanup failed`, and `kotoba cleanup worker failed`. Monitor pending/deleting
rows with high `attempts` or old cleanup dates. Cleanup is disabled with missing
Storage credentials and resumes automatically once they are restored. Remote
objects are removed through Storage's API, never by deleting `storage.objects`
rows with SQL.

## Verification

```sh
# Uses disposable databases; the named database is never migrated or cleared.
KANA_TEMPLATE_TEST_DATABASE_URL=postgresql://postgres:password@127.0.0.1:5432/postgres?sslmode=disable go test ./...
go vet ./...
go generate ./cmd
graphify update .
```

Tests cover migration up/down, existing-level seed collisions, CRUD and rollback,
shared media, stale/concurrent edits, exclusive cleanup claims, retry/restart,
authorization, visibility, multipart contracts, size boundaries, corrupt media,
and the Supabase REST adapter using a local HTTP server. Public storage tests
also cover public URL construction, bucket configuration, and HTTP upload/download.
Without the test DSN,
database tests explicitly skip; unit and HTTP tests still run. The MP3 fixture is
a generated 440 Hz test tone, not production vocabulary audio.

For a real staging smoke test, configure a separate Supabase project, provision
its buckets, upload one PNG and one MP3 through the admin endpoint, save an active
topic referencing them, and open the returned public URLs without passing the
service key to the browser. Verify the image displays and the MP3 plays. Replace
the cover, clear the audio, and confirm refreshed detail returns the new cover
and `audio_url: null`.

An explicit live adapter smoke test can also verify the configured buckets:

```powershell
$env:SUPABASE_STORAGE_LIVE_TEST='1'
go test -v -count=1 ./pkg/supabasestorage -run '^TestLivePublicStorage$'
Remove-Item Env:SUPABASE_STORAGE_LIVE_TEST
```

This test uploads a generated PNG and MP3 tone under UUID-scoped
`_kotoba-integration-checks/` paths, checks public downloads against the original
bytes, and removes its own objects.
It never changes existing objects or bucket configuration. Normal test runs skip
this live check unless explicitly opted in. Use dedicated development buckets.

References: [Storage buckets](https://supabase.com/docs/guides/storage/buckets/fundamentals),
[standard uploads](https://supabase.com/docs/guides/storage/uploads/standard-uploads),
[access control](https://supabase.com/docs/guides/storage/security/access-control),
[signed URLs](https://supabase.com/docs/reference/javascript/storage-from-createsignedurls),
[object deletion](https://supabase.com/docs/guides/storage/management/delete-objects).
