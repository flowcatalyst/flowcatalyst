package operations

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// Requeue-by-filter: reset every job a list filter matches, not just the ids a
// caller has in hand. It is not a use case of its own. It walks the matching
// ids and hands them, a batch at a time, to ResendDispatchJobs — so each batch
// is one transaction with that use case's own tenant check, event and audit
// row, exactly as if the caller had sent the ids to POST /requeue.
const (
	// ResendBatchSize is how many ids go to one ResendDispatchJobs run.
	ResendBatchSize = 500
	// ResendByFilterMaxLimit caps one request; it is also the default.
	ResendByFilterMaxLimit = 10000
)

// VisibleTo scopes a filter to what the caller may see: anchors see every
// job; anyone else sees platform-scoped jobs plus their own tenants'. The
// caller's own clientId filters can only narrow within that. Shared by the
// list and requeue-by-filter so both see the same rows.
func VisibleTo(ac *auth.AuthContext, f dispatchjob.FilterParams) dispatchjob.FilterParams {
	if !ac.IsAnchor() {
		clients := ac.Clients
		f.AccessibleClientIDs = &clients
	}
	return f
}

// MatchSummary describes the jobs a filter matches for one caller.
type MatchSummary struct {
	// Count is every job the filter matches that the caller can see.
	Count int64
	// ByStatus breaks Count down by status.
	ByStatus map[string]int64
	// Eligible is how many of them a requeue would reset for this caller: its
	// tenant rule (auth.CanAccessScope) applied to each job's client, and the
	// status guard (dispatchjob.RequeueableStatuses) — jobs in flight and
	// COMPLETED jobs are not eligible. Send it back as confirmCount.
	Eligible int64
	// Completed is how many COMPLETED jobs the caller can see in the match.
	// They are redriven only with the two redrive fields; a redrive's
	// confirmCount is Eligible + Completed.
	Completed int64
	// InFlight is how many QUEUED or PROCESSING jobs the caller can see in the
	// match. A requeue never resets them.
	InFlight int64
}

// RedriveEligible is the count a confirmed redrive must confirm: the eligible
// jobs plus the COMPLETED ones.
func (s MatchSummary) RedriveEligible() int64 { return s.Eligible + s.Completed }

// SummarizeMatches counts the jobs f matches for the caller in ctx. f must
// already carry its time window (dispatchjob.EnforceWindow): the count scans
// the window's partitions and nothing more.
func SummarizeMatches(ctx context.Context, repo *dispatchjob.Repository, f dispatchjob.FilterParams) (MatchSummary, error) {
	ac := auth.FromContext(ctx)
	buckets, err := repo.CountMatches(ctx, VisibleTo(ac, f))
	if err != nil {
		return MatchSummary{}, usecase.Internal("REPO", "count_matches failed", err)
	}
	s := MatchSummary{ByStatus: map[string]int64{}}
	for _, b := range buckets {
		s.Count += b.Count
		s.ByStatus[b.Status] += b.Count
		if !auth.CanAccessScope(ac, b.ClientID) {
			continue
		}
		st, _ := common.ParseDispatchStatus(b.Status)
		switch dispatchjob.ClassifyForRequeue(st) {
		case dispatchjob.VerdictRequeue:
			s.Eligible += b.Count
		case dispatchjob.VerdictCompleted:
			s.Completed += b.Count
		case dispatchjob.VerdictInFlight:
			s.InFlight += b.Count
		}
	}
	return s, nil
}

// ResendByFilterResult is what a requeue-by-filter run did.
type ResendByFilterResult struct {
	// Requeued is how many jobs were actually reset (Redriven included).
	Requeued int64
	// Redriven is how many of them were COMPLETED jobs re-delivered.
	Redriven int64
	// Remaining is how many eligible jobs still match after the run stopped
	// at its cap; 0 when the walk ran out of matches.
	Remaining int64
	// SkippedInFlight and SkippedCompleted are the matching jobs the status
	// guard left alone: the QUEUED/PROCESSING matches (and, for a run that was
	// not a confirmed redrive, the COMPLETED ones) counted when the run
	// started, plus any job the walk handed over that had changed status by
	// the time its batch ran.
	SkippedInFlight  int64
	SkippedCompleted int64
}

// ResendByFilterOptions are the redrive fields of a requeue-by-filter request.
type ResendByFilterOptions struct {
	RedriveCompleted        bool
	ConfirmRedriveCompleted string
}

// ResendByFilter resets the jobs f matches, at most min(limit, confirmCount)
// of them. f must already carry its time window (dispatchjob.EnforceWindow).
//
// Status guard: jobs QUEUED or PROCESSING are never reset; COMPLETED jobs are
// reset only when opts confirm a redrive (both fields), in which case
// confirmCount covers Eligible + Completed. Without a confirmed redrive the
// COMPLETED matches are skipped and counted in the result.
//
// confirmCount is the count the caller was shown: if more jobs would be reset
// now, nothing is reset and the error is a 409 FILTER_MATCH_CHANGED carrying
// the new counts, so the caller can ask again. It never resets more jobs than
// the caller confirmed, even when jobs start matching while it runs.
//
// The walk pages the matching ids in id order (keyset, ResendBatchSize at a
// time) and runs ResendDispatchJobs on each page in its own transaction.
// Batches already committed stay committed if a later one fails.
func ResendByFilter(
	ctx context.Context,
	uow *usecasepgx.UnitOfWork,
	repo *dispatchjob.Repository,
	f dispatchjob.FilterParams,
	confirmCount int64,
	limit int,
	opts ResendByFilterOptions,
	ec usecase.ExecutionContext,
) (ResendByFilterResult, error) {
	if confirmCount < 0 {
		return ResendByFilterResult{}, usecase.Validation("INVALID_CONFIRM_COUNT", "confirmCount must not be negative")
	}
	if limit < 0 || limit > ResendByFilterMaxLimit {
		return ResendByFilterResult{}, usecase.Validation("INVALID_LIMIT",
			fmt.Sprintf("limit must be between 1 and %d", ResendByFilterMaxLimit))
	}
	if limit == 0 {
		limit = ResendByFilterMaxLimit
	}
	redrive := dispatchjob.RedriveConfirmed(opts.RedriveCompleted, opts.ConfirmRedriveCompleted)

	now, err := SummarizeMatches(ctx, repo, f)
	if err != nil {
		return ResendByFilterResult{}, err
	}
	toReset := now.Eligible
	if redrive {
		toReset = now.RedriveEligible()
	}
	if toReset > confirmCount {
		return ResendByFilterResult{}, usecase.Conflict("FILTER_MATCH_CHANGED",
			fmt.Sprintf("%d jobs now match this filter and can be requeued, not the %d confirmed; review the new count and confirm again",
				toReset, confirmCount)).
			WithDetails(map[string]any{
				"count": now.Count, "eligibleCount": now.Eligible, "completedCount": now.Completed,
				"inFlightCount": now.InFlight, "redriveEligibleCount": now.RedriveEligible(), "byStatus": now.ByStatus,
			})
	}

	ac := auth.FromContext(ctx)
	walk := VisibleTo(ac, f)
	// Only walk jobs the use case would reset, so every id in a batch counts
	// towards the cap. Its tenant rule is still applied to each batch.
	walk.ExcludePlatformScoped = !auth.CanAccessScope(ac, nil)
	walk.StatusIn = resettableStatuses(redrive)

	res := ResendByFilterResult{SkippedInFlight: now.InFlight}
	if !redrive {
		res.SkippedCompleted = now.Completed
	}
	budget := min(int64(limit), confirmCount)
	var handed int64
	exhausted := false
	for handed < budget {
		page := int(min(int64(ResendBatchSize), budget-handed))
		ids, err := repo.MatchingIDs(ctx, walk, page)
		if err != nil {
			return res, interrupted(res, usecase.Internal("REPO", "matching_ids failed", err))
		}
		if len(ids) == 0 {
			exhausted = true
			break
		}
		cmd := ResendCommand{IDs: ids}
		if redrive {
			cmd.RedriveCompleted, cmd.ConfirmRedriveCompleted = true, opts.ConfirmRedriveCompleted
		}
		ev, err := usecaseop.Run(ctx, uow, ResendDispatchJobs(repo), cmd, ec)
		if err != nil {
			return res, interrupted(res, err)
		}
		res.Requeued += int64(len(ev.IDs))
		res.Redriven += int64(len(ev.Redriven))
		for _, sk := range ev.Skipped {
			if sk.Reason == dispatchjob.SkipInFlight {
				res.SkippedInFlight++
			} else {
				res.SkippedCompleted++
			}
		}
		handed += int64(len(ids))
		last := ids[len(ids)-1]
		walk.AfterID = &last
		if len(ids) < page {
			exhausted = true
			break
		}
	}
	if exhausted || handed == 0 {
		return res, nil
	}
	// Stopped at the budget: count what is left after the last id handed out.
	rest := f
	rest.AfterID = walk.AfterID
	left, err := SummarizeMatches(ctx, repo, rest)
	if err != nil {
		return res, err
	}
	res.Remaining = left.Eligible
	if redrive {
		res.Remaining = left.RedriveEligible()
	}
	return res, nil
}

// resettableStatuses are the statuses the id walk hands to ResendDispatchJobs:
// what a requeue may reset, plus COMPLETED for a confirmed redrive.
func resettableStatuses(redrive bool) []string {
	out := make([]string, 0, len(dispatchjob.RequeueableStatuses)+1)
	for _, s := range dispatchjob.RequeueableStatuses {
		out = append(out, string(s))
	}
	if redrive {
		out = append(out, string(common.DispatchCompleted))
	}
	return out
}

// interrupted reports a failure part-way through: the batches before it are
// committed, so the caller is told how many jobs were already reset.
func interrupted(res ResendByFilterResult, err error) error {
	if res.Requeued == 0 {
		return err
	}
	slog.Error("requeue by filter stopped part-way", "requeued", res.Requeued, "err", err)
	return usecase.Internal("REQUEUE_INTERRUPTED",
		fmt.Sprintf("requeue stopped after %d jobs were reset; run it again to continue", res.Requeued), err).
		WithDetails(map[string]any{"requeued": res.Requeued})
}
