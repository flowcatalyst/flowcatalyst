//go:build integration

package dispatchjob_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// A size above the 1,000 cap is clamped to 1,000, not replaced by the
// default 100 (bug 21 in docs/ui/README.md; docs/platform-bug-fixes.md):
// size=1001 returned fewer rows than size=1000.
func TestFindWithFilters_SizeAboveTheCapIsClamped(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := dispatchjob.NewRepository(pool)
	const code = "clamptest:jobs:list"
	_, err := pool.Exec(ctx, `
		INSERT INTO msg_dispatch_jobs_read (id, code, target_url, kind, protocol, mode, status, max_retries, created_at, updated_at)
		SELECT 'djclp' || lpad(n::text, 8, '0'), $1, 'http://example.invalid/hook', 'EVENT', 'HTTP_WEBHOOK', 'IMMEDIATE', 'PENDING', 3, NOW(), NOW()
		  FROM generate_series(1, 150) n`, code)
	require.NoError(t, err)

	for _, size := range []int{1000, 1001, 5000} {
		rows, err := repo.FindWithFilters(ctx, dispatchjob.FilterParams{Codes: []string{code}, Limit: size})
		require.NoError(t, err)
		assert.Equal(t, 150, len(rows), "size %d", size)
	}
	rows, err := repo.FindWithFilters(ctx, dispatchjob.FilterParams{Codes: []string{code}})
	require.NoError(t, err)
	assert.Equal(t, 100, len(rows), "no size: the default 100")
}
