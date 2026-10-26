package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"nihon-no-hikari-api/pkg/db"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestParseReorder(t *testing.T) {
	parent, id := uuid.New(), uuid.New()
	valid, _ := json.Marshal(ReorderRequest{ParentID: parent, OrderedIDs: []uuid.UUID{id}})
	for _, kind := range []string{"module", "lesson"} {
		if _, err := validatedReorderInput(kind, valid); err != nil {
			t.Fatal(err)
		}
	}
	for _, raw := range []string{
		`{`, `null`, `[]`, `{}`, `{"parent_id":"bad","ordered_ids":[]}`,
		fmt.Sprintf(`{"parent_id":%q,"ordered_ids":null}`, parent),
		fmt.Sprintf(`{"parent_id":%q,"ordered_ids":[%q,%q]}`, parent, id, strings.ToUpper(id.String())),
		fmt.Sprintf(`{"parent_id":%q,"ordered_ids":[%q]}`, parent, uuid.Nil),
	} {
		if _, err := validatedReorderInput("module", []byte(raw)); err == nil {
			t.Fatalf("accepted invalid payload %s", raw)
		}
	}
	if _, err := validatedReorderInput("course", valid); err == nil {
		t.Fatal("accepted course reordering")
	}
	tooMany := ReorderRequest{ParentID: parent, OrderedIDs: make([]uuid.UUID, 32769)}
	raw, _ := json.Marshal(tooMany)
	if _, err := validatedReorderInput("lesson", raw); err == nil {
		t.Fatal("accepted smallint overflow")
	}
}

// The supplied instance is only used to create/drop a uniquely named test database.
// Never creates tables in an application database. See docs/admin-reorder-tests.md.
func TestReorderDatabase(t *testing.T) {
	dsn := os.Getenv("ADMIN_REORDER_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set ADMIN_REORDER_TEST_DATABASE_URL to an isolated PostgreSQL instance")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	database := "nnh_reorder_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, `CREATE DATABASE `+pgx.Identifier{database}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec(ctx, `DROP DATABASE `+pgx.Identifier{database}.Sanitize()+` WITH (FORCE)`); err != nil {
			t.Error(err)
		}
	}()
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.Database = database
	config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`CREATE TABLE courses(id uuid PRIMARY KEY);
		CREATE TABLE modules(id uuid PRIMARY KEY, course_id uuid NOT NULL REFERENCES courses(id), slug text NOT NULL, title text NOT NULL, description text,
		order_index smallint NOT NULL, is_mandatory boolean NOT NULL DEFAULT true, is_entry boolean NOT NULL DEFAULT false, estimated_minutes integer,
		is_published boolean NOT NULL DEFAULT false, archived_at timestamptz, updated_at timestamptz DEFAULT now(),
		UNIQUE(course_id,order_index) DEFERRABLE INITIALLY DEFERRED);
		CREATE TABLE lessons(id uuid PRIMARY KEY, module_id uuid NOT NULL REFERENCES modules(id), slug text NOT NULL, title text NOT NULL,
		content jsonb, order_index smallint NOT NULL, is_entry boolean NOT NULL DEFAULT false, is_published boolean NOT NULL DEFAULT false,
		archived_at timestamptz, updated_at timestamptz DEFAULT now());
		CREATE TABLE admin_drafts(resource_type text NOT NULL, resource_id uuid NOT NULL, payload jsonb NOT NULL, updated_at timestamptz DEFAULT now(), PRIMARY KEY(resource_type,resource_id));`)
	h := &Handler{service: NewService(db.NewTx(pool), NewRepository(pool))}
	app := fiber.New()
	app.Post("/content/:kind/reorder", h.reorder)
	app.Post("/content/:kind/:id/publish", h.publish)
	post := func(path string, payload any, status int) {
		t.Helper()
		raw, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		response, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != status {
			t.Fatalf("%s: status %d, want %d", path, response.StatusCode, status)
		}
	}
	for _, kind := range []string{"module", "lesson"} {
		t.Run(kind, func(t *testing.T) {
			courseID, otherCourse := uuid.New(), uuid.New()
			exec(`INSERT INTO courses(id) VALUES($1),($2)`, courseID, otherCourse)
			parent, otherParent := courseID, otherCourse
			parentField := "course_id"
			if kind == "lesson" {
				parent, otherParent = uuid.New(), uuid.New()
				parentField = "module_id"
				exec(`INSERT INTO modules(id,course_id,slug,title,order_index) VALUES($1,$2,'parent','Parent',0),($3,$4,'other','Other',0)`, parent, courseID, otherParent, otherCourse)
			}
			ids := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
			foreign := uuid.New()
			for i, id := range append(append([]uuid.UUID{}, ids...), foreign) {
				owner, order := parent, i
				if i == 3 {
					owner, order = otherParent, 0
				}
				exec(fmt.Sprintf(`INSERT INTO %ss(id,%s,slug,title,order_index,is_published,archived_at) VALUES($1,$2,$3,$3,$4,$5,CASE WHEN $6 THEN now() ELSE NULL END)`, kind, parentField), id, owner, fmt.Sprintf("item-%d", i), order, i == 0, i == 2)
			}
			draft := map[string]any{parentField: parent, "slug": "draft-title", "title": "Pending material", "order_index": 0, "is_mandatory": true, "content": map[string]string{"content": "<p>Pending lesson.</p>", "tips": ""}}
			raw, _ := json.Marshal(draft)
			exec(`INSERT INTO admin_drafts(resource_type,resource_id,payload) VALUES($1,$2,$3::jsonb)`, kind, ids[0], string(raw))
			movedDraft := fmt.Sprintf(`{"%s":%q,"title":"Move later","order_index":19}`, parentField, otherParent)
			exec(`INSERT INTO admin_drafts(resource_type,resource_id,payload) VALUES($1,$2,$3::jsonb)`, kind, ids[1], movedDraft)
			path := "/content/" + kind + "/reorder"
			post(path, ReorderRequest{ParentID: parent, OrderedIDs: []uuid.UUID{ids[0], ids[1], foreign}}, 409)
			post(path, ReorderRequest{ParentID: parent, OrderedIDs: ids[:2]}, 409)
			post(path, ReorderRequest{ParentID: uuid.New(), OrderedIDs: ids}, 404)
			assertOrder := func(expected []uuid.UUID) {
				t.Helper()
				rows, err := pool.Query(ctx, fmt.Sprintf(`SELECT id FROM %ss WHERE %s=$1 ORDER BY order_index`, kind, parentField), parent)
				if err != nil {
					t.Fatal(err)
				}
				actual, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
				if err != nil || fmt.Sprint(actual) != fmt.Sprint(expected) {
					t.Fatalf("order %v, want %v: %v", actual, expected, err)
				}
			}
			// Fail after live rows have updated: both the live and draft writes must roll back.
			exec(`CREATE FUNCTION fail_reorder_test() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected failure'; END $$;
				CREATE TRIGGER fail_reorder BEFORE UPDATE ON admin_drafts FOR EACH ROW EXECUTE FUNCTION fail_reorder_test();`)
			reversed := []uuid.UUID{ids[2], ids[1], ids[0]}
			post(path, ReorderRequest{ParentID: parent, OrderedIDs: reversed}, 500)
			assertOrder(ids)
			exec(`DROP TRIGGER fail_reorder ON admin_drafts; DROP FUNCTION fail_reorder_test();`)
			post(path, ReorderRequest{ParentID: parent, OrderedIDs: reversed}, 200)
			assertOrder(reversed)
			var updated map[string]any
			if err := pool.QueryRow(ctx, `SELECT payload FROM admin_drafts WHERE resource_type=$1 AND resource_id=$2`, kind, ids[0]).Scan(&updated); err != nil {
				t.Fatal(err)
			}
			if updated["title"] != "Pending material" || updated["order_index"] != float64(2) {
				t.Fatalf("draft changed unexpectedly: %v", updated)
			}
			var destinationOrder int
			if err := pool.QueryRow(ctx, `SELECT (payload->>'order_index')::int FROM admin_drafts WHERE resource_type=$1 AND resource_id=$2`, kind, ids[1]).Scan(&destinationOrder); err != nil {
				t.Fatal(err)
			}
			if destinationOrder != 19 {
				t.Fatal("changed a draft destined for another parent")
			}
			post("/content/"+kind+"/"+ids[0].String()+"/publish", nil, 200)
			assertOrder(reversed)
			var title string
			if err := pool.QueryRow(ctx, fmt.Sprintf(`SELECT title FROM %ss WHERE id=$1`, kind), ids[0]).Scan(&title); err != nil || title != "Pending material" {
				t.Fatalf("publish lost draft: %s, %v", title, err)
			}
		})
	}
}

func validatedReorderInput(kind string, raw []byte) (ReorderRequest, error) {
	var input ReorderRequest
	if err := json.Unmarshal(raw, &input); err != nil {
		return input, err
	}
	return input, input.Validate(kind)
}
