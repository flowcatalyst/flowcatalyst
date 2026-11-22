package fieldidentity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PhotoSize names one of a photo's two renditions.
type PhotoSize string

// The renditions: full (fit within 512 px) and thumb (fit within 128 px).
const (
	PhotoThumb PhotoSize = "thumb"
	PhotoFull  PhotoSize = "full"
)

// Valid reports whether s is one of the two renditions.
func (s PhotoSize) Valid() bool { return s == PhotoThumb || s == PhotoFull }

// PhotoActor is who set or removed a photo.
type PhotoActor string

// The actors recorded with a photo, in events and in field_worker_photos.set_by_type.
const (
	PhotoByAdmin          PhotoActor = "ADMIN"
	PhotoByServiceAccount PhotoActor = "SERVICE_ACCOUNT"
	// PhotoByWorker is the worker themselves, from their device (the policy's
	// workerMaySetOwnPhoto).
	PhotoByWorker PhotoActor = "WORKER"
)

// Valid reports whether a is a known actor.
func (a PhotoActor) Valid() bool {
	switch a {
	case PhotoByAdmin, PhotoByServiceAccount, PhotoByWorker:
		return true
	}
	return false
}

// PhotoRepo reads field_worker_photos. Writes are the photo use cases' and
// the policy's, in their transactions (see photo_ops.go and PolicyRepo.Persist).
type PhotoRepo struct{ pool *pgxpool.Pool }

// StoredPhoto is one rendition read back with what an HTTP answer needs.
type StoredPhoto struct {
	Bytes         []byte
	ContentType   string
	Version       string
	Width, Height int
	UpdatedAt     time.Time
}

// Rendition returns one rendition of the worker's photo, found false when the
// worker has none.
func (r *PhotoRepo) Rendition(ctx context.Context, identityID string, size PhotoSize) (StoredPhoto, bool, error) {
	col := "thumb"
	if size == PhotoFull {
		col = "full"
	}
	var p StoredPhoto
	err := r.pool.QueryRow(ctx,
		`SELECT `+col+`_bytes, content_type, version, `+col+`_width, `+col+`_height, updated_at
		 FROM field_worker_photos WHERE identity_id = $1`, identityID).
		Scan(&p.Bytes, &p.ContentType, &p.Version, &p.Width, &p.Height, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return StoredPhoto{}, false, nil
	}
	if err != nil {
		return StoredPhoto{}, false, fmt.Errorf("field_worker_photos rendition: %w", err)
	}
	return p, true, nil
}

// CountForClient is how many photos the client's workers hold.
func (r *PhotoRepo) CountForClient(ctx context.Context, clientID string) (int, error) {
	var n int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM field_worker_photos WHERE client_id = $1`, clientID).Scan(&n); err != nil {
		return 0, fmt.Errorf("field_worker_photos count: %w", err)
	}
	return n, nil
}
