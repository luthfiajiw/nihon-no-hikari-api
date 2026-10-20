# Admin kana stroke templates

The admin **Template Kana** menu manages one KanjiVG SVG per template. Changes
apply immediately to the existing UUID, including script and character changes.
Question instructions are not rewritten automatically. There is no template
draft, publication, deletion, bulk import, or raw JSON editor.

## API

Every route requires a valid session and the `Admin` role.

| Method | Route under `/api/v1/admin` | Response `data` |
| --- | --- | --- |
| GET | `/templates` | Metadata array with `id`, `script`, `character`, `romaji`, `source_version`, `stroke_count` |
| GET | `/templates/{id}` | Metadata plus `stroke_data` |
| POST | `/templates/preview` | Metadata without ID, plus converted `stroke_data`; no database write |
| POST | `/templates` | Saved detail, HTTP 201 |
| PATCH | `/templates/{id}` | Updated detail, HTTP 200; ID preserved |

Creation and preview accept:

```json
{
  "script": "hiragana",
  "character": "あ",
  "romaji": "a",
  "source_version": "r20260714",
  "svg_content": "<svg xmlns=\"http://www.w3.org/2000/svg\" ...>...</svg>"
}
```

PATCH requires the complete metadata. Omit `svg_content` to retain current
geometry; changing script or character requires a replacement SVG. Blank SVG is
invalid. Metadata is trimmed and character is normalized to NFC. The character
must be one letter in the selected kana script; romaji is limited to eight
characters. The script/character/source-version tuple remains unique.

The SVG limit is 256 KiB. XML must contain a `0 0 109 109` SVG viewBox and a
matching KanjiVG `StrokePaths` group, with consecutive stroke IDs starting at 1.
Conversion supports `M/m`, `C/c`, `S/s` with one moveto per stroke. Transformed
stroke paths and unsupported commands are rejected. Stroke-number groups are
ignored, so their text transforms are accepted. XML entities are not resolved;
only extracted paths and computed points are returned, never uploaded markup.
Each cubic is sampled at 30 intervals then resampled to 32 equally spaced
reference points, rounded to three decimals, matching the pinned seed converter.
Only the converted data is stored; original SVG files are not retained.

Errors use `success: false` and `error`: invalid input 400, unauthenticated 401,
non-admin 403, missing ID 404, duplicate tuple 409, oversized SVG 413. Invalid
uploads and conflicting edits preserve the existing record atomically.

## Attempt consistency and deployment

Migration `000013_admin_kana_templates` backfills
`attempt_answers.writing_guide_snapshot` for existing writing answers using the
then-immutable templates. Choice answers retain NULL. The template trigger then
allows updates while continuing to reject deletion.

New attempts persist the exact `WritingGuide` returned in the start response,
inside the same transaction. Submit reads the snapshot, not the live template.
Missing or invalid writing snapshots cause an invalid-configuration error rather
than silently grading against a changed guide. Completed scores are not
recalculated. The student API shape and guide visibility settings are unchanged.

Deploy in this order:

1. Stop all old API instances and drain their transactions; temporarily stop new
   attempt/start and submit requests.
2. Back up the database and apply migration 000013, including the backfill.
3. Start the new API that writes and grades snapshots, then restore requests.
4. Release the admin frontend and verify add, preview, edit, and writing attempts.

Do not run old API writers after migration 000013: they would create answers
without snapshots. Downgrade requires stopped API instances and no in-progress
writing attempts; the down migration enforces this before restoring immutability
and dropping snapshots. Submitted scores remain stored.

## Verification

Run `go test ./...` for ordinary tests. To include full-schema migration, API,
real authentication/role, atomic update, and attempt snapshot integration tests:

```powershell
$env:KANA_TEMPLATE_TEST_DATABASE_URL = 'postgresql://postgres@127.0.0.1:55439/postgres?sslmode=disable'
go test ./internal/admin ./internal/question -run 'Templates|TemplateInput|WritingSnapshot' -v
```

Use an isolated PostgreSQL instance. Tests create a uniquely named database,
apply real migrations, and drop that database afterward. They do not install
tables in the supplied database. The account needs create/drop database rights.
Fixtures use the five pinned local KanjiVG guides plus synthetic katakana paths.

Frontend checks: `npm run check`, `npm run test:unit -- --run --project server`,
`npm run test:admin`, and `npm run build`. The admin browser test uses an isolated
HTTP fixture, including preview, file retention on failure, metadata-only edits,
identity changes, mobile layout, and selecting the template in a writing question.
