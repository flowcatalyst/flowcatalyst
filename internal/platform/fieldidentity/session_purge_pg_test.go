//go:build integration

package fieldidentity_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

func TestPurgeLoginAttempts(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	repo := fieldidentity.NewRepos(pool).Sessions
	code := "purge-" + tsid.GenerateUntyped()

	insert := func(age string) {
		_, err := pool.Exec(ctx,
			`INSERT INTO field_login_attempts (id, worker_code, outcome, attempted_at)
			 VALUES ($1, $2, 'SUCCESS', NOW() - $3::interval)`, tsid.GenerateUntyped(), code, age)
		require.NoError(t, err)
	}
	count := func() int {
		var n int
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM field_login_attempts WHERE worker_code = $1`, code).Scan(&n))
		return n
	}
	insert("100 days")
	insert("91 days")
	insert("89 days")
	insert("1 hour")

	// Retention 0 (and negative) keeps everything.
	_, err := repo.PurgeLoginAttempts(ctx, 0)
	require.NoError(t, err)
	_, err = repo.PurgeLoginAttempts(ctx, -5)
	require.NoError(t, err)
	assert.Equal(t, 4, count())

	n, err := repo.PurgeLoginAttempts(ctx, 90)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, n, int64(2))
	assert.Equal(t, 2, count(), "only the rows inside the window remain")
}
