//go:build integration

package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// GET /api/dispatch-jobs/filter-options lists only the values of jobs the
// caller can see (bug 18 in docs/ui/README.md; docs/platform-bug-fixes.md):
// a client user saw every tenant's codes, client ids, pools and
// subscriptions.
func TestFilterOptions_ScopedToTheCallersClients(t *testing.T) {
	pool := testpg.Pool(t)
	s := newState(pool)
	mine, other := "clt_fomine00001", "clt_foother0001"
	seedJobs(t, pool, "fomin", "fomine:orders:order:created", "COMPLETED", &mine, 1)
	seedJobs(t, pool, "footh", "foother:orders:order:created", "FAILED", &other, 1)
	seedJobs(t, pool, "fopla", "foplatform:orders:order:created", "PENDING", nil, 1)

	out, err := s.filterOptions(clientCaller(mine), &apicommon.Empty{})
	require.NoError(t, err)
	assert.Contains(t, out.Body.Codes, "fomine:orders:order:created")
	assert.Contains(t, out.Body.Codes, "foplatform:orders:order:created", "platform jobs stay visible, as in the list")
	assert.NotContains(t, out.Body.Codes, "foother:orders:order:created")
	assert.Contains(t, out.Body.ClientIDs, mine)
	assert.NotContains(t, out.Body.ClientIDs, other)

	all, err := s.filterOptions(anchor(), &apicommon.Empty{})
	require.NoError(t, err)
	assert.Contains(t, all.Body.Codes, "foother:orders:order:created", "an anchor sees every tenant")
	assert.Contains(t, all.Body.ClientIDs, other)
}
