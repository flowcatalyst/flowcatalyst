package operations

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
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
	// Eligible is how many of them ResendDispatchJobs would reset for this
	// caller — its tenant rule (auth.CanAccessScope) applied to each job's
	// client. ResendDispatchJobs resets a job in any status, so status does
	// not make a job ineligible.
	Eligible int64
}

// SummarizeMatches counts the jobs f matches for the caller in ctx.
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
		if auth.CanAccessScope(ac, b.ClientID) {
			s.Eligible += b.Count
		}
	}
	return s, nil
}

// ResendByFilterResult is what a requeue-by-filter run did.
type ResendByFilterResult struct {
	// Requeued is how many jobs ResendDispatchJobs actually reset.
	Requeued int64
	// Remaining is how many eligible jobs still match after the run stopped
	// at its cap; 0 when the walk ran out of matches.
	Remaining int64
}

// ResendByFilter resets the jobs f matches, at most min(limit, confirmCount)
// of them. confirmCount is the eligible count the caller was shown: if more
// eligible jobs match now, nothing is reset and the error is a 409
// FILTER_MATCH_CHANGED carrying the new counts, so the caller can ask again.
// It never resets more jobs than the caller confirmed, even when jobs start
// matching while it runs.
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

	now, err := SummarizeMatches(ctx, repo, f)
	if err != nil {
		return ResendByFilterResult{}, err
	}
	if now.Eligible > confirmCount {
		return ResendByFilterResult{}, usecase.Conflict("FILTER_MATCH_CHANGED",
			fmt.Sprintf("%d jobs now match this filter and can be requeued, not the %d confirmed; review the new count and confirm again",
				now.Eligible, confirmCount)).
			WithDetails(map[string]any{"count": now.Count, "eligibleCount": now.Eligible, "byStatus": now.ByStatus})
	}

	ac := auth.FromContext(ctx)
	walk := VisibleTo(ac, f)
	// Only walk jobs the use case would reset, so every id in a batch counts
	// towards the cap. Its tenant rule is still applied to each batch.
	walk.ExcludePlatformScoped = !auth.CanAccessScope(ac, nil)

	budget := min(int64(limit), confirmCount)
	var res ResendByFilterResult
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
		ev, err := usecaseop.Run(ctx, uow, ResendDispatchJobs(repo), ResendCommand{IDs: ids}, ec)
		if err != nil {
			return res, interrupted(res, err)
		}
		res.Requeued += int64(len(ev.IDs))
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
	return res, nil
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
