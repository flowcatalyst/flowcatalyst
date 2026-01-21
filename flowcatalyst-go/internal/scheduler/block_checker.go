// Package scheduler provides dispatch job scheduling
package scheduler

import (
	"context"

	"log/slog"
	"go.flowcatalyst.tech/internal/platform/dispatchjob"
)

// BlockChecker checks whether message groups should be blocked from dispatch
// due to existing ERROR status jobs (for BLOCK_ON_ERROR mode)
type BlockChecker struct {
	jobRepo dispatchjob.Repository
}

// NewBlockChecker creates a new block checker
func NewBlockChecker(jobRepo dispatchjob.Repository) *BlockChecker {
	return &BlockChecker{
		jobRepo: jobRepo,
	}
}

// IsGroupBlocked returns true if the message group has any ERROR status jobs
// This is used to implement BLOCK_ON_ERROR dispatch mode
func (c *BlockChecker) IsGroupBlocked(ctx context.Context, messageGroup string) bool {
	if messageGroup == "" {
		return false
	}

	blocked, err := c.jobRepo.HasErrorJobsInGroup(ctx, messageGroup)
	if err != nil {
		slog.Error("Failed to check if group is blocked", "error", err, "messageGroup", messageGroup)
		// On error, don't block - fail open to avoid stopping all dispatches
		return false
	}

	if blocked {
		slog.Debug("Message group is blocked due to ERROR jobs", "messageGroup", messageGroup)
	}

	return blocked
}

// GetBlockedGroups checks multiple message groups and returns a map
// of which ones are blocked (have ERROR status jobs)
func (c *BlockChecker) GetBlockedGroups(ctx context.Context, groups []string) map[string]bool {
	if len(groups) == 0 {
		return map[string]bool{}
	}

	// De-duplicate groups
	uniqueGroups := make(map[string]struct{})
	for _, g := range groups {
		if g != "" {
			uniqueGroups[g] = struct{}{}
		}
	}

	groupList := make([]string, 0, len(uniqueGroups))
	for g := range uniqueGroups {
		groupList = append(groupList, g)
	}

	if len(groupList) == 0 {
		return map[string]bool{}
	}

	blocked, err := c.jobRepo.GetBlockedMessageGroups(ctx, groupList)
	if err != nil {
		slog.Error("Failed to get blocked message groups", "error", err, "groupCount", len(groupList))
		// On error, return empty map - fail open
		return map[string]bool{}
	}

	if len(blocked) > 0 {
		slog.Debug("Found blocked message groups", "blockedCount", len(blocked), "totalGroups", len(groupList))
	}

	return blocked
}

// ShouldBlockJob determines if a job should be blocked based on its dispatch mode
// and whether its message group has errors
func (c *BlockChecker) ShouldBlockJob(ctx context.Context, job *dispatchjob.DispatchJob) bool {
	// Only BLOCK_ON_ERROR mode blocks on errors
	if job.Mode != dispatchjob.DispatchModeBlockOnError {
		return false
	}

	// Check if the message group has any ERROR jobs
	return c.IsGroupBlocked(ctx, job.MessageGroup)
}

// FilterBlockedJobs removes jobs that should be blocked from the list
// Returns the jobs that can be dispatched and a map of blocked groups
func (c *BlockChecker) FilterBlockedJobs(ctx context.Context, jobs []*dispatchjob.DispatchJob) ([]*dispatchjob.DispatchJob, map[string]bool) {
	if len(jobs) == 0 {
		return jobs, map[string]bool{}
	}

	// Collect unique message groups from BLOCK_ON_ERROR jobs
	blockOnErrorGroups := make([]string, 0)
	groupSet := make(map[string]struct{})

	for _, job := range jobs {
		if job.Mode == dispatchjob.DispatchModeBlockOnError && job.MessageGroup != "" {
			if _, exists := groupSet[job.MessageGroup]; !exists {
				groupSet[job.MessageGroup] = struct{}{}
				blockOnErrorGroups = append(blockOnErrorGroups, job.MessageGroup)
			}
		}
	}

	// Get blocked groups
	blockedGroups := c.GetBlockedGroups(ctx, blockOnErrorGroups)

	if len(blockedGroups) == 0 {
		// No blocked groups, return all jobs
		return jobs, blockedGroups
	}

	// Filter out blocked jobs
	allowed := make([]*dispatchjob.DispatchJob, 0, len(jobs))
	for _, job := range jobs {
		// Block if:
		// 1. Job has BLOCK_ON_ERROR mode AND
		// 2. Its message group is in the blocked list
		if job.Mode == dispatchjob.DispatchModeBlockOnError && blockedGroups[job.MessageGroup] {
			slog.Debug("Job blocked due to ERROR jobs in group", "jobId", job.ID, "messageGroup", job.MessageGroup)
			continue
		}
		allowed = append(allowed, job)
	}

	blockedCount := len(jobs) - len(allowed)
	if blockedCount > 0 {
		slog.Info("Filtered blocked jobs due to BLOCK_ON_ERROR mode", "blockedJobs", blockedCount, "allowedJobs", len(allowed), "blockedGroups", len(blockedGroups))
	}

	return allowed, blockedGroups
}
