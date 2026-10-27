# Backend module conventions

## Domain ownership

There is no `internal/admin` module. Administrative access is a handler concern:
each domain's `RegisterRoutes` registers its own user and administrative routes.
Administrative routes require both `AuthMiddleware()` and `RequireRole("Admin")`.
Register static paths such as `preview` and `reorder` before parameterized paths.

| Module | Ownership |
| --- | --- |
| `auth` | Authentication and the `/api/v1/admin/me` access check |
| `course` | Courses, learning modules, lessons, lesson content preview, levels, catalog, prerequisites, and module/lesson reorder |
| `question` | Question sets, questions, options, publication versions, attempts, writing snapshots, and kana stroke templates |
| `kotoba` | User and administrative Kotoba topics, vocabulary, media, and cleanup |
| `user` | User administration and roles |
| `speech` | Lesson speech tokens |

Course composes the unfiltered administrative catalog through question's read-only
`CatalogReader`. It does not implement question-set management or dispatch writes
to another domain. Question must not import course. A catalog filtered by
`course_id` returns only that course's modules and lessons; `courses` and
`question_sets` remain empty arrays.

Preserve existing module structure: course models stay in `course/model`, and
other modules retain their existing model placement and handler/service/repository
patterns. Administrative capabilities extend the original domain service and
repository through their content interfaces, without introducing an admin facade.

Administrative transactions use `db.WithTx` and transaction-bound repositories.
Keep the attempt transaction implementation intact. Draft storage continues to
use `admin_drafts` and the resource keys `course`, `module`, `lesson`, and
`question_set`; URL changes do not require a database migration.

## Go file naming

Every Go file under an `internal/<module>` directory starts with that module's
name, including files in nested model directories and integration tests.

- Primary layers: `[module]_[layer].go`, such as `course_repository.go`,
  `kotoba_handler.go`, `kotoba_service.go`, and `kotoba_model.go`.
- Feature layers: `[module]_[feature]_[layer].go`, such as
  `course_content_repository.go` and `question_content_service.go`.
- Helpers retain their own responsibility after the module prefix, such as
  `kotoba_media.go`, `question_stroke_evaluator.go`, and
  `question_writing_snapshot.go`.
- Nested models use the same module prefix, such as
  `course/model/course_lesson_model.go` and
  `course/model/course_lesson_progress_model.go`.
- Tests append `_test.go` to the source name; contract, database, and integration
  suites also start with the module name, such as
  `course_content_handler_contract_test.go` and
  `question_content_database_test.go`.
- The shared test helper is `internal/testdb/testdb_helper.go`.

These naming rules cover all internal modules. Entry points under `cmd`,
infrastructure under `pkg` and `config`, SQL migrations, and generated files keep
their own conventions. Renaming files does not move their Go packages or change
the domain's code structure.

## Administrative endpoint migration

All paths below are relative to `/api/v1`. Old content routes have no aliases.

| Old path | New path | Handler module |
| --- | --- | --- |
| `/admin/content/course` | `/admin/courses` | course |
| `/admin/content/module` | `/admin/modules` | course |
| `/admin/content/lesson` | `/admin/lessons` | course |
| `/admin/content/question_set` | `/admin/question-sets` | question |
| `/admin/content/module/reorder` | `/admin/modules/reorder` | course |
| `/admin/content/lesson/reorder` | `/admin/lessons/reorder` | course |
| `/admin/lesson-content/preview` | `/admin/lessons/preview` | course |
| `/admin/templates` and its children | Unchanged | question |
| `/admin/catalog`, `/admin/levels` | Unchanged | course |
| `/admin/me` | Unchanged | auth |

For the four content collections, preserve `GET` list, `POST` create,
`GET /:id`, `PATCH /:id`, `DELETE /:id`, `POST /:id/publish`, and
`POST /:id/restore`. Only modules and lessons expose `POST /reorder`.
Templates preserve list/detail, preview, create, and metadata/geometry update.

Request and response JSON, validation messages, status codes, full-payload PATCH
semantics, HTML sanitization, and null/empty/omitted fields are unchanged.
Publishing question-set changes retains lineage and creates a new version as
before. Template edits do not alter snapshots belonging to existing attempts.

This change is backend-only. Frontend callers must migrate to the new content
URLs as a separate change before using the refactored backend. User-facing API
paths, Kotoba administrative paths, and user administration paths are unchanged.

## Verification and graphify

Run `go test ./...`, `go vet ./...`, and `go build ./...`. Generate Swagger using
`go generate ./cmd`. Database suites use isolated disposable databases and keep
the existing configuration names:

- `KANA_TEMPLATE_TEST_DATABASE_URL`: course content, question publication,
  template/snapshot, and Kotoba database suites using the shared test helper.
- `ADMIN_REORDER_TEST_DATABASE_URL`: course reorder database suite.

After code changes, run `graphify update .`. Record architecture and naming
decisions with graphify's `save-result` mechanism, and confirm queries locate the
current domain handlers and this naming convention. Source nodes for deleted Go
paths must be removed by the update; historical URL mappings remain in this doc.
