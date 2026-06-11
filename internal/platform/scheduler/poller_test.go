package scheduler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// These tests mirror the Rust poller's unit tests
// (fc-platform/src/scheduler/poller.rs) on the pure filter helpers.

func mkClaim(id, group, mode string) dispatchClaim {
	return dispatchClaim{id: id, group: group, mode: mode, target: "http://target.example.com/webhook"}
}

func mkClaimWithSub(id, group, subID string) dispatchClaim {
	c := mkClaim(id, group, "IMMEDIATE")
	c.subID = subID
	return c
}

func claimIDs(claims []dispatchClaim) []string {
	out := make([]string, 0, len(claims))
	for _, c := range claims {
		out = append(out, c.id)
	}
	return out
}

func TestGroupByMessageGroup_SeparatesGroups(t *testing.T) {
	grouped := groupByMessageGroup([]dispatchClaim{
		mkClaim("j1", "alpha", "IMMEDIATE"),
		mkClaim("j2", "beta", "IMMEDIATE"),
		mkClaim("j3", "alpha", "IMMEDIATE"),
	})
	assert.Len(t, grouped, 2)
	assert.Len(t, grouped["alpha"], 2)
	assert.Len(t, grouped["beta"], 1)
}

func TestGroupByMessageGroup_UngroupedUsesDefault(t *testing.T) {
	// NULL message_group scans as "" — it buckets under "default",
	// matching Rust's None → DEFAULT_MESSAGE_GROUP.
	grouped := groupByMessageGroup([]dispatchClaim{
		mkClaim("j1", "", "IMMEDIATE"),
		mkClaim("j2", "", "IMMEDIATE"),
		mkClaim("j3", "explicit", "IMMEDIATE"),
	})
	assert.Len(t, grouped, 2)
	assert.Len(t, grouped[defaultMessageGroup], 2)
	assert.Len(t, grouped["explicit"], 1)
}

func TestGroupByMessageGroup_EmptyInput(t *testing.T) {
	assert.Empty(t, groupByMessageGroup(nil))
}

func TestGroupByMessageGroup_PreservesJobIDsAndOrder(t *testing.T) {
	grouped := groupByMessageGroup([]dispatchClaim{
		mkClaim("aaa", "g1", "IMMEDIATE"),
		mkClaim("bbb", "g1", "IMMEDIATE"),
	})
	// Claim order (the poll query's sequence, created_at order) must
	// survive grouping — the dispatcher FIFO depends on it.
	assert.Equal(t, []string{"aaa", "bbb"}, claimIDs(grouped["g1"]))
}

func TestFilterByDispatchMode_ImmediateAlwaysPasses(t *testing.T) {
	blocked := map[string]struct{}{"grp_a": {}}
	result := filterByDispatchMode([]dispatchClaim{
		mkClaim("j1", "grp_a", "IMMEDIATE"),
		mkClaim("j2", "grp_b", "IMMEDIATE"),
	}, blocked)
	assert.Len(t, result, 2)
}

func TestFilterByDispatchMode_BlockOnErrorExcludedWhenGroupBlocked(t *testing.T) {
	blocked := map[string]struct{}{"grp_a": {}}
	result := filterByDispatchMode([]dispatchClaim{
		mkClaim("j1", "grp_a", "BLOCK_ON_ERROR"),
		mkClaim("j2", "grp_b", "BLOCK_ON_ERROR"),
	}, blocked)
	assert.Equal(t, []string{"j2"}, claimIDs(result))
}

func TestFilterByDispatchMode_NextOnErrorExcludedWhenGroupBlocked(t *testing.T) {
	blocked := map[string]struct{}{"grp_x": {}}
	result := filterByDispatchMode([]dispatchClaim{
		mkClaim("j1", "grp_x", "NEXT_ON_ERROR"),
		mkClaim("j2", "grp_y", "NEXT_ON_ERROR"),
	}, blocked)
	assert.Equal(t, []string{"j2"}, claimIDs(result))
}

func TestFilterByDispatchMode_NoBlockedGroupsPassesEverything(t *testing.T) {
	result := filterByDispatchMode([]dispatchClaim{
		mkClaim("j1", "g1", "BLOCK_ON_ERROR"),
		mkClaim("j2", "g2", "NEXT_ON_ERROR"),
		mkClaim("j3", "g3", "IMMEDIATE"),
	}, map[string]struct{}{})
	assert.Len(t, result, 3)
}

func TestFilterByDispatchMode_MixedModesInSameGroup(t *testing.T) {
	blocked := map[string]struct{}{"grp": {}}
	result := filterByDispatchMode([]dispatchClaim{
		mkClaim("j_imm", "grp", "IMMEDIATE"),
		mkClaim("j_noe", "grp", "NEXT_ON_ERROR"),
		mkClaim("j_boe", "grp", "BLOCK_ON_ERROR"),
	}, blocked)
	assert.Equal(t, []string{"j_imm"}, claimIDs(result))
}

func TestFilterByDispatchMode_UngroupedUsesDefaultKey(t *testing.T) {
	// Ungrouped ordered-mode jobs check the "default" bucket — same as
	// Rust's unwrap_or(DEFAULT_MESSAGE_GROUP) inside filter_by_dispatch_mode.
	blocked := map[string]struct{}{defaultMessageGroup: {}}
	result := filterByDispatchMode([]dispatchClaim{
		mkClaim("j1", "", "NEXT_ON_ERROR"),
		mkClaim("j2", "", "IMMEDIATE"),
	}, blocked)
	assert.Equal(t, []string{"j2"}, claimIDs(result))
}

func TestFilterByDispatchMode_UnknownModeCountsAsImmediate(t *testing.T) {
	// Rust's DispatchMode::from_str maps unknown strings to Immediate;
	// common.ParseDispatchMode does the same.
	blocked := map[string]struct{}{"grp": {}}
	result := filterByDispatchMode([]dispatchClaim{
		mkClaim("j1", "grp", "SOMETHING_ELSE"),
	}, blocked)
	assert.Equal(t, []string{"j1"}, claimIDs(result))
}

func TestFilterPausedSubscriptions(t *testing.T) {
	paused := map[string]struct{}{"sub_paused_1": {}, "sub_paused_2": {}}
	kept, skipped := filterPausedSubscriptions([]dispatchClaim{
		mkClaimWithSub("j1", "g", "sub_active"),
		mkClaimWithSub("j2", "g", "sub_paused_1"),
		mkClaimWithSub("j3", "g", ""), // no subscription always passes
		mkClaimWithSub("j4", "g", "sub_paused_2"),
	}, paused)
	assert.Equal(t, []string{"j1", "j3"}, claimIDs(kept))
	assert.Equal(t, 2, skipped)
}

func TestFilterPausedSubscriptions_NothingPaused(t *testing.T) {
	claims := []dispatchClaim{mkClaimWithSub("j1", "g", "sub_x")}
	kept, skipped := filterPausedSubscriptions(claims, map[string]struct{}{})
	assert.Equal(t, []string{"j1"}, claimIDs(kept))
	assert.Zero(t, skipped)
}
