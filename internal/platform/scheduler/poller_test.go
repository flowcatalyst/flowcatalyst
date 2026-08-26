package scheduler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// These tests cover the pure filter helpers.

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
	// NULL message_group scans as "" — it buckets under "default".
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

// heldBy builds the holder map: group → the id of the earliest job holding it.
// Claims tie on sequence and created_at here, so ordering falls to the id,
// which makes "j0 holds, j1 and j2 wait behind it" read directly.
func heldBy(group, holderID string) map[string]jobKey {
	return map[string]jobKey{group: {id: holderID}}
}

func TestFilterByDispatchMode_ImmediateAlwaysPasses(t *testing.T) {
	result := filterByDispatchMode([]dispatchClaim{
		mkClaim("j1", "grp_a", "IMMEDIATE"),
		mkClaim("j2", "grp_b", "IMMEDIATE"),
	}, heldBy("grp_a", "j0"))
	assert.Len(t, result, 2)
}

func TestFilterByDispatchMode_BlockOnErrorWaitsBehindAHeldJob(t *testing.T) {
	result := filterByDispatchMode([]dispatchClaim{
		mkClaim("j1", "grp_a", "BLOCK_ON_ERROR"),
		mkClaim("j2", "grp_b", "BLOCK_ON_ERROR"),
	}, heldBy("grp_a", "j0"))
	assert.Equal(t, []string{"j2"}, claimIDs(result))
}

// The rule is positional. A job that is ITSELF the one holding the group — the
// backed-off job, now that its backoff has expired — must dispatch, or the
// group it is at the front of would never move again.
func TestFilterByDispatchMode_TheHeldJobItselfDispatches(t *testing.T) {
	result := filterByDispatchMode([]dispatchClaim{
		mkClaim("j1", "grp", "BLOCK_ON_ERROR"),
	}, heldBy("grp", "j1"))
	assert.Equal(t, []string{"j1"}, claimIDs(result),
		"the head of the group is not blocked by its own hold")
}

// And a job in front of the holder is unaffected: only what is BEHIND waits.
func TestFilterByDispatchMode_JobsAheadOfTheHolderPass(t *testing.T) {
	result := filterByDispatchMode([]dispatchClaim{
		mkClaim("j1", "grp", "BLOCK_ON_ERROR"),
		mkClaim("j3", "grp", "BLOCK_ON_ERROR"),
	}, heldBy("grp", "j2"))
	assert.Equal(t, []string{"j1"}, claimIDs(result))
}

// NEXT_ON_ERROR is ordered but explicitly "the group moves on" past a
// failure — only BLOCK_ON_ERROR stops for a sibling in front of it.
func TestFilterByDispatchMode_NextOnErrorPassesWhenGroupHeld(t *testing.T) {
	result := filterByDispatchMode([]dispatchClaim{
		mkClaim("j1", "grp_x", "NEXT_ON_ERROR"),
		mkClaim("j2", "grp_y", "NEXT_ON_ERROR"),
	}, heldBy("grp_x", "j0"))
	assert.Equal(t, []string{"j1", "j2"}, claimIDs(result))
}

func TestFilterByDispatchMode_NoHeldGroupsPassesEverything(t *testing.T) {
	result := filterByDispatchMode([]dispatchClaim{
		mkClaim("j1", "g1", "BLOCK_ON_ERROR"),
		mkClaim("j2", "g2", "NEXT_ON_ERROR"),
		mkClaim("j3", "g3", "IMMEDIATE"),
	}, map[string]jobKey{})
	assert.Len(t, result, 3)
}

func TestFilterByDispatchMode_MixedModesInSameGroup(t *testing.T) {
	result := filterByDispatchMode([]dispatchClaim{
		mkClaim("j_imm", "grp", "IMMEDIATE"),
		mkClaim("j_noe", "grp", "NEXT_ON_ERROR"),
		mkClaim("j_boe", "grp", "BLOCK_ON_ERROR"),
	}, heldBy("grp", "j_aaa"))
	// Only BLOCK_ON_ERROR stops for a sibling in front of it.
	assert.Equal(t, []string{"j_imm", "j_noe"}, claimIDs(result))
}

func TestFilterByDispatchMode_UngroupedUsesDefaultKey(t *testing.T) {
	// Ungrouped BLOCK_ON_ERROR jobs check the "default" bucket.
	result := filterByDispatchMode([]dispatchClaim{
		mkClaim("j1", "", "BLOCK_ON_ERROR"),
		mkClaim("j2", "", "IMMEDIATE"),
	}, heldBy(defaultMessageGroup, "j0"))
	assert.Equal(t, []string{"j2"}, claimIDs(result))
}

func TestFilterByDispatchMode_UnknownModeIsNotBlockOnError(t *testing.T) {
	// An unrecognised mode takes the default (NEXT_ON_ERROR), which orders but
	// does not stop for a sibling — so it flows.
	result := filterByDispatchMode([]dispatchClaim{
		mkClaim("j1", "grp", "SOMETHING_ELSE"),
	}, heldBy("grp", "j0"))
	assert.Equal(t, []string{"j1"}, claimIDs(result))
}

// jobKey is the total order the hold-back compares against: sequence first,
// then created_at, then id.
func TestJobKeyOrdering(t *testing.T) {
	early := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	late := early.Add(time.Second)

	assert.True(t, jobKey{sequence: 1, createdAt: late, id: "z"}.
		before(jobKey{sequence: 2, createdAt: early, id: "a"}), "sequence wins")
	assert.True(t, jobKey{sequence: 1, createdAt: early, id: "z"}.
		before(jobKey{sequence: 1, createdAt: late, id: "a"}), "then created_at")
	assert.True(t, jobKey{sequence: 1, createdAt: early, id: "a"}.
		before(jobKey{sequence: 1, createdAt: early, id: "b"}), "then id")
	assert.False(t, jobKey{sequence: 1, createdAt: early, id: "a"}.
		before(jobKey{sequence: 1, createdAt: early, id: "a"}), "a job is not before itself")
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
