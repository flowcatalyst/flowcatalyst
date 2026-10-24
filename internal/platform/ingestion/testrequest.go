package ingestion

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/repocommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// TestRequestTTL is how long a "test connection" request stays open for a
// runtime to pick up and answer; after it the request reads as EXPIRED.
const TestRequestTTL = 2 * time.Minute

// Statuses of a test request. EXPIRED is not stored: an open request past its
// expiry reads as EXPIRED (EffectiveStatus), so no sweep has to move it.
const (
	TestPending = "PENDING"
	TestRunning = "RUNNING"
	TestDone    = "DONE"
	TestExpired = "EXPIRED"
)

// TestOutcomes are the answers a runtime gives, as the contract names them.
var TestOutcomes = []string{"RESULTS", "NO_RESULTS", "REFUSED", "FAILED", "NOT_SUPPORTED"}

// TestResult is what a runtime found when it tried the source once: it logged
// in and asked as the route would, from its own network and with its own
// secrets, and dispatched nothing. No secret or token is in it.
type TestResult struct {
	Outcome string   `json:"outcome"`
	Steps   []string `json:"steps"`
	// Records is how many records the split makes of the answer, when it answered.
	Records *int64 `json:"records,omitempty"`
	// Excerpt is the start of the answer, only for an ingestion that keeps
	// failed payloads.
	Excerpt *string `json:"excerpt,omitempty"`
}

// TestRequest is a "test connection" request: queued by an operator, pulled by
// a runtime that carries the kind, answered through the runtime's report.
type TestRequest struct {
	ID string
	// IngestionID is set when a saved ingestion is tested.
	IngestionID   *string
	ApplicationID ids.ApplicationID
	ClientID      ids.ClientID
	// Kind is the kind a runtime must carry (RunKind).
	Kind       string
	Definition Definition
	Status     string
	ClaimedBy  *string
	Result     *TestResult
	CreatedAt  time.Time
	ExpiresAt  time.Time
	// CompletedAt is set once the request is DONE.
	CompletedAt *time.Time

	// Names a claiming poll fills in so the runtime can address the secrets
	// and label the test; empty when the request was read by id.
	ApplicationCode, ClientIdentifier, IngestionCode string
}

// EffectiveStatus is the status a reader sees: an open request past its expiry
// is EXPIRED.
func (t TestRequest) EffectiveStatus(now time.Time) string {
	if (t.Status == TestPending || t.Status == TestRunning) && !now.Before(t.ExpiresAt) {
		return TestExpired
	}
	return t.Status
}

// QueueTest queues a request to try def's source, for a runtime that carries
// its kind to pick up at its next poll. ingestionID names the saved ingestion
// being tested, nil for a definition not yet saved.
func (r *Repository) QueueTest(ctx context.Context, ingestionID *string, applicationID ids.ApplicationID, clientID ids.ClientID, def Definition, requestedBy *string, now time.Time) (*TestRequest, error) {
	body, err := json.Marshal(def)
	if err != nil {
		return nil, fmt.Errorf("ingestion repo: marshal test definition: %w", err)
	}
	t := &TestRequest{
		ID: tsid.Generate(tsid.IngestionTest), IngestionID: ingestionID, ApplicationID: applicationID, ClientID: clientID,
		Kind: RunKind(def.Source), Definition: def, Status: TestPending, CreatedAt: now, ExpiresAt: now.Add(TestRequestTTL),
	}
	if err := r.q.IngestionTestInsert(ctx, dbq.IngestionTestInsertParams{
		ID: t.ID, IngestionID: ingestionID, ApplicationID: string(applicationID), ClientID: string(clientID), Kind: t.Kind,
		Definition: body, RequestedBy: requestedBy, CreatedAt: t.CreatedAt, ExpiresAt: t.ExpiresAt,
	}); err != nil {
		return nil, fmt.Errorf("ingestion repo: queue test: %w", err)
	}
	return t, nil
}

// FindTest loads a request, nil when there is none.
func (r *Repository) FindTest(ctx context.Context, id string) (*TestRequest, error) {
	res, err := r.q.IngestionTestFind(ctx, id)
	row, err := repocommon.One(res, err, "ingestion repo")
	if row == nil || err != nil {
		return nil, err
	}
	return rowToTest(*row)
}

// claimTests takes the pending requests for the kinds the poller carries and
// is allowed to reach. SKIP LOCKED in the query lets runtimes polling together
// take different requests, never the same one.
func claimTests(ctx context.Context, q *dbq.Queries, req PollRequest, carried []string) ([]TestRequest, error) {
	p := dbq.IngestionTestClaimParams{RuntimeID: &req.RuntimeID, Now: req.Now, Kinds: carried, Lim: maxTestsPerPoll}
	if req.Clients != nil {
		p.Clients = *req.Clients
	}
	if req.Applications != nil {
		p.Applications = *req.Applications
	}
	rows, err := q.IngestionTestClaim(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("claim tests: %w", err)
	}
	out := make([]TestRequest, 0, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	claimed := make([]string, len(rows))
	for i, row := range rows {
		claimed[i] = row.ID
	}
	refs, err := q.IngestionTestRefs(ctx, claimed)
	if err != nil {
		return nil, fmt.Errorf("test refs: %w", err)
	}
	byID := make(map[string]dbq.IngestionTestRefsRow, len(refs))
	for _, ref := range refs {
		byID[ref.ID] = ref
	}
	for _, row := range rows {
		t, err := rowToTest(row)
		if err != nil {
			continue // logged by rowToTest; the request expires unanswered
		}
		ref := byID[row.ID]
		t.ApplicationCode, t.ClientIdentifier = ref.ApplicationCode, ref.ClientIdentifier
		if ref.IngestionCode != nil {
			t.IngestionCode = *ref.IngestionCode
		}
		out = append(out, *t)
	}
	return out, nil
}

// completeTest records a runtime's answer; false when the request is not open
// for it (not claimed by it, answered already, or expired).
func completeTest(ctx context.Context, q *dbq.Queries, id, runtimeID string, result TestResult, now time.Time) (bool, error) {
	body, err := json.Marshal(result)
	if err != nil {
		return false, fmt.Errorf("marshal test result: %w", err)
	}
	n, err := q.IngestionTestComplete(ctx, dbq.IngestionTestCompleteParams{Result: body, Now: &now, ID: id, RuntimeID: &runtimeID})
	if err != nil {
		return false, fmt.Errorf("complete test: %w", err)
	}
	return n == 1, nil
}

func rowToTest(row dbq.IngTestRequest) (*TestRequest, error) {
	var def Definition
	if err := json.Unmarshal(row.Definition, &def); err != nil {
		slog.Error("ingestion test request does not parse", "id", row.ID, "err", err)
		return nil, fmt.Errorf("ingestion repo: test request %s does not parse: %w", row.ID, err)
	}
	t := &TestRequest{
		ID: row.ID, IngestionID: row.IngestionID, ApplicationID: ids.ApplicationID(row.ApplicationID), ClientID: ids.ClientID(row.ClientID),
		Kind: row.Kind, Definition: def, Status: row.Status, ClaimedBy: row.ClaimedBy,
		CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt, CompletedAt: row.CompletedAt,
	}
	if len(row.Result) > 0 {
		var res TestResult
		if err := json.Unmarshal(row.Result, &res); err != nil {
			slog.Error("ingestion test result does not parse", "id", row.ID, "err", err)
			return nil, fmt.Errorf("ingestion repo: test result %s does not parse: %w", row.ID, err)
		}
		t.Result = &res
	}
	return t, nil
}
