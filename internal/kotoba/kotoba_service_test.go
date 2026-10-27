package kotoba

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type memoryStorage struct {
	enabled                       bool
	uploadErr, deleteErr, signErr error
	uploads, deletes              []string
	signCalls                     int
}

func (s *memoryStorage) Enabled() bool { return s.enabled }
func (s *memoryStorage) Upload(ctx context.Context, bucket, path, mime string, r io.Reader) error {
	s.uploads = append(s.uploads, bucket+"/"+path)
	_, _ = io.Copy(io.Discard, r)
	return s.uploadErr
}
func (s *memoryStorage) Delete(ctx context.Context, bucket, path string) error {
	s.deletes = append(s.deletes, bucket+"/"+path)
	return s.deleteErr
}
func (s *memoryStorage) SignedURLs(ctx context.Context, bucket string, paths []string, ttl time.Duration) (map[string]string, error) {
	s.signCalls++
	if s.signErr != nil {
		return nil, s.signErr
	}
	urls := map[string]string{}
	for _, path := range paths {
		urls[path] = "https://storage.test/object/sign/" + bucket + "/" + path + "?token=temporary"
	}
	return urls, nil
}

type transactionStub struct{ err error }

func (t transactionStub) Exec(ctx context.Context, fn func(pgx.Tx) error) error {
	if t.err != nil {
		return t.err
	}
	return fn(nil)
}

type repositoryStub struct {
	Repository
	inserted            []Media
	ready               []uuid.UUID
	insertErr, readyErr error
	claims              []Media
	completed           []uuid.UUID
	retries             []time.Time
	version             int64
	lockErr             error
	medias              map[uuid.UUID]Media
	knownWords          map[uuid.UUID]bool
	saved               bool
	topic               Topic
	getErr              error
	scheduled           []uuid.UUID
}

func (r *repositoryStub) UseTx(pgx.Tx) Repository { return r }
func (r *repositoryStub) InsertMedia(ctx context.Context, m Media) error {
	if r.insertErr != nil {
		return r.insertErr
	}
	r.inserted = append(r.inserted, m)
	return nil
}
func (r *repositoryStub) ReadyMedia(ctx context.Context, id uuid.UUID) error {
	r.ready = append(r.ready, id)
	return r.readyErr
}
func (r *repositoryStub) ClaimCleanup(context.Context, time.Time) (Media, error) {
	if len(r.claims) == 0 {
		return Media{}, ErrNotFound
	}
	m := r.claims[0]
	r.claims = r.claims[1:]
	return m, nil
}
func (r *repositoryStub) CompleteCleanup(ctx context.Context, id uuid.UUID) error {
	r.completed = append(r.completed, id)
	return nil
}
func (r *repositoryStub) RetryCleanup(ctx context.Context, id uuid.UUID, at time.Time) error {
	r.retries = append(r.retries, at)
	return nil
}
func (r *repositoryStub) LockTopic(context.Context, uuid.UUID) (int64, error) {
	return r.version, r.lockErr
}
func (r *repositoryStub) TopicMedia(context.Context, uuid.UUID) ([]uuid.UUID, error) { return nil, nil }
func (r *repositoryStub) VocabularyIDs(context.Context, uuid.UUID) (map[uuid.UUID]bool, error) {
	return r.knownWords, nil
}
func (r *repositoryStub) LockMedia(ctx context.Context, id uuid.UUID) (Media, error) {
	m, ok := r.medias[id]
	if !ok {
		return m, ErrNotFound
	}
	return m, nil
}
func (r *repositoryStub) Save(context.Context, uuid.UUID, TopicInput, bool) error {
	r.saved = true
	return nil
}
func (r *repositoryStub) ScheduleUnused(ctx context.Context, ids []uuid.UUID) error {
	r.scheduled = ids
	return nil
}
func (r *repositoryStub) Get(context.Context, uuid.UUID, bool) (Topic, error) {
	return r.topic, r.getErr
}

func TestUploadLifecycle(t *testing.T) {
	failure := errors.New("injected failure")
	for _, tc := range []struct {
		name                                 string
		enabled                              bool
		insertErr, uploadErr, readyErr, want error
	}{
		{name: "success", enabled: true},
		{name: "disabled", want: ErrUnavailable},
		{name: "db before upload", enabled: true, insertErr: failure, want: failure},
		{name: "upload failure", enabled: true, uploadErr: failure, want: ErrStorage},
		{name: "db after upload", enabled: true, readyErr: failure, want: failure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &repositoryStub{insertErr: tc.insertErr, readyErr: tc.readyErr}
			storage := &memoryStorage{enabled: tc.enabled, uploadErr: tc.uploadErr}
			svc := NewService(transactionStub{}, repo, storage, "covers", "audio")
			m, err := svc.Upload(context.Background(), uuid.New(), "cover", "x.png", "image/png", bytes.NewReader(pngFixture(t)))
			if !errors.Is(err, tc.want) {
				t.Fatalf("want %v, got %v", tc.want, err)
			}
			if tc.want == nil && (m.URL == "" || m.Status != "ready" || len(repo.ready) != 1) {
				t.Fatalf("incomplete upload: %+v", m)
			}
			if (tc.insertErr != nil || !tc.enabled) && len(storage.uploads) != 0 {
				t.Fatal("uploaded before durable metadata")
			}
			if tc.uploadErr != nil && (len(repo.inserted) != 1 || len(repo.ready) != 0 || len(storage.deletes) != 0) {
				t.Fatal("ambiguous upload must remain pending for later cleanup")
			}
		})
	}
}
func TestSaveRejectsStaleAndInvalidReferences(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*repositoryStub, *TopicInput)
		want  error
		field string
	}{
		{name: "stale version", setup: func(r *repositoryStub, in *TopicInput) { r.version = 2 }, want: ErrConflict},
		{name: "foreign word", setup: func(r *repositoryStub, in *TopicInput) { id := uuid.New(); in.Vocabulary[0].ID = &id }, field: "vocabulary.0.id"},
		{name: "missing cover", setup: func(r *repositoryStub, in *TopicInput) { r.medias = nil }, field: "cover_media_id"},
		{name: "pending cover", setup: func(r *repositoryStub, in *TopicInput) {
			r.medias[in.CoverMediaID] = Media{Kind: "cover", Status: "uploading"}
		}, field: "cover_media_id"},
		{name: "claimed cover", setup: func(r *repositoryStub, in *TopicInput) {
			r.medias[in.CoverMediaID] = Media{Kind: "cover", Status: "deleting"}
		}, field: "cover_media_id"},
		{name: "wrong audio type", setup: func(r *repositoryStub, in *TopicInput) { in.Vocabulary[0].AudioMediaID = &in.CoverMediaID }, field: "vocabulary.0.audio_media_id"},
		{name: "success", setup: func(r *repositoryStub, in *TopicInput) {}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := validInput()
			in.Version = 1
			repo := &repositoryStub{version: 1, medias: map[uuid.UUID]Media{in.CoverMediaID: {Kind: "cover", Status: "ready"}}}
			tc.setup(repo, &in)
			svc := NewService(transactionStub{}, repo, &memoryStorage{}, "covers", "audio")
			_, err := svc.Save(context.Background(), uuid.New(), in)
			if tc.field != "" {
				var fields FieldErrors
				if !errors.As(err, &fields) || fields[tc.field] == "" {
					t.Fatalf("want %s: %v", tc.field, err)
				}
			} else if !errors.Is(err, tc.want) {
				t.Fatalf("want %v, got %v", tc.want, err)
			}
			if (tc.field != "" || tc.want != nil) && repo.saved {
				t.Fatal("invalid input reached write")
			}
		})
	}
}
func TestCleanupRetryAndRestart(t *testing.T) {
	now := time.Now()
	id := uuid.New()
	repo := &repositoryStub{claims: []Media{{ID: id, Bucket: "audio", ObjectPath: "id.mp3", Attempts: 1}}}
	storage := &memoryStorage{enabled: true, deleteErr: errors.New("temporary failure")}
	svc := NewService(transactionStub{}, repo, storage, "covers", "audio")
	if err := svc.Cleanup(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if len(repo.completed) != 0 || len(repo.retries) != 1 || repo.retries[0].Sub(now.Add(CleanupInterval)) < 0 || repo.retries[0].Sub(now.Add(CleanupInterval)) > time.Second {
		t.Fatal("failed deletion was not retained for retry")
	}
	// A fresh service instance recovers the persisted claim after its lease.
	repo.claims = []Media{{ID: id, Bucket: "audio", ObjectPath: "id.mp3", Attempts: 2}}
	storage.deleteErr = nil
	if err := NewService(transactionStub{}, repo, storage, "covers", "audio").Cleanup(context.Background(), now.Add(CleanupInterval)); err != nil {
		t.Fatal(err)
	}
	if len(repo.completed) != 1 || repo.completed[0] != id {
		t.Fatal("restart did not complete cleanup")
	}
}
func TestStudentProjection(t *testing.T) {
	id := uuid.New()
	active, visible, version := true, true, int64(1)
	topic := Topic{CoverMediaID: &id, IsActive: &active, IsVisible: &visible, Version: &version, CoverBucket: "covers", CoverPath: "id.png", Vocabulary: []Vocabulary{{AudioMediaID: &id, AudioBucket: "audio", AudioPath: "id.mp3"}}}
	svc := NewService(nil, nil, &memoryStorage{enabled: true}, "covers", "audio")
	topics := []Topic{topic}
	if err := svc.present(context.Background(), topics, false); err != nil {
		t.Fatal(err)
	}
	topic = topics[0]
	if topic.CoverMediaID != nil || topic.IsActive != nil || topic.IsVisible != nil || topic.Version != nil || topic.Vocabulary[0].AudioMediaID != nil {
		t.Fatal("admin metadata leaked to student")
	}
	if topic.CoverURL == "" || topic.Vocabulary[0].AudioURL == nil {
		t.Fatal("missing playback URLs")
	}
	if topic.MediaURLsExpiresAt == nil || time.Until(*topic.MediaURLsExpiresAt) > MediaURLTTL {
		t.Fatal("missing signed URL expiry")
	}
}

func TestSigningFailureDoesNotUndoCommittedMutation(t *testing.T) {
	storage := &memoryStorage{enabled: true, signErr: errors.New("signing unavailable")}
	repo := &repositoryStub{}
	svc := NewService(transactionStub{}, repo, storage, "covers", "audio")
	media, err := svc.Upload(context.Background(), uuid.New(), "cover", "cover.png", "image/png", bytes.NewReader(pngFixture(t)))
	if err != nil || media.ID == uuid.Nil || media.Status != "ready" || media.URL != "" || media.URLExpiresAt != nil {
		t.Fatal("uploaded media must retain its successful ID when signing fails")
	}
	in := validInput()
	version := int64(1)
	repo.medias = map[uuid.UUID]Media{in.CoverMediaID: {Kind: "cover", Status: "ready"}}
	repo.topic = Topic{ID: uuid.New(), Version: &version, CoverMediaID: &in.CoverMediaID, CoverBucket: "covers", CoverPath: "saved.png"}
	saved, err := svc.Save(context.Background(), uuid.Nil, in)
	if err != nil || !repo.saved || saved.Version == nil || *saved.Version != 1 || saved.CoverURL != "" || saved.MediaURLsExpiresAt != nil {
		t.Fatal("a committed save must not be reported as failed when signing fails")
	}
	topics := []Topic{{CoverBucket: "covers", CoverPath: "id.png"}}
	if err := svc.present(context.Background(), topics, false); !errors.Is(err, ErrStorage) {
		t.Fatal("read should report signing failure")
	}
	storage.enabled = false
	if err := svc.present(context.Background(), topics, false); !errors.Is(err, ErrUnavailable) {
		t.Fatal("private reads need signing credentials")
	}
}

func TestUnavailableTopicDoesNotSignMedia(t *testing.T) {
	storage := &memoryStorage{enabled: true}
	repo := &repositoryStub{getErr: ErrNotFound}
	svc := NewService(nil, repo, storage, "covers", "audio")
	if _, err := svc.Detail(context.Background(), uuid.New(), false); !errors.Is(err, ErrNotFound) || storage.signCalls != 0 {
		t.Fatal("access must be checked before signing")
	}
}

func TestSigningUsesBucketBatches(t *testing.T) {
	storage := &memoryStorage{enabled: true}
	svc := NewService(nil, nil, storage, "covers", "audio")
	topics := []Topic{
		{CoverBucket: "covers", CoverPath: "first.png", Vocabulary: []Vocabulary{{AudioBucket: "audio", AudioPath: "shared.mp3"}, {AudioBucket: "audio", AudioPath: "shared.mp3"}}},
		{CoverBucket: "covers", CoverPath: "second.png"},
	}
	if err := svc.present(context.Background(), topics, true); err != nil {
		t.Fatal(err)
	}
	if storage.signCalls != 2 {
		t.Fatal("URLs must be signed by bucket, not one request per object")
	}
	if *topics[0].Vocabulary[0].AudioURL != *topics[0].Vocabulary[1].AudioURL {
		t.Fatal("shared audio should share its signed URL")
	}
}
