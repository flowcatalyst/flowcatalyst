package payloadstore

import (
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
)

// Row is one msg_payloads row to insert, as Prepare returns it: the payload
// in Data, or, when it was offloaded, its reference in Ref (Data then nil).
// Size is the payload's length as sent.
type Row struct {
	ID        string
	CreatedAt time.Time
	Data      []byte
	Ref       *Ref
	Size      int
}

// Offloaded reports whether the row holds a reference instead of the bytes.
func (r Row) Offloaded() bool { return r.Ref != nil }

// InsertSQL inserts one msg_payloads row; its arguments are Row.InsertArgs.
// For a caller that inserts the payload row in a statement of its own
// (QueueInsert queues exactly this).
const InsertSQL = `INSERT INTO msg_payloads (id, created_at, data, ref, size) VALUES ($1, $2, $3, $4, $5)`

// InsertArgs are the arguments of InsertSQL for the row: exactly one of data
// and ref is non-NULL.
func (r Row) InsertArgs() []any {
	var data, ref *string
	if r.Ref != nil {
		s := r.Ref.String()
		ref = &s
	} else {
		s := string(r.Data)
		data = &s
	}
	return []any{r.ID, r.CreatedAt, data, ref, int32(r.Size)}
}

// QueueInsert queues the insert of each row on b, so a caller inserts its
// payload rows in the same pgx.Batch (and transaction, when b is sent on one)
// as the event or job rows that point at them. Each row is one queued
// statement, in order.
func QueueInsert(b *pgx.Batch, rows ...Row) {
	for _, r := range rows {
		b.Queue(InsertSQL, r.InsertArgs()...)
	}
}

// Batcher sends a pgx.Batch: a pgx.Tx, a *pgxpool.Pool or a *pgx.Conn.
type Batcher interface {
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

// Insert inserts rows on db in one round trip; on a pgx.Tx they commit or
// roll back with it.
func Insert(ctx context.Context, db Batcher, rows []Row) error {
	if len(rows) == 0 {
		return nil
	}
	b := &pgx.Batch{}
	QueueInsert(b, rows...)
	br := db.SendBatch(ctx, b)
	for _, r := range rows {
		if _, err := br.Exec(); err != nil {
			_ = br.Close()
			return fmt.Errorf("payloadstore: insert payload %s: %w", r.ID, err)
		}
	}
	if err := br.Close(); err != nil {
		return fmt.Errorf("payloadstore: insert payloads: %w", err)
	}
	return nil
}

// Stored is a msg_payloads row as a reader selected it (data, ref, size),
// for Resolve. ID is the payload id, used for the cache and in logs.
type Stored struct {
	ID   string
	Data *string
	Ref  *string
	Size int
}

// Get reads the payload id stored under createdAt (the created_at of the
// event or job that points at it) by primary key and resolves it. A row that
// is not there is ErrMissing.
func (s *Service) Get(ctx context.Context, id string, createdAt time.Time) ([]byte, error) {
	if s.q == nil {
		return nil, errors.New("payloadstore: Get needs a database (the service was built without one)")
	}
	row, err := s.q.PayloadFindByKey(ctx, dbq.PayloadFindByKeyParams{ID: id, CreatedAt: createdAt})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, s.missing(id, "", "no payload row under its created_at")
	}
	if err != nil {
		return nil, fmt.Errorf("payloadstore: read payload %s: %w", id, err)
	}
	return s.Resolve(ctx, Stored{ID: row.ID, Data: row.Data, Ref: row.Ref, Size: int(row.Size)})
}

// Resolve produces the payload of a row: its data, or the resolved reference.
// Resolving a reference reads its range from the store (or the cache),
// verifies the CRC-32C of the stored bytes, decompresses them and checks the
// length equals the row's size.
//
//   - ErrMissing: the object is gone, or the bytes do not match (range,
//     checksum, decompression, size), or the reference is malformed. Logged
//     at error with the payload id and key, never the bytes.
//   - ErrStoreUnavailable: any other store failure (retryable).
//   - ErrStoreNotConfigured: the row holds a reference and no store is
//     configured.
func (s *Service) Resolve(ctx context.Context, p Stored) ([]byte, error) {
	if p.Ref == nil {
		if p.Data == nil {
			return nil, s.missing(p.ID, "", "row has neither data nor ref")
		}
		return []byte(*p.Data), nil
	}
	if s.store == nil {
		return nil, fmt.Errorf("%w: payload %s is offloaded", ErrStoreNotConfigured, p.ID)
	}
	if b, ok := s.cache.get(p.ID); ok {
		cacheHitsTotal.Inc()
		return b, nil
	}
	ref, err := ParseRef(*p.Ref)
	if err != nil {
		return nil, s.missing(p.ID, "", err.Error())
	}
	start := time.Now()
	stored, err := s.store.GetRange(ctx, ref.Key, ref.Offset, ref.Length)
	storeGetSeconds.Observe(time.Since(start).Seconds())
	switch {
	case errors.Is(err, ErrObjectNotFound):
		s.health.success(s.now()) // the store answered
		return nil, s.missing(p.ID, ref.Key, "object not found")
	case err != nil:
		storeErrors.WithLabelValues(opGet).Inc()
		s.health.failure(s.now(), HealthOpGet)
		return nil, fmt.Errorf("%w: payload %s: %w", ErrStoreUnavailable, p.ID, err)
	}
	s.health.success(s.now())
	if int64(len(stored)) != ref.Length {
		return nil, s.missing(p.ID, ref.Key, fmt.Sprintf("range read %d bytes, want %d", len(stored), ref.Length))
	}
	if crc32.Checksum(stored, castagnoli) != ref.CRC32C {
		return nil, s.missing(p.ID, ref.Key, "checksum mismatch")
	}
	payload, err := decode(ref.Codec, stored, p.Size)
	if err != nil {
		return nil, s.missing(p.ID, ref.Key, err.Error())
	}
	s.cache.put(p.ID, payload) // the cache keeps a copy of its own
	return payload, nil
}

// missing counts and logs a payload that cannot be produced and returns
// ErrMissing. reason never holds payload bytes.
func (s *Service) missing(id, key, reason string) error {
	missingTotal.Inc()
	slog.Error("payload missing", "payload_id", id, "key", key, "reason", reason)
	if key == "" {
		return fmt.Errorf("%w: payload %s: %s", ErrMissing, id, reason)
	}
	return fmt.Errorf("%w: payload %s (key %s): %s", ErrMissing, id, key, reason)
}
