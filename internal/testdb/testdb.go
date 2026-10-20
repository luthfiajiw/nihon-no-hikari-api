// Package testdb creates disposable databases for integration tests. It never
// installs tables in the database named by KANA_TEMPLATE_TEST_DATABASE_URL.
package testdb

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func New(t *testing.T, through int) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("KANA_TEMPLATE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set KANA_TEMPLATE_TEST_DATABASE_URL to an isolated PostgreSQL instance")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := "nnh_kana_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, `CREATE DATABASE `+pgx.Identifier{name}.Sanitize()); err != nil {
		admin.Close(ctx)
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, `DROP DATABASE `+pgx.Identifier{name}.Sanitize()+` WITH (FORCE)`); err != nil {
			t.Error(err)
		}
		admin.Close(ctx)
	})
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.Database = name
	config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	for version := 1; version <= through; version++ {
		Migrate(t, pool, version, "up")
	}
	return pool
}

func Migrate(t *testing.T, pool *pgxpool.Pool, version int, direction string) {
	t.Helper()
	_, source, _, _ := runtime.Caller(0)
	pattern := filepath.Join(filepath.Dir(source), "../../pkg/db/migrations", fmt.Sprintf("%06d_*.%s.sql", version, direction))
	paths, err := filepath.Glob(pattern)
	if err != nil || len(paths) != 1 {
		t.Fatalf("migration %d: %v %v", version, paths, err)
	}
	raw, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), string(raw)); err != nil {
		t.Fatalf("%s: %v", filepath.Base(paths[0]), err)
	}
}

// Fixture creates one live writing question using the pinned local KanjiVG data.
func Fixture(t *testing.T, pool *pgxpool.Pool) (user, set, question, template uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	user, set, question, template = uuid.New(), uuid.New(), uuid.New(), uuid.New()
	level, course, module, lesson := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec(`INSERT INTO users(id,email,display_name,role) VALUES($1,'test@example.test','Test','Admin')`, user)
	exec(`INSERT INTO levels(id,code,name,order_index) VALUES($1,'N5','N5',0)`, level)
	exec(`INSERT INTO courses(id,level_id,slug,title,is_published) VALUES($1,$2,'test','Test',true)`, course, level)
	exec(`INSERT INTO modules(id,course_id,slug,title,order_index,is_entry,is_published) VALUES($1,$2,'test','Test',0,true,true)`, module, course)
	exec(`INSERT INTO lessons(id,module_id,slug,title,order_index,is_entry,is_published,content) VALUES($1,$2,'test','Test',0,true,true,'{}')`, lesson, module)
	_, source, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(source), "../../pkg/db/stroke_sources/hiragana_a_guides.json"))
	if err != nil {
		t.Fatal(err)
	}
	// Extract only the first guide's evaluator data with PostgreSQL JSON operators.
	exec(`INSERT INTO kana_stroke_templates(id,script,character,romaji,source_version,stroke_data)
		VALUES($1,'hiragana','あ','a','r20260714',($2::jsonb->0)-'code'-'character'-'romaji')`, template, string(raw))
	exec(`INSERT INTO question_sets(id,lineage_id,lesson_id,title,kind,skill,passing_score,shuffle_questions,is_published,order_index)
		VALUES($1,$1,$2,'Test writing','practice','writing',100,false,false,0)`, set, lesson)
	exec(`INSERT INTO questions(id,question_set_id,question_type,skill,prompt_text,stroke_template_id,points,order_index)
		VALUES($1,$2,'stroke_writing','writing','Tulis a',$3,1,0)`, question, set, template)
	exec(`UPDATE question_sets SET is_published=true WHERE id=$1`, set)
	return
}
