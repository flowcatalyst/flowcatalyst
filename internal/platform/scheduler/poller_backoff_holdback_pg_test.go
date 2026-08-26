//go:build integration

package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// seedSequencedJob inserts a PENDING job at a known position in its group,
// optionally backed off (scheduled_for in the future) or in another status.
// created_at is explicit so the group's order is the test's to control.
func seedSequencedJob(t *testing.T, pool *pgxpool.Pool, id, group, status string, createdAt time.Time, scheduledFor *time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO msg_dispatch_jobs
		    (id, code, target_url, status, mode, message_group, sequence,
		     created_at, updated_at, scheduled_for)
		 VALUES ($1, 'app:evt', 'http://sub.example/hook', $2, 'BLOCK_ON_ERROR',
		         $3, 10, $4, $4, $5)`,
		id, status, group, createdAt, scheduledFor)
	require.NoError(t, err)
}

// A job sitting out a retry backoff holds its group. It is PENDING with a
// future scheduled_for, so it is invisible to the claim query and is not
// FAILED — which is exactly why nothing used to treat it as holding anything,
// and its successors were claimed and delivered while it waited.
//
// It is still the next message that must be delivered, so nothing behind it may
// go past it.
func TestPollOnce_BackedOffJobHoldsItsGroup(t *testing.T) {
	pool := testpg.Pool(t)
	base := time.Now().UTC().Add(-time.Hour)
	backoff := time.Now().UTC().Add(30 * time.Second)

	// j1 delivered; j2 failed transiently and is backed off; j3, j4 behind it.
	seedSequencedJob(t, pool, "djbackoff001", "grp-boff", "COMPLETED", base, nil)
	seedSequencedJob(t, pool, "djbackoff002", "grp-boff", "PENDING", base.Add(time.Second), &backoff)
	seedSequencedJob(t, pool, "djbackoff003", "grp-boff", "PENDING", base.Add(2*time.Second), nil)
	seedSequencedJob(t, pool, "djbackoff004", "grp-boff", "PENDING", base.Add(3*time.Second), nil)

	msgs := pollAndCapture(t, pool)

	assert.Empty(t, msgs, "nothing may be published past a job waiting out its backoff")
	assert.Equal(t, "PENDING", jobStatus(t, pool, "djbackoff003"))
	assert.Equal(t, "PENDING", jobStatus(t, pool, "djbackoff004"))
}

// The other half: once the backoff expires the held job is claimable, and it
// must dispatch. The hold-back is positional, so a job cannot be blocked by its
// own hold — a set-membership test ("this group contains a backed-off job")
// would wedge the group permanently at exactly this moment.
func TestPollOnce_HeldJobDispatchesOnceItsBackoffExpires(t *testing.T) {
	pool := testpg.Pool(t)
	base := time.Now().UTC().Add(-time.Hour)
	expired := time.Now().UTC().Add(-time.Second) // backoff already elapsed

	seedSequencedJob(t, pool, "djbackoff011", "grp-ready", "PENDING", base, &expired)
	seedSequencedJob(t, pool, "djbackoff012", "grp-ready", "PENDING", base.Add(time.Second), nil)

	msgs := pollAndCapture(t, pool)

	require.NotEmpty(t, msgs, "the head of the group must dispatch once it is eligible")
	ids := make([]string, 0, len(msgs))
	for _, m := range msgs {
		ids = append(ids, m.ID)
	}
	assert.Contains(t, ids, "djbackoff011", "the previously backed-off job goes first")
}

// A held job holds only what is BEHIND it. Anything in front is unaffected —
// the old set-membership rule blocked a whole group regardless of position.
func TestPollOnce_JobsAheadOfABackedOffSiblingStillDispatch(t *testing.T) {
	pool := testpg.Pool(t)
	base := time.Now().UTC().Add(-time.Hour)
	backoff := time.Now().UTC().Add(30 * time.Second)

	seedSequencedJob(t, pool, "djbackoff021", "grp-ahead", "PENDING", base, nil)
	seedSequencedJob(t, pool, "djbackoff022", "grp-ahead", "PENDING", base.Add(time.Second), &backoff)
	seedSequencedJob(t, pool, "djbackoff023", "grp-ahead", "PENDING", base.Add(2*time.Second), nil)

	msgs := pollAndCapture(t, pool)

	require.Len(t, msgs, 1, "only the job in front of the backed-off one may go")
	assert.Equal(t, "djbackoff021", msgs[0].ID)
	assert.Equal(t, "PENDING", jobStatus(t, pool, "djbackoff023"), "the one behind waits")
}
