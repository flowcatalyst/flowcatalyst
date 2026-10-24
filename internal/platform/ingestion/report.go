package ingestion

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// RouteReport is the state of one ingestion's route on a runtime.
type RouteReport struct {
	IngestionID string
	// Version is the version the runtime built.
	Version int
	// Status is STARTED, FAILING (running, but the source keeps failing) or FAILED.
	Status string
	Error  *string
}

// WindowReport is the exchange counts of one ingestion over a window.
type WindowReport struct {
	IngestionID                          string
	From, To                             time.Time
	Completed, Failed, Records, Dispatch int64
}

// FailureReport is one thing that went wrong with an ingestion.
type FailureReport struct {
	IngestionID string
	Version     int
	At          time.Time
	// Kind is REJECTED, NOT_DISPATCHED, SOURCE or AUTH.
	Kind    string
	Message string
	// AfterRecords is how many of the payload's records were read before it failed.
	AfterRecords *int64
	// Excerpt is the start of what it failed on; kept only for an ingestion
	// that keeps failed payloads.
	Excerpt *string
}

// TestAnswer is a runtime's answer to a test request it claimed.
type TestAnswer struct {
	TestID string
	Result TestResult
}

// Report is what a runtime sends back in one batch.
type Report struct {
	RuntimeID string
	Routes    []RouteReport
	Metrics   []WindowReport
	Failures  []FailureReport
	Tests     []TestAnswer
	// Caller bounds the report to the ingestions the runtime's service account reaches.
	Caller *auth.AuthContext
	Now    time.Time
}

// ReportSummary counts the entries of a report that were recorded and those
// that were ignored.
type ReportSummary struct{ Accepted, Ignored int }

// Record stores a runtime's report. An entry about an ingestion that does not
// exist, that the caller does not reach, or whose polled source the runtime
// does not hold the lease of, is ignored: a runtime that lost its lease may
// still be reporting the batch it finished. Shared sources have no lease and
// are recorded. A window is idempotent per (ingestion, runtime, window start):
// sent again it replaces the counts and never adds to them.
func (r *Repository) Record(ctx context.Context, rep Report) (ReportSummary, error) {
	var sum ReportSummary
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)
		accepted, err := r.acceptable(ctx, q, rep)
		if err != nil {
			return err
		}
		ok := func(id string) (*Ingestion, bool) {
			ing, found := accepted[id]
			if found {
				sum.Accepted++
			} else {
				sum.Ignored++
			}
			return ing, found
		}
		for _, route := range rep.Routes {
			if _, found := ok(route.IngestionID); !found {
				continue
			}
			if err := q.IngestionRouteStateUpsert(ctx, dbq.IngestionRouteStateUpsertParams{
				IngestionID: route.IngestionID, RuntimeID: rep.RuntimeID, Version: int32(route.Version),
				Status: route.Status, Error: route.Error, UpdatedAt: rep.Now,
			}); err != nil {
				return fmt.Errorf("route state: %w", err)
			}
		}
		for _, w := range rep.Metrics {
			if _, found := ok(w.IngestionID); !found {
				continue
			}
			if err := q.IngestionMetricUpsert(ctx, dbq.IngestionMetricUpsertParams{
				IngestionID: w.IngestionID, RuntimeID: rep.RuntimeID, WindowStart: w.From,
				WindowSecs: int32(w.To.Sub(w.From) / time.Second),
				Completed:  w.Completed, Failed: w.Failed, Records: w.Records, Jobs: w.Dispatch,
			}); err != nil {
				return fmt.Errorf("exchange metric: %w", err)
			}
		}
		for _, f := range rep.Failures {
			ing, found := ok(f.IngestionID)
			if !found {
				continue
			}
			// A payload is kept only for an ingestion whose client has agreed.
			excerpt := f.Excerpt
			if !ing.Definition.KeepFailedPayloads {
				excerpt = nil
			}
			var after int64
			if f.AfterRecords != nil {
				after = *f.AfterRecords
			}
			version := int32(f.Version)
			if err := q.IngestionFailureInsert(ctx, dbq.IngestionFailureInsertParams{
				ID: tsid.Generate(tsid.IngestionFailure), IngestionID: f.IngestionID, RuntimeID: &rep.RuntimeID, Version: &version,
				Kind: f.Kind, Message: f.Message, AfterRecords: after, Excerpt: excerpt, OccurredAt: f.At, CreatedAt: rep.Now,
			}); err != nil {
				return fmt.Errorf("failure: %w", err)
			}
		}
		for _, a := range rep.Tests {
			if err := answerTest(ctx, q, rep, a); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return ReportSummary{}, fmt.Errorf("ingestion repo: record report: %w", err)
	}
	return sum, nil
}

// acceptable loads the ingestions a report names and keeps those it may be
// recorded for.
func (r *Repository) acceptable(ctx context.Context, q *dbq.Queries, rep Report) (map[string]*Ingestion, error) {
	seen := map[string]bool{}
	var named []string
	add := func(id string) {
		if !seen[id] {
			seen[id] = true
			named = append(named, id)
		}
	}
	for _, x := range rep.Routes {
		add(x.IngestionID)
	}
	for _, x := range rep.Metrics {
		add(x.IngestionID)
	}
	for _, x := range rep.Failures {
		add(x.IngestionID)
	}
	out := make(map[string]*Ingestion, len(named))
	if len(named) == 0 {
		return out, nil
	}
	rows, err := q.IngestionFindByIDs(ctx, named)
	if err != nil {
		return nil, fmt.Errorf("find ingestions: %w", err)
	}
	for _, row := range rows {
		ing, err := rowToIngestion(row)
		if err != nil {
			continue // logged by rowToIngestion
		}
		if CheckReach(rep.Caller, ing) != nil {
			continue
		}
		if ing.Definition.Source.SingleOwner() {
			held, err := q.IngestionLeaseHeld(ctx, dbq.IngestionLeaseHeldParams{IngestionID: ing.ID, RuntimeID: rep.RuntimeID, Now: rep.Now})
			if err != nil {
				return nil, fmt.Errorf("lease held: %w", err)
			}
			if !held {
				continue
			}
		}
		out[ing.ID] = ing
	}
	return out, nil
}

// answerTest stores a runtime's answer to a test request it claimed. An answer
// for a request it did not claim, or that expired, or that is answered
// already, is dropped. The excerpt of the answer is kept only if the tested
// definition keeps failed payloads.
func answerTest(ctx context.Context, q *dbq.Queries, rep Report, a TestAnswer) error {
	res, err := q.IngestionTestFind(ctx, a.TestID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("find test: %w", err)
	}
	t, err := rowToTest(res)
	if err != nil {
		return nil // logged by rowToTest; nothing to answer
	}
	if !t.Definition.KeepFailedPayloads {
		a.Result.Excerpt = nil
	}
	_, err = completeTest(ctx, q, a.TestID, rep.RuntimeID, a.Result, rep.Now)
	return err
}
