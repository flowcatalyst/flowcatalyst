package ingestion

import (
	"context"
	"fmt"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/repocommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
)

// RouteState is what one runtime last reported for an ingestion: the version
// it runs, how the route is doing and, when it is not well, why.
type RouteState struct {
	RuntimeID string
	Version   int
	// Status is STARTED, FAILING or FAILED.
	Status    string
	Error     *string
	UpdatedAt time.Time
}

// ListRouteStates returns the ingestion's route states, one per runtime that
// has reported it, by runtime id.
func (r *Repository) ListRouteStates(ctx context.Context, ingestionID string) ([]RouteState, error) {
	rows, err := r.q.IngestionRouteStateList(ctx, ingestionID)
	if err != nil {
		return nil, fmt.Errorf("ingestion repo: list route states: %w", err)
	}
	out := make([]RouteState, 0, len(rows))
	for _, row := range rows {
		out = append(out, RouteState{
			RuntimeID: row.RuntimeID, Version: int(row.Version), Status: row.Status, Error: row.Error, UpdatedAt: row.UpdatedAt,
		})
	}
	return out, nil
}

// Exchanges counts what the runtimes reported over a period, all runtimes
// together.
type Exchanges struct {
	Completed, Failed, Records, Jobs int64
	// Polls is how many polls the runtimes reported finishing, empty ones
	// included.
	Polls int64
}

// Activity is an ingestion's exchanges over the last day and the last week.
// LastExchange is the start of the newest window with an exchange in the week,
// LastPoll the end of the newest poll a runtime reported in the week (so a
// polled source that found nothing still shows as polling), LastFailure the
// newest failure kept (nil: none).
type Activity struct {
	Day, Week    Exchanges
	LastExchange *time.Time
	LastPoll     *time.Time
	LastFailure  *time.Time
}

// ActivityOf reads the ingestion's activity as of now.
func (r *Repository) ActivityOf(ctx context.Context, ingestionID string, now time.Time) (*Activity, error) {
	since := now.Add(-7 * 24 * time.Hour)
	t, err := r.q.IngestionExchangeTotals(ctx, dbq.IngestionExchangeTotalsParams{
		IngestionID: ingestionID, SinceDay: now.Add(-24 * time.Hour), SinceWeek: since,
	})
	if err != nil {
		return nil, fmt.Errorf("ingestion repo: exchange totals: %w", err)
	}
	a := &Activity{
		Day:  Exchanges{Completed: t.CompletedDay, Failed: t.FailedDay, Records: t.RecordsDay, Jobs: t.JobsDay, Polls: t.PollsDay},
		Week: Exchanges{Completed: t.CompletedWeek, Failed: t.FailedWeek, Records: t.RecordsWeek, Jobs: t.JobsWeek, Polls: t.PollsWeek},
	}
	last, err := r.q.IngestionLastExchange(ctx, dbq.IngestionLastExchangeParams{IngestionID: ingestionID, WindowStart: since})
	if a.LastExchange, err = repocommon.One(last, err, "ingestion repo"); err != nil {
		return nil, err
	}
	polled, err := r.q.IngestionLastPoll(ctx, dbq.IngestionLastPollParams{IngestionID: ingestionID, WindowStart: since})
	if a.LastPoll, err = repocommon.One(polled, err, "ingestion repo"); err != nil {
		return nil, err
	}
	failed, err := r.q.IngestionLastFailure(ctx, ingestionID)
	if a.LastFailure, err = repocommon.One(failed, err, "ingestion repo"); err != nil {
		return nil, err
	}
	return a, nil
}
