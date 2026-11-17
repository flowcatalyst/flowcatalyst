//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A message held past its visibility timeout is claimed again by the next
// poll under a new receipt handle. A Nack sent with the handle it was first
// claimed under matches no row and reports no error, so the row stays claimed
// rather than nacked with a delay, and a claimed row does not hold back the
// rest of its group: the group's next member is claimed at once, ahead of the
// member that was handed back. Nacked with the current handle, the same row
// blocks its group (R4). The router therefore nacks with the freshest handle
// it has seen (Pool.nackMsg, TestReleaseNacksWithTheFreshestReceipt in
// internal/router).
func TestNackWithAStaleReceiptLeavesTheGroupOpen(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name      string
		useLatest bool
		wantNext  []string
	}{
		{name: "stale", useLatest: false, wantNext: []string{"m2"}},
		{name: "current", useLatest: true, wantNext: []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := newTestQueue(t, "stale-receipt-"+tc.name)
			group := "grp-stale-" + tc.name
			now := time.Now().Unix()
			insertQueued(t, q, "m1", group, now, now)
			insertQueued(t, q, "m2", group, now, now)

			first, err := q.Poll(ctx, 10)
			require.NoError(t, err)
			require.Equal(t, []string{"m1"}, idsOf(first))

			// m1's visibility timeout lapses while it is held; the next poll
			// claims it again under a new handle.
			setVisibleAt(t, q, "m1", now-1)
			second, err := q.Poll(ctx, 10)
			require.NoError(t, err)
			require.Equal(t, []string{"m1"}, idsOf(second))
			require.NotEqual(t, first[0].ReceiptHandle, second[0].ReceiptHandle)

			receipt := first[0].ReceiptHandle
			if tc.useLatest {
				receipt = second[0].ReceiptHandle
			}
			require.NoError(t, q.Nack(ctx, receipt, new(uint32(600))), "a stale handle is not reported")

			next, err := q.Poll(ctx, 10)
			require.NoError(t, err)
			assert.Equal(t, tc.wantNext, idsOf(next))
		})
	}
}
