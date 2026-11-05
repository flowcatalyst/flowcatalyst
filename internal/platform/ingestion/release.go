package ingestion

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
)

// MaxReleaseIDs bounds the ingestions one release call names.
const MaxReleaseIDs = 1000

// ReleaseRequest is a runtime handing back the leases of single-owner routes it
// has stopped (POST /api/ingestion-runtime/v1/leases/release).
type ReleaseRequest struct {
	RuntimeID string
	// InstanceID, when set, must be the instance that last polled as the
	// runtime (or none was stored): an imposter cannot release another
	// process's leases.
	InstanceID *string
	// IngestionIDs are the ingestions whose routes have stopped. One the
	// runtime does not hold is ignored.
	IngestionIDs []string
	// Goodbye says the runtime is shutting down: it stops counting as live at
	// once, for fair shares and for the instance guard.
	Goodbye bool
	// Now is the time the release is made as of; zero is the database's clock
	// (see PollRequest.Now).
	Now time.Time
}

// Release ends, now, the leases of the named ingestions that the runtime
// holds, so another runtime's next poll can take them, and with Goodbye marks
// the runtime gone. It answers how many leases it ended. It runs under the
// poll's lease lock, so it never interleaves with a poll's renew-and-claim. A
// release from another instance than the one stored is refused
// (RUNTIME_ID_IN_USE) and changes nothing.
func (r *Repository) Release(ctx context.Context, req ReleaseRequest) (int, error) {
	ids := req.IngestionIDs
	if ids == nil {
		ids = []string{} // not nil: sqlc sends a nil slice as NULL
	}
	var released int64
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)
		now, err := clock(ctx, q, req.Now)
		if err != nil {
			return err
		}
		// The runtime row first, then the lease lock: the order a poll takes them.
		prev, err := previousPoll(ctx, q, req.RuntimeID)
		if err != nil {
			return err
		}
		if req.InstanceID != nil && prev != nil && prev.InstanceID != nil && *prev.InstanceID != *req.InstanceID {
			return runtimeInUse(req.RuntimeID)
		}
		if err := q.IngestionLeasesLock(ctx); err != nil {
			return fmt.Errorf("lock leases: %w", err)
		}
		if len(ids) > 0 {
			if released, err = q.IngestionLeasesReleaseNow(ctx, dbq.IngestionLeasesReleaseNowParams{
				Now: now, RuntimeID: req.RuntimeID, IngestionIds: ids,
			}); err != nil {
				return fmt.Errorf("release leases: %w", err)
			}
		}
		if req.Goodbye {
			if err := q.IngestionRuntimeLeave(ctx, dbq.IngestionRuntimeLeaveParams{Now: now, RuntimeID: req.RuntimeID}); err != nil {
				return fmt.Errorf("leave: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("ingestion repo: release: %w", err)
	}
	return int(released), nil
}
