package kotoba

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	UseTx(pgx.Tx) Repository
	List(context.Context, Filter, bool) ([]Topic, error)
	Get(context.Context, uuid.UUID, bool) (Topic, error)
	LockTopic(context.Context, uuid.UUID) (int64, error)
	TopicMedia(context.Context, uuid.UUID) ([]uuid.UUID, error)
	VocabularyIDs(context.Context, uuid.UUID) (map[uuid.UUID]bool, error)
	Save(context.Context, uuid.UUID, TopicInput, bool) error
	Remove(context.Context, uuid.UUID) error
	InsertMedia(context.Context, Media) error
	ReadyMedia(context.Context, uuid.UUID) error
	LockMedia(context.Context, uuid.UUID) (Media, error)
	ScheduleUnused(context.Context, []uuid.UUID) error
	ClaimCleanup(context.Context, time.Time) (Media, error)
	CompleteCleanup(context.Context, uuid.UUID) error
	RetryCleanup(context.Context, uuid.UUID, time.Time) error
}
type querier interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}
type repository struct{ q querier }

func NewRepository(pool *pgxpool.Pool) Repository { return &repository{q: pool} }
func (r *repository) UseTx(tx pgx.Tx) Repository  { return &repository{q: tx} }
func dbError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var p *pgconn.PgError
	if errors.As(err, &p) && p.Code == "23505" {
		return fmt.Errorf("%w: %w", ErrConflict, err)
	}
	return err
}

const topicColumns = `t.id,t.title,l.code,t.category,t.description,t.order_index,t.is_active,t.is_visible,
    t.cover_media_id,t.version,t.created_at,t.updated_at,c.bucket,c.object_path,
    (SELECT count(*) FROM kotoba_vocabularies v WHERE v.topic_id=t.id)`
const topicFrom = ` FROM kotoba_topics t JOIN levels l ON l.id=t.level_id JOIN kotoba_media c ON c.id=t.cover_media_id `
const vocabularyJSON = `(SELECT coalesce(jsonb_agg(jsonb_build_object(
    'id',v.id,'kanji',v.kanji,'hiragana',v.hiragana,'furigana',v.furigana,'romaji',v.romaji,
    'translation',v.translation,'example',jsonb_build_object('japanese',v.example_japanese,'indonesian',v.example_indonesian),
    'type',v.type,'order_index',v.order_index,'audio_media_id',v.audio_media_id,
    'audio_bucket',a.bucket,'audio_path',a.object_path) ORDER BY v.order_index),'[]'::jsonb)
    FROM kotoba_vocabularies v LEFT JOIN kotoba_media a ON a.id=v.audio_media_id WHERE v.topic_id=t.id)`

func scanTopic(row interface{ Scan(...any) error }) (Topic, error) {
	var t Topic
	var active, visible bool
	var cover uuid.UUID
	var version int64
	var words []byte
	err := row.Scan(&t.ID, &t.Title, &t.Level, &t.Category, &t.Description, &t.OrderIndex, &active, &visible,
		&cover, &version, &t.CreatedAt, &t.UpdatedAt, &t.CoverBucket, &t.CoverPath, &t.VocabularyCount, &words)
	if err != nil {
		return t, dbError(err)
	}
	t.IsActive, t.IsVisible, t.CoverMediaID, t.Version = &active, &visible, &cover, &version
	var stored []struct {
		Vocabulary
		Bucket *string `json:"audio_bucket"`
		Path   *string `json:"audio_path"`
	}
	if err := json.Unmarshal(words, &stored); err != nil {
		return t, err
	}
	for _, w := range stored {
		if w.Bucket != nil {
			w.Vocabulary.AudioBucket = *w.Bucket
		}
		if w.Path != nil {
			w.Vocabulary.AudioPath = *w.Path
		}
		t.Vocabulary = append(t.Vocabulary, w.Vocabulary)
	}
	return t, nil
}
func (r *repository) List(ctx context.Context, f Filter, admin bool) ([]Topic, error) {
	order := map[string]string{"order": "t.order_index ASC,t.id ASC", "title": "t.title ASC,t.id ASC", "newest": "t.created_at DESC,t.id ASC", "vocabulary": "15 DESC,t.id ASC"}[f.Sort]
	if order == "" {
		return nil, FieldErrors{"sort": "Pengurutan tidak valid."}
	}
	rows, err := r.q.Query(ctx, `SELECT `+topicColumns+`, '[]'::jsonb`+topicFrom+`
        WHERE ($1 OR (t.is_active AND t.is_visible)) AND ($2='' OR l.code=$2)
        AND ($3='' OR strpos(lower(t.title || ' ' || t.description),lower($3))>0)
        AND ($4='' OR $4='all' OR ($4='active' AND t.is_active) OR ($4='inactive' AND NOT t.is_active))
        ORDER BY `+order, admin, f.Level, f.Search, f.Status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Topic{}
	for rows.Next() {
		t, err := scanTopic(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, rows.Err()
}
func (r *repository) Get(ctx context.Context, id uuid.UUID, admin bool) (Topic, error) {
	return scanTopic(r.q.QueryRow(ctx, `SELECT `+topicColumns+`,`+vocabularyJSON+topicFrom+` WHERE t.id=$1 AND ($2 OR t.is_active)`, id, admin))
}
func (r *repository) LockTopic(ctx context.Context, id uuid.UUID) (int64, error) {
	var version int64
	err := r.q.QueryRow(ctx, `SELECT version FROM kotoba_topics WHERE id=$1 FOR UPDATE`, id).Scan(&version)
	return version, dbError(err)
}
func (r *repository) TopicMedia(ctx context.Context, id uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.q.Query(ctx, `SELECT cover_media_id FROM kotoba_topics WHERE id=$1 UNION SELECT audio_media_id FROM kotoba_vocabularies WHERE topic_id=$1 AND audio_media_id IS NOT NULL`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
func (r *repository) VocabularyIDs(ctx context.Context, id uuid.UUID) (map[uuid.UUID]bool, error) {
	rows, err := r.q.Query(ctx, `SELECT id FROM kotoba_vocabularies WHERE topic_id=$1`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := map[uuid.UUID]bool{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids[id] = true
	}
	return ids, rows.Err()
}
func (r *repository) Save(ctx context.Context, id uuid.UUID, in TopicInput, create bool) error {
	args := []any{id, in.Title, in.Level, in.Category, in.Description, in.OrderIndex, in.IsActive, in.IsVisible, in.CoverMediaID}
	query := `INSERT INTO kotoba_topics(id,title,level_id,category,description,order_index,is_active,is_visible,cover_media_id)
        SELECT $1,$2,l.id,$4,$5,$6,$7,$8,$9 FROM levels l WHERE l.code=$3`
	if !create {
		query = `UPDATE kotoba_topics t SET title=$2,level_id=l.id,category=$4,description=$5,order_index=$6,
            is_active=$7,is_visible=$8,cover_media_id=$9,version=t.version+1,updated_at=now() FROM levels l WHERE t.id=$1 AND l.code=$3`
	}
	tag, err := r.q.Exec(ctx, query, args...)
	if err != nil {
		return dbError(err)
	}
	if tag.RowsAffected() == 0 {
		return FieldErrors{"level": "Level belum tersedia."}
	}
	if _, err := r.q.Exec(ctx, `DELETE FROM kotoba_vocabularies WHERE topic_id=$1`, id); err != nil {
		return err
	}
	for i, w := range in.Vocabulary {
		wordID := uuid.New()
		if w.ID != nil {
			wordID = *w.ID
		}
		furigana, err := json.Marshal(w.Furigana)
		if err != nil {
			return err
		}
		_, err = r.q.Exec(ctx, `INSERT INTO kotoba_vocabularies(id,topic_id,kanji,hiragana,furigana,romaji,translation,
            example_japanese,example_indonesian,type,order_index,audio_media_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
			wordID, id, w.Kanji, w.Hiragana, string(furigana), w.Romaji, w.Translation, w.Example.Japanese, w.Example.Indonesian, w.Type, i+1, w.AudioMediaID)
		if err != nil {
			return dbError(err)
		}
	}
	return nil
}
func (r *repository) Remove(ctx context.Context, id uuid.UUID) error {
	_, err := r.q.Exec(ctx, `DELETE FROM kotoba_topics WHERE id=$1`, id)
	return err
}
func (r *repository) InsertMedia(ctx context.Context, m Media) error {
	_, err := r.q.Exec(ctx, `INSERT INTO kotoba_media(id,kind,bucket,object_path,mime_type,size_bytes,uploaded_by) VALUES($1,$2,$3,$4,$5,$6,$7)`,
		m.ID, m.Kind, m.Bucket, m.ObjectPath, m.MIMEType, m.SizeBytes, m.UploadedBy)
	return err
}
func (r *repository) ReadyMedia(ctx context.Context, id uuid.UUID) error {
	tag, err := r.q.Exec(ctx, `UPDATE kotoba_media SET status='ready' WHERE id=$1 AND status='uploading'`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

const mediaColumns = `id,kind,bucket,object_path,mime_type,size_bytes,uploaded_by,status,attempts`

func scanMedia(row interface{ Scan(...any) error }) (Media, error) {
	var m Media
	err := row.Scan(&m.ID, &m.Kind, &m.Bucket, &m.ObjectPath, &m.MIMEType, &m.SizeBytes, &m.UploadedBy, &m.Status, &m.Attempts)
	return m, dbError(err)
}
func (r *repository) LockMedia(ctx context.Context, id uuid.UUID) (Media, error) {
	return scanMedia(r.q.QueryRow(ctx, `SELECT `+mediaColumns+` FROM kotoba_media WHERE id=$1 FOR UPDATE`, id))
}

// Call while holding every affected media row lock, after changing the references.
func (r *repository) ScheduleUnused(ctx context.Context, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := r.q.Exec(ctx, `UPDATE kotoba_media m SET cleanup_after=CASE WHEN
        EXISTS(SELECT 1 FROM kotoba_topics t WHERE t.cover_media_id=m.id) OR
        EXISTS(SELECT 1 FROM kotoba_vocabularies v WHERE v.audio_media_id=m.id)
		THEN NULL ELSE coalesce(m.cleanup_after,now()+interval '24 hours') END WHERE m.id=ANY($1::uuid[])`, uuidStrings(ids))
	return err
}

// QueryExecModeExec has no statement parameter OIDs, so UUID slices must be
// supplied as text arrays, as with the existing prerequisite repositories.
func uuidStrings(ids []uuid.UUID) []string {
	values := make([]string, len(ids))
	for i, id := range ids {
		values[i] = id.String()
	}
	return values
}

// The deleting state is committed before the remote request. Attachments lock the
// same row and accept only ready media, so a claimed object cannot be reattached.
func (r *repository) ClaimCleanup(ctx context.Context, now time.Time) (Media, error) {
	return scanMedia(r.q.QueryRow(ctx, `WITH candidate AS (
        SELECT m.id FROM kotoba_media m WHERE m.cleanup_after <= $1
        AND (m.retry_after IS NULL OR m.retry_after <= $1)
        AND NOT EXISTS(SELECT 1 FROM kotoba_topics t WHERE t.cover_media_id=m.id)
        AND NOT EXISTS(SELECT 1 FROM kotoba_vocabularies v WHERE v.audio_media_id=m.id)
        ORDER BY m.cleanup_after,m.id FOR UPDATE OF m SKIP LOCKED LIMIT 1)
        UPDATE kotoba_media SET status='deleting',attempts=attempts+1,retry_after=$1+interval '5 minutes'
        WHERE id IN (SELECT id FROM candidate) RETURNING `+mediaColumns, now))
}
func (r *repository) CompleteCleanup(ctx context.Context, id uuid.UUID) error {
	_, err := r.q.Exec(ctx, `DELETE FROM kotoba_media WHERE id=$1 AND status='deleting'`, id)
	return err
}
func (r *repository) RetryCleanup(ctx context.Context, id uuid.UUID, retry time.Time) error {
	_, err := r.q.Exec(ctx, `UPDATE kotoba_media SET retry_after=$2 WHERE id=$1 AND status='deleting'`, id, retry)
	return err
}
