package kotoba

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"time"

	"nihon-no-hikari-api/pkg/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Storage interface {
	Enabled() bool
	Upload(context.Context, string, string, string, io.Reader) error
	Delete(context.Context, string, string) error
	SignedURLs(context.Context, string, []string, time.Duration) (map[string]string, error)
}
type Operations interface {
	List(context.Context, Filter, bool) ([]Topic, error)
	Detail(context.Context, uuid.UUID, bool) (Topic, error)
	Save(context.Context, uuid.UUID, TopicInput) (Topic, error)
	Delete(context.Context, uuid.UUID) error
	Upload(context.Context, uuid.UUID, string, string, string, io.Reader) (Media, error)
}
type Service struct {
	tx                       db.WithTx
	repo                     Repository
	storage                  Storage
	coverBucket, audioBucket string
}

func NewService(tx db.WithTx, repo Repository, storage Storage, coverBucket, audioBucket string) *Service {
	return &Service{tx: tx, repo: repo, storage: storage, coverBucket: coverBucket, audioBucket: audioBucket}
}

const MediaURLTTL = 15 * time.Minute

// Repository projections have already enforced topic access before signing.
// Gather all objects by bucket to avoid one remote request per vocabulary item.
func (s *Service) present(ctx context.Context, topics []Topic, admin bool) error {
	groups := map[string][]string{}
	for i := range topics {
		t := &topics[i]
		t.CoverURL, t.MediaURLsExpiresAt = "", nil
		if t.CoverBucket != "" && t.CoverPath != "" {
			groups[t.CoverBucket] = append(groups[t.CoverBucket], t.CoverPath)
		}
		if !admin {
			t.IsActive, t.IsVisible, t.Version, t.CoverMediaID = nil, nil, nil, nil
		}
		for j := range t.Vocabulary {
			w := &t.Vocabulary[j]
			w.AudioURL = nil
			if w.AudioBucket != "" && w.AudioPath != "" {
				groups[w.AudioBucket] = append(groups[w.AudioBucket], w.AudioPath)
			}
			if !admin {
				w.AudioMediaID = nil
			}
		}
	}
	if len(groups) == 0 {
		return nil
	}
	if !s.storage.Enabled() {
		return ErrUnavailable
	}
	expiresAt := time.Now().UTC().Add(MediaURLTTL)
	signed := map[string]map[string]string{}
	for bucket, paths := range groups {
		urls, err := s.storage.SignedURLs(ctx, bucket, paths, MediaURLTTL)
		if err != nil {
			slog.Error("kotoba media signing failed", "bucket", bucket)
			return ErrStorage
		}
		for _, path := range paths {
			if urls[path] == "" {
				return ErrStorage
			}
		}
		signed[bucket] = urls
	}
	for i := range topics {
		t := &topics[i]
		t.CoverURL = signed[t.CoverBucket][t.CoverPath]
		t.MediaURLsExpiresAt = &expiresAt
		for j := range t.Vocabulary {
			w := &t.Vocabulary[j]
			if w.AudioPath != "" {
				value := signed[w.AudioBucket][w.AudioPath]
				w.AudioURL = &value
			}
		}
	}
	return nil
}
func (s *Service) List(ctx context.Context, f Filter, admin bool) ([]Topic, error) {
	if err := f.Validate(admin); err != nil {
		return nil, err
	}
	topics, err := s.repo.List(ctx, f, admin)
	if err != nil {
		return nil, err
	}
	if err := s.present(ctx, topics, admin); err != nil {
		return nil, err
	}
	return topics, nil
}
func (s *Service) Detail(ctx context.Context, id uuid.UUID, admin bool) (Topic, error) {
	t, err := s.repo.Get(ctx, id, admin)
	if err != nil {
		return t, err
	}
	topics := []Topic{t}
	if err := s.present(ctx, topics, admin); err != nil {
		return Topic{}, err
	}
	return topics[0], nil
}
func sortedIDs(ids []uuid.UUID) []uuid.UUID {
	seen := map[uuid.UUID]bool{}
	result := []uuid.UUID{}
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].String() < result[j].String() })
	return result
}
func (s *Service) Save(ctx context.Context, id uuid.UUID, in TopicInput) (Topic, error) {
	create := id == uuid.Nil
	if err := in.Validate(!create); err != nil {
		return Topic{}, err
	}
	if create {
		id = uuid.New()
	}
	var saved Topic
	err := s.tx.Exec(ctx, func(tx pgx.Tx) error {
		repo := s.repo.UseTx(tx)
		oldMedia := []uuid.UUID{}
		if !create {
			version, err := repo.LockTopic(ctx, id)
			if err != nil {
				return err
			}
			if version != in.Version {
				return ErrConflict
			}
			oldMedia, err = repo.TopicMedia(ctx, id)
			if err != nil {
				return err
			}
			existing, err := repo.VocabularyIDs(ctx, id)
			if err != nil {
				return err
			}
			for i, w := range in.Vocabulary {
				if w.ID != nil && !existing[*w.ID] {
					return FieldErrors{fmt.Sprintf("vocabulary.%d.id", i): "Kosakata bukan bagian dari topik ini; kosongkan ID untuk kosakata baru."}
				}
			}
		}
		requested := map[uuid.UUID][]struct{ kind, field string }{}
		requested[in.CoverMediaID] = append(requested[in.CoverMediaID], struct{ kind, field string }{"cover", "cover_media_id"})
		ids := append([]uuid.UUID{}, oldMedia...)
		ids = append(ids, in.CoverMediaID)
		for i, w := range in.Vocabulary {
			if w.AudioMediaID != nil {
				ids = append(ids, *w.AudioMediaID)
				requested[*w.AudioMediaID] = append(requested[*w.AudioMediaID], struct{ kind, field string }{"audio", fmt.Sprintf("vocabulary.%d.audio_media_id", i)})
			}
		}
		ids = sortedIDs(ids)
		for _, mediaID := range ids {
			m, err := repo.LockMedia(ctx, mediaID)
			if err != nil && !errors.Is(err, ErrNotFound) {
				return err
			}
			for _, ref := range requested[mediaID] {
				if errors.Is(err, ErrNotFound) || m.Status != "ready" || m.Kind != ref.kind {
					return FieldErrors{ref.field: "Media tidak ditemukan, belum siap, sedang dihapus, atau jenisnya tidak sesuai."}
				}
			}
			if err != nil {
				return err
			}
		}
		if err := repo.Save(ctx, id, in, create); err != nil {
			return err
		}
		if err := repo.ScheduleUnused(ctx, ids); err != nil {
			return err
		}
		var err error
		saved, err = repo.Get(ctx, id, true)
		return err
	})
	if err != nil {
		return Topic{}, err
	}
	topics := []Topic{saved}
	// The transaction has committed. A signing outage must not report a failed
	// save and encourage creation of duplicates; the client can refetch URLs.
	if err := s.present(ctx, topics, true); err != nil {
		slog.Error("kotoba saved without media URLs", "topic_id", saved.ID, "error", err)
	}
	return topics[0], nil
}
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.tx.Exec(ctx, func(tx pgx.Tx) error {
		repo := s.repo.UseTx(tx)
		if _, err := repo.LockTopic(ctx, id); err != nil {
			return err
		}
		ids, err := repo.TopicMedia(ctx, id)
		if err != nil {
			return err
		}
		ids = sortedIDs(ids)
		for _, mediaID := range ids {
			if _, err := repo.LockMedia(ctx, mediaID); err != nil {
				return err
			}
		}
		if err := repo.Remove(ctx, id); err != nil {
			return err
		}
		return repo.ScheduleUnused(ctx, ids)
	})
}
func (s *Service) Upload(ctx context.Context, user uuid.UUID, kind, name, contentType string, source io.Reader) (Media, error) {
	if !s.storage.Enabled() || s.coverBucket == "" || s.audioBucket == "" {
		return Media{}, ErrUnavailable
	}
	data, mime, ext, err := ValidateMedia(kind, name, contentType, source)
	if err != nil {
		return Media{}, err
	}
	m := Media{ID: uuid.New(), Kind: kind, MIMEType: mime, SizeBytes: int64(len(data)), UploadedBy: user, Status: "uploading"}
	m.Bucket = s.coverBucket
	if kind == "audio" {
		m.Bucket = s.audioBucket
	}
	m.ObjectPath = m.ID.String() + ext
	if err := s.repo.InsertMedia(ctx, m); err != nil {
		return Media{}, err
	}
	// This row remains recoverable by cleanup even if the remote request or the
	// ready-state update fails, including an ambiguous remote timeout.
	if err := s.storage.Upload(ctx, m.Bucket, m.ObjectPath, m.MIMEType, bytes.NewReader(data)); err != nil {
		slog.Error("kotoba media upload failed", "media_id", m.ID, "error", err)
		return Media{}, ErrStorage
	}
	if err := s.repo.ReadyMedia(ctx, m.ID); err != nil {
		return Media{}, err
	}
	m.Status = "ready"
	expiresAt := time.Now().UTC().Add(MediaURLTTL)
	urls, err := s.storage.SignedURLs(ctx, m.Bucket, []string{m.ObjectPath}, MediaURLTTL)
	if err != nil || urls[m.ObjectPath] == "" {
		// The object is already uploaded; retain the successful media ID so it
		// can still be attached and its preview refreshed via topic detail.
		slog.Error("kotoba uploaded without preview URL", "media_id", m.ID)
	} else {
		m.URL, m.URLExpiresAt = urls[m.ObjectPath], &expiresAt
	}
	return m, nil
}

// Cleanup processes a bounded batch. A persisted lease makes concurrent workers
// and process restarts safe; remote calls never hold open database transactions.
func (s *Service) Cleanup(ctx context.Context, now time.Time) error {
	if !s.storage.Enabled() {
		return nil
	}
	started := time.Now()
	for range 100 {
		if err := ctx.Err(); err != nil {
			return err
		}
		// Keep leases relative to each claim even when earlier remote deletions
		// in this batch took several minutes. now also permits deterministic tests.
		claimTime := now.Add(time.Since(started))
		m, err := s.repo.ClaimCleanup(ctx, claimTime)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		err = s.storage.Delete(ctx, m.Bucket, m.ObjectPath)
		if err == nil {
			err = s.repo.CompleteCleanup(ctx, m.ID)
		}
		if err != nil {
			slog.Error("kotoba media cleanup failed", "media_id", m.ID, "attempt", m.Attempts, "error", err)
			delay := CleanupInterval * time.Duration(1<<min(max(m.Attempts-1, 0), 4))
			delay = min(delay, time.Hour)
			if retryErr := s.repo.RetryCleanup(ctx, m.ID, now.Add(time.Since(started)).Add(delay)); retryErr != nil {
				return retryErr
			}
		}
	}
	return nil
}
func (s *Service) RunCleanup(ctx context.Context) {
	if !s.storage.Enabled() {
		return
	}
	run := func() {
		if err := s.Cleanup(ctx, time.Now()); err != nil && ctx.Err() == nil {
			slog.Error("kotoba cleanup worker failed", "error", err)
		}
	}
	run()
	timer := time.NewTicker(CleanupInterval)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			run()
		}
	}
}
