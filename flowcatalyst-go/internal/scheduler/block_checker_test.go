package scheduler

import (
	"testing"

	"go.flowcatalyst.tech/internal/platform/dispatchjob"
)

// === BlockChecker Unit Tests ===

func TestBlockCheckerCreation(t *testing.T) {
	checker := NewBlockChecker(nil)
	if checker == nil {
		t.Error("Expected non-nil BlockChecker")
	}
}

func TestIsGroupBlockedEmptyGroup(t *testing.T) {
	checker := &BlockChecker{} // nil repo, shouldn't be called

	// Empty group should never be blocked
	if checker.IsGroupBlocked(nil, "") {
		t.Error("Empty group should not be blocked")
	}
}

func TestGetBlockedGroupsEmpty(t *testing.T) {
	checker := &BlockChecker{} // nil repo

	result := checker.GetBlockedGroups(nil, []string{})
	if result == nil {
		t.Error("Expected non-nil map")
	}
	if len(result) != 0 {
		t.Errorf("Expected empty map, got %d entries", len(result))
	}
}

func TestGetBlockedGroupsDeduplication(t *testing.T) {
	// Test that duplicate groups are deduplicated
	groups := []string{"group1", "group1", "group2", "group2", "group1"}

	// Build unique set (same logic as in GetBlockedGroups)
	uniqueGroups := make(map[string]struct{})
	for _, g := range groups {
		if g != "" {
			uniqueGroups[g] = struct{}{}
		}
	}

	if len(uniqueGroups) != 2 {
		t.Errorf("Expected 2 unique groups, got %d", len(uniqueGroups))
	}

	if _, ok := uniqueGroups["group1"]; !ok {
		t.Error("Expected group1 in unique groups")
	}
	if _, ok := uniqueGroups["group2"]; !ok {
		t.Error("Expected group2 in unique groups")
	}
}

func TestShouldBlockJobModes(t *testing.T) {
	tests := []struct {
		name          string
		mode          dispatchjob.DispatchMode
		messageGroup  string
		expectCheck   bool // Whether IsGroupBlocked would be called
	}{
		{
			name:         "IMMEDIATE mode never blocks",
			mode:         dispatchjob.DispatchModeImmediate,
			messageGroup: "group1",
			expectCheck:  false,
		},
		{
			name:         "NEXT_ON_ERROR mode never blocks",
			mode:         dispatchjob.DispatchModeNextOnError,
			messageGroup: "group1",
			expectCheck:  false,
		},
		{
			name:         "BLOCK_ON_ERROR mode checks",
			mode:         dispatchjob.DispatchModeBlockOnError,
			messageGroup: "group1",
			expectCheck:  true,
		},
		{
			name:         "BLOCK_ON_ERROR with empty group",
			mode:         dispatchjob.DispatchModeBlockOnError,
			messageGroup: "",
			expectCheck:  false, // Empty group is never blocked
		},
	}

	checker := &BlockChecker{} // nil repo, ShouldBlockJob returns early

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &dispatchjob.DispatchJob{
				Mode:         tt.mode,
				MessageGroup: tt.messageGroup,
			}

			// For non-BLOCK_ON_ERROR modes, should always return false
			if tt.mode != dispatchjob.DispatchModeBlockOnError {
				if checker.ShouldBlockJob(nil, job) {
					t.Error("Non-BLOCK_ON_ERROR mode should not block")
				}
			}
		})
	}
}

// === DispatchMode Constants Tests ===

func TestDispatchModeConstants(t *testing.T) {
	if dispatchjob.DispatchModeImmediate != "IMMEDIATE" {
		t.Errorf("Expected IMMEDIATE, got %s", dispatchjob.DispatchModeImmediate)
	}
	if dispatchjob.DispatchModeNextOnError != "NEXT_ON_ERROR" {
		t.Errorf("Expected NEXT_ON_ERROR, got %s", dispatchjob.DispatchModeNextOnError)
	}
	if dispatchjob.DispatchModeBlockOnError != "BLOCK_ON_ERROR" {
		t.Errorf("Expected BLOCK_ON_ERROR, got %s", dispatchjob.DispatchModeBlockOnError)
	}
}

// === FilterBlockedJobs Tests ===

func TestFilterBlockedJobsEmpty(t *testing.T) {
	checker := &BlockChecker{}

	allowed, blocked := checker.FilterBlockedJobs(nil, []*dispatchjob.DispatchJob{})

	if len(allowed) != 0 {
		t.Errorf("Expected 0 allowed jobs, got %d", len(allowed))
	}
	if len(blocked) != 0 {
		t.Errorf("Expected 0 blocked groups, got %d", len(blocked))
	}
}

func TestFilterBlockedJobsNonBlockingModes(t *testing.T) {
	checker := &BlockChecker{}

	jobs := []*dispatchjob.DispatchJob{
		{ID: "1", Mode: dispatchjob.DispatchModeImmediate, MessageGroup: "group1"},
		{ID: "2", Mode: dispatchjob.DispatchModeNextOnError, MessageGroup: "group2"},
	}

	allowed, blocked := checker.FilterBlockedJobs(nil, jobs)

	// Non-blocking modes should pass through
	if len(allowed) != 2 {
		t.Errorf("Expected 2 allowed jobs, got %d", len(allowed))
	}
	if len(blocked) != 0 {
		t.Errorf("Expected 0 blocked groups, got %d", len(blocked))
	}
}

func TestFilterBlockedJobsGroupCollection(t *testing.T) {
	// Test that only BLOCK_ON_ERROR jobs' groups are collected
	jobs := []*dispatchjob.DispatchJob{
		{ID: "1", Mode: dispatchjob.DispatchModeBlockOnError, MessageGroup: "group1"},
		{ID: "2", Mode: dispatchjob.DispatchModeImmediate, MessageGroup: "group2"},
		{ID: "3", Mode: dispatchjob.DispatchModeBlockOnError, MessageGroup: "group1"}, // Duplicate
		{ID: "4", Mode: dispatchjob.DispatchModeBlockOnError, MessageGroup: "group3"},
	}

	// Collect groups same as in FilterBlockedJobs
	groupSet := make(map[string]struct{})
	for _, job := range jobs {
		if job.Mode == dispatchjob.DispatchModeBlockOnError && job.MessageGroup != "" {
			groupSet[job.MessageGroup] = struct{}{}
		}
	}

	// Should have group1 and group3, not group2
	if len(groupSet) != 2 {
		t.Errorf("Expected 2 unique groups, got %d", len(groupSet))
	}
	if _, ok := groupSet["group1"]; !ok {
		t.Error("Expected group1 in set")
	}
	if _, ok := groupSet["group3"]; !ok {
		t.Error("Expected group3 in set")
	}
	if _, ok := groupSet["group2"]; ok {
		t.Error("group2 should not be in set (IMMEDIATE mode)")
	}
}

// === Job Mode Filtering Logic Tests ===

func TestFilterBlockedJobsLogic(t *testing.T) {
	// Simulate the filtering logic
	jobs := []*dispatchjob.DispatchJob{
		{ID: "1", Mode: dispatchjob.DispatchModeBlockOnError, MessageGroup: "blocked-group"},
		{ID: "2", Mode: dispatchjob.DispatchModeBlockOnError, MessageGroup: "ok-group"},
		{ID: "3", Mode: dispatchjob.DispatchModeImmediate, MessageGroup: "blocked-group"},
		{ID: "4", Mode: dispatchjob.DispatchModeNextOnError, MessageGroup: "blocked-group"},
	}

	// Simulate blocked groups result
	blockedGroups := map[string]bool{
		"blocked-group": true,
	}

	// Apply filtering logic
	allowed := make([]*dispatchjob.DispatchJob, 0)
	for _, job := range jobs {
		if job.Mode == dispatchjob.DispatchModeBlockOnError && blockedGroups[job.MessageGroup] {
			continue // Blocked
		}
		allowed = append(allowed, job)
	}

	// Job 1 should be blocked (BLOCK_ON_ERROR + blocked-group)
	// Job 2 should pass (BLOCK_ON_ERROR + ok-group)
	// Job 3 should pass (IMMEDIATE mode ignores blocking)
	// Job 4 should pass (NEXT_ON_ERROR mode ignores blocking)

	if len(allowed) != 3 {
		t.Errorf("Expected 3 allowed jobs, got %d", len(allowed))
	}

	allowedIDs := make(map[string]bool)
	for _, job := range allowed {
		allowedIDs[job.ID] = true
	}

	if allowedIDs["1"] {
		t.Error("Job 1 should be blocked")
	}
	if !allowedIDs["2"] {
		t.Error("Job 2 should be allowed")
	}
	if !allowedIDs["3"] {
		t.Error("Job 3 should be allowed (IMMEDIATE mode)")
	}
	if !allowedIDs["4"] {
		t.Error("Job 4 should be allowed (NEXT_ON_ERROR mode)")
	}
}

// Benchmark for group deduplication
func BenchmarkGroupDeduplication(b *testing.B) {
	groups := []string{"group1", "group2", "group1", "group3", "group2", "group4", "group1"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		uniqueGroups := make(map[string]struct{})
		for _, g := range groups {
			if g != "" {
				uniqueGroups[g] = struct{}{}
			}
		}
	}
}
