//go:build integration

package event_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/event"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// A size above the 1,000 cap is clamped to 1,000, not replaced by the
// default 100 (the bug 21 flaw, in the events list too;
// docs/platform-bug-fixes.md).
func TestFindWithFilters_SizeAboveTheCapIsClamped(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := event.NewRepository(pool)
	const typ = "clamptest:events:list:made"
	_, err := pool.Exec(ctx, `
		INSERT INTO msg_events_read (id, type, source, time, created_at)
		SELECT 'evclp' || lpad(n::text, 8, '0'), $1, 'test://clamp', NOW(), NOW()
		  FROM generate_series(1, 150) n`, typ)
	require.NoError(t, err)

	for _, size := range []int{1000, 1001} {
		rows, err := repo.FindWithFilters(ctx, event.FilterParams{Types: []string{typ}, Limit: size})
		require.NoError(t, err)
		assert.Equal(t, 150, len(rows), "size %d", size)
	}
}
