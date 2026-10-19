package function_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
)

var pruneNow = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

func ver(n int32, status function.VersionStatus, age time.Duration) function.Version {
	return function.Version{ID: fmt.Sprintf("v%d", n), Number: n, Status: status, CreatedAt: pruneNow.Add(-age)}
}

func numbers(vs []function.Version) []int32 {
	out := []int32{}
	for _, v := range vs {
		out = append(out, v.Number)
	}
	return out
}

func many(n int) []function.Version {
	var vs []function.Version
	for i := 1; i <= n; i++ {
		vs = append(vs, ver(int32(i), function.VersionReady, 48*time.Hour))
	}
	return vs
}

func TestSelectPrunable_OldestFirstDownToKeep(t *testing.T) {
	got := function.SelectPrunable(many(25), nil, 20, pruneNow)
	assert.Equal(t, []int32{1, 2, 3, 4, 5}, numbers(got))
}

func TestSelectPrunable_AtOrUnderLimitPrunesNothing(t *testing.T) {
	assert.Empty(t, function.SelectPrunable(many(20), nil, 20, pruneNow))
	assert.Empty(t, function.SelectPrunable(many(3), nil, 20, pruneNow))
	assert.Empty(t, function.SelectPrunable(many(30), nil, 0, pruneNow), "keep < 1 selects nothing")
}

func TestSelectPrunable_TaggedNeverPrunedAndNotCounted(t *testing.T) {
	// 22 versions, v1 and v2 tagged: 20 untagged = at the limit, nothing goes.
	tagged := map[string]bool{"v1": true, "v2": true}
	assert.Empty(t, function.SelectPrunable(many(22), tagged, 20, pruneNow))
	// 23 versions: 21 untagged, one over; v1/v2 are skipped, v3 is oldest untagged.
	got := function.SelectPrunable(many(23), tagged, 20, pruneNow)
	assert.Equal(t, []int32{3}, numbers(got))
}

func TestSelectPrunable_NewestIsProtected(t *testing.T) {
	// keep 1 of 3 untagged: v1, v2 go; the newest (v3) stays.
	assert.Equal(t, []int32{1, 2}, numbers(function.SelectPrunable(many(3), nil, 1, pruneNow)))
	// A tagged newest still counts as the newest: v3 tagged, v2 untagged is
	// not the newest overall, so it is prunable.
	got := function.SelectPrunable(many(3), map[string]bool{"v3": true}, 1, pruneNow)
	assert.Equal(t, []int32{1}, numbers(got))
}

func TestSelectPrunable_YoungPublishedIsProtectedOldPublishedIsNot(t *testing.T) {
	vs := []function.Version{
		ver(1, function.VersionPublished, 10*time.Minute), // young, waiting for a runner
		ver(2, function.VersionPublished, 3*time.Hour),    // stuck for hours: prunable
		ver(3, function.VersionReady, 48*time.Hour),
		ver(4, function.VersionReady, 48*time.Hour), // newest
	}
	got := function.SelectPrunable(vs, nil, 2, pruneNow)
	assert.Equal(t, []int32{2, 3}, numbers(got))
}

func TestSelectPrunable_StatusesAllCount(t *testing.T) {
	vs := []function.Version{
		ver(1, function.VersionRetired, 48*time.Hour),
		ver(2, function.VersionFailed, 48*time.Hour),
		ver(3, function.VersionReady, 48*time.Hour),
		ver(4, function.VersionReady, 48*time.Hour),
	}
	assert.Equal(t, []int32{1, 2}, numbers(function.SelectPrunable(vs, nil, 2, pruneNow)))
}

func TestSelectPrunable_FewerThanExcessWhenProtected(t *testing.T) {
	// 3 untagged, keep 1: excess 2, but v2 is young PUBLISHED and v3 is the
	// newest, so only v1 is pruned this time.
	vs := []function.Version{
		ver(1, function.VersionReady, 48*time.Hour),
		ver(2, function.VersionPublished, time.Minute),
		ver(3, function.VersionReady, 48*time.Hour),
	}
	assert.Equal(t, []int32{1}, numbers(function.SelectPrunable(vs, nil, 1, pruneNow)))
}
