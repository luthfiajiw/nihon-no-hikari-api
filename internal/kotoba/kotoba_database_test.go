package kotoba

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"nihon-no-hikari-api/internal/testdb"
	"nihon-no-hikari-api/pkg/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func inputFromTopic(t Topic) TopicInput {
	in := TopicInput{Title: t.Title, Level: t.Level, Category: t.Category, Description: t.Description, OrderIndex: t.OrderIndex, IsActive: *t.IsActive, IsVisible: *t.IsVisible, CoverMediaID: *t.CoverMediaID, Version: *t.Version}
	for _, w := range t.Vocabulary {
		id := w.ID
		in.Vocabulary = append(in.Vocabulary, VocabularyInput{ID: &id, WordFields: w.WordFields, AudioMediaID: w.AudioMediaID})
	}
	return in
}
func execSQL(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatal(err)
	}
}
func countSQL(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestKotobaMigrationDatabase(t *testing.T) {
	pool := testdb.New(t, 13)
	execSQL(t, pool, `INSERT INTO levels(code,name,order_index) VALUES('N5','Existing N5',0),('CUSTOM','Existing level',1)`)
	testdb.Migrate(t, pool, 14, "up")
	if n := countSQL(t, pool, `SELECT count(*) FROM levels WHERE code IN ('N5','N4','N3','N2','N1')`); n != 5 {
		t.Fatalf("missing JLPT seeds: %d", n)
	}
	if n := countSQL(t, pool, `SELECT count(*) FROM levels WHERE code='N5' AND name='Existing N5' AND order_index=0`); n != 1 {
		t.Fatal("seed changed existing shared level")
	}
	testdb.Migrate(t, pool, 14, "down")
	if n := countSQL(t, pool, `SELECT count(*) FROM information_schema.tables WHERE table_name IN ('kotoba_topics','kotoba_vocabularies','kotoba_media')`); n != 0 {
		t.Fatal("down migration left tables")
	}
	testdb.Migrate(t, pool, 14, "up")
	if n := countSQL(t, pool, `SELECT count(*) FROM levels`); n != 6 {
		t.Fatal("reapplying seed duplicated levels")
	}
}

func TestKotobaDatabase(t *testing.T) {
	pool := testdb.New(t, 14)
	ctx := context.Background()
	user := uuid.New()
	execSQL(t, pool, `INSERT INTO users(id,email,display_name,role) VALUES($1,'kotoba@example.test','Kotoba Admin','Admin')`, user)
	repo := NewRepository(pool)
	storage := &memoryStorage{enabled: true}
	svc := NewService(db.NewTx(pool), repo, storage, "covers", "audio")
	upload := func(kind string) Media {
		t.Helper()
		name, mime, data := "cover.png", "image/png", pngFixture(t)
		if kind == "audio" {
			name, mime, data = "word.mp3", "audio/mpeg", audioFixture(t)
		}
		m, err := svc.Upload(ctx, user, kind, name, mime, bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	cover, audio := upload("cover"), upload("audio")
	in := validInput()
	in.CoverMediaID = cover.ID
	in.Vocabulary[0].AudioMediaID = &audio.ID
	topic, err := svc.Save(ctx, uuid.Nil, in)
	if err != nil {
		t.Fatal(err)
	}
	if *topic.Version != 1 || topic.VocabularyCount != 1 || topic.Vocabulary[0].AudioURL == nil || topic.CoverURL == "" {
		t.Fatalf("bad created topic: %+v", topic)
	}
	originalWord := topic.Vocabulary[0].ID

	t.Run("duplicate reorder preserve IDs and remove audio", func(t *testing.T) {
		in := inputFromTopic(topic)
		duplicate := in.Vocabulary[0]
		duplicate.ID = nil
		duplicate.Kanji = "教室"
		duplicate.Hiragana = "きょうしつ"
		duplicate.Furigana = nil
		in.Vocabulary = append([]VocabularyInput{duplicate}, in.Vocabulary...)
		in.Vocabulary[1].AudioMediaID = nil
		updated, err := svc.Save(ctx, topic.ID, in)
		if err != nil {
			t.Fatal(err)
		}
		if updated.VocabularyCount != 2 || updated.Vocabulary[1].ID != originalWord || updated.Vocabulary[0].ID == originalWord || updated.Vocabulary[0].OrderIndex != 1 || updated.Vocabulary[1].AudioURL != nil {
			t.Fatal("word identity, order, or null audio not preserved")
		}
		topic = updated
	})
	t.Run("invalid reference and foreign ID roll back", func(t *testing.T) {
		in := inputFromTopic(topic)
		in.Title = "Must not persist"
		badID := uuid.New()
		in.Vocabulary[0].AudioMediaID = &badID
		if _, err := svc.Save(ctx, topic.ID, in); err == nil {
			t.Fatal("unknown media accepted")
		}
		actual, err := svc.Detail(ctx, topic.ID, true)
		if err != nil || actual.Title != topic.Title || *actual.Version != *topic.Version {
			t.Fatal("failed save was not atomic")
		}
		in = inputFromTopic(topic)
		in.Vocabulary[0].ID = &badID
		if _, err := svc.Save(ctx, topic.ID, in); err == nil {
			t.Fatal("foreign vocabulary ID accepted")
		}
		in = inputFromTopic(topic)
		in.CoverMediaID = audio.ID
		if _, err := svc.Save(ctx, topic.ID, in); err == nil {
			t.Fatal("audio accepted as cover")
		}
		pending := upload("cover")
		execSQL(t, pool, `UPDATE kotoba_media SET status='uploading' WHERE id=$1`, pending.ID)
		in = inputFromTopic(topic)
		in.CoverMediaID = pending.ID
		if _, err := svc.Save(ctx, topic.ID, in); err == nil {
			t.Fatal("pending cover accepted")
		}
	})
	t.Run("concurrent version conflict", func(t *testing.T) {
		start := make(chan struct{})
		results := make(chan error, 2)
		for i := range 2 {
			in := inputFromTopic(topic)
			if i == 0 {
				in.Title = "Concurrent A"
			} else {
				in.Title = "Concurrent B"
			}
			go func(in TopicInput) { <-start; _, err := svc.Save(ctx, topic.ID, in); results <- err }(in)
		}
		close(start)
		success, conflict := 0, 0
		for range 2 {
			err := <-results
			if err == nil {
				success++
			} else if errors.Is(err, ErrConflict) {
				conflict++
			} else {
				t.Fatal(err)
			}
		}
		if success != 1 || conflict != 1 {
			t.Fatalf("success=%d conflict=%d", success, conflict)
		}
		topic, err = svc.Detail(ctx, topic.ID, true)
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("database failure rolls back the entire form", func(t *testing.T) {
		execSQL(t, pool, `CREATE FUNCTION kotoba_test_fail_word() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN IF NEW.romaji='fail-write' THEN RAISE EXCEPTION 'injected write failure'; END IF; RETURN NEW; END $$;
		CREATE TRIGGER kotoba_test_fail_word BEFORE INSERT ON kotoba_vocabularies FOR EACH ROW EXECUTE FUNCTION kotoba_test_fail_word()`)
		t.Cleanup(func() {
			execSQL(t, pool, `DROP TRIGGER kotoba_test_fail_word ON kotoba_vocabularies; DROP FUNCTION kotoba_test_fail_word()`)
		})
		in := inputFromTopic(topic)
		in.Title = "Must roll back"
		newWord := in.Vocabulary[0]
		newWord.ID = nil
		newWord.Romaji = "fail-write"
		in.Vocabulary = append(in.Vocabulary, newWord)
		if _, err := svc.Save(ctx, topic.ID, in); err == nil {
			t.Fatal("injected database failure was ignored")
		}
		actual, err := svc.Detail(ctx, topic.ID, true)
		if err != nil || actual.Title != topic.Title || *actual.Version != *topic.Version || actual.VocabularyCount != topic.VocabularyCount || actual.Vocabulary[0].ID != topic.Vocabulary[0].ID {
			t.Fatal("partial form persisted despite transaction rollback")
		}
	})
	t.Run("student visibility combinations and filters", func(t *testing.T) {
		for _, active := range []bool{false, true} {
			for _, visible := range []bool{false, true} {
				in := inputFromTopic(topic)
				in.IsActive, in.IsVisible = active, visible
				topic, err = svc.Save(ctx, topic.ID, in)
				if err != nil {
					t.Fatal(err)
				}
				list, err := svc.List(ctx, Filter{Level: "N5"}, false)
				if err != nil {
					t.Fatal(err)
				}
				want := 0
				if active && visible {
					want = 1
				}
				if len(list) != want {
					t.Fatalf("active=%v visible=%v: %d", active, visible, len(list))
				}
				detail, err := svc.Detail(ctx, topic.ID, false)
				if !active && !errors.Is(err, ErrNotFound) {
					t.Fatal("inactive topic accessible")
				}
				if active && (err != nil || detail.Version != nil || detail.CoverMediaID != nil || detail.Vocabulary[0].AudioMediaID != nil) {
					t.Fatal("student projection/access incorrect")
				}
			}
		}
		for _, sort := range []string{"order", "title", "newest", "vocabulary"} {
			if _, err := svc.List(ctx, Filter{Sort: sort}, false); err != nil {
				t.Fatal(err)
			}
		}
		list, err := svc.List(ctx, Filter{Search: "Concurrent", Level: "N5", Status: "active"}, true)
		if err != nil || len(list) != 1 {
			t.Fatalf("admin filters: %v %d", err, len(list))
		}
		list, err = svc.List(ctx, Filter{Search: "does not exist"}, true)
		if err != nil || len(list) != 0 {
			t.Fatal("search filtering failed")
		}
	})
	t.Run("shared media retained after replacement and deletion", func(t *testing.T) {
		secondInput := validInput()
		secondInput.CoverMediaID = cover.ID
		secondInput.Vocabulary[0].AudioMediaID = &audio.ID
		second, err := svc.Save(ctx, uuid.Nil, secondInput)
		if err != nil {
			t.Fatal(err)
		}
		replacement := upload("cover")
		in := inputFromTopic(topic)
		in.CoverMediaID = replacement.ID
		in.Vocabulary = in.Vocabulary[:1]
		topic, err = svc.Save(ctx, topic.ID, in)
		if err != nil {
			t.Fatal(err)
		}
		if topic.VocabularyCount != 1 {
			t.Fatal("removed word persisted")
		}
		if err := svc.Delete(ctx, topic.ID); err != nil {
			t.Fatal(err)
		}
		if n := countSQL(t, pool, `SELECT count(*) FROM kotoba_vocabularies WHERE topic_id=$1`, topic.ID); n != 0 {
			t.Fatal("word cascade failed")
		}
		if n := countSQL(t, pool, `SELECT count(*) FROM kotoba_media WHERE id=ANY($1::uuid[]) AND cleanup_after IS NULL`, uuidStrings([]uuid.UUID{cover.ID, audio.ID})); n != 2 {
			t.Fatal("shared media incorrectly scheduled")
		}
		execSQL(t, pool, `UPDATE kotoba_media SET cleanup_after=now()-interval '1 second' WHERE cleanup_after IS NOT NULL`)
		if err := svc.Cleanup(ctx, time.Now().Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		if n := countSQL(t, pool, `SELECT count(*) FROM kotoba_media WHERE id=ANY($1::uuid[])`, uuidStrings([]uuid.UUID{cover.ID, audio.ID})); n != 2 {
			t.Fatal("shared files deleted")
		}
		if err := svc.Delete(ctx, second.ID); err != nil {
			t.Fatal(err)
		}
		if err := svc.Delete(ctx, second.ID); !errors.Is(err, ErrNotFound) {
			t.Fatal("repeat delete must return 404")
		}
	})
	t.Run("claim races reject reattachment and survive restart", func(t *testing.T) {
		execSQL(t, pool, `UPDATE kotoba_media SET cleanup_after=now()-interval '1 second'`)
		claim, err := repo.ClaimCleanup(ctx, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if claim.Status != "deleting" {
			t.Fatal("claim not persisted")
		}
		if _, err := repo.ClaimCleanup(ctx, time.Now()); err != nil && !errors.Is(err, ErrNotFound) {
			t.Fatal(err)
		}
		in := validInput()
		in.CoverMediaID = claim.ID
		if _, err := svc.Save(ctx, uuid.Nil, in); err == nil {
			t.Fatal("claimed media reattached")
		}
		if err := repo.RetryCleanup(ctx, claim.ID, time.Now().Add(-time.Second)); err != nil {
			t.Fatal(err)
		}
		if err := NewService(db.NewTx(pool), NewRepository(pool), storage, "covers", "audio").Cleanup(ctx, time.Now()); err != nil {
			t.Fatal(err)
		}
		if n := countSQL(t, pool, `SELECT count(*) FROM kotoba_media WHERE id=$1`, claim.ID); n != 0 {
			t.Fatal("restart did not resume claim")
		}
	})
	t.Run("simultaneous cleanup claims are exclusive", func(t *testing.T) {
		m := upload("cover")
		execSQL(t, pool, `UPDATE kotoba_media SET cleanup_after=now()-interval '1 second' WHERE id=$1`, m.ID)
		// Remove leftover leases so only the new object is eligible.
		execSQL(t, pool, `UPDATE kotoba_media SET retry_after=now()+interval '1 hour' WHERE id<>$1`, m.ID)
		start := make(chan struct{})
		ids := make(chan uuid.UUID, 2)
		errs := make(chan error, 2)
		var wg sync.WaitGroup
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				claim, err := repo.ClaimCleanup(ctx, time.Now())
				ids <- claim.ID
				errs <- err
			}()
		}
		close(start)
		wg.Wait()
		claimed := 0
		for range 2 {
			if id := <-ids; id == m.ID {
				claimed++
			}
			if err := <-errs; err != nil && !errors.Is(err, ErrNotFound) {
				t.Fatal(err)
			}
		}
		if claimed != 1 {
			t.Fatalf("media claimed %d times", claimed)
		}
	})
	t.Run("remote deletion failure persists retry state", func(t *testing.T) {
		m := upload("cover")
		execSQL(t, pool, `UPDATE kotoba_media SET cleanup_after=now()-interval '1 second' WHERE id=$1`, m.ID)
		execSQL(t, pool, `UPDATE kotoba_media SET retry_after=now()+interval '1 hour' WHERE id<>$1`, m.ID)
		storage.deleteErr = errors.New("temporary storage outage")
		if err := svc.Cleanup(ctx, time.Now()); err != nil {
			t.Fatal(err)
		}
		if n := countSQL(t, pool, `SELECT count(*) FROM kotoba_media WHERE id=$1 AND status='deleting' AND attempts=1 AND retry_after>now()`, m.ID); n != 1 {
			t.Fatal("retry state was not durable")
		}
		storage.deleteErr = nil
		if err := NewService(db.NewTx(pool), NewRepository(pool), storage, "covers", "audio").Cleanup(ctx, time.Now().Add(10*time.Minute)); err != nil {
			t.Fatal(err)
		}
		if n := countSQL(t, pool, `SELECT count(*) FROM kotoba_media WHERE id=$1`, m.ID); n != 0 {
			t.Fatal("retry did not remove recovered object")
		}
	})
}
