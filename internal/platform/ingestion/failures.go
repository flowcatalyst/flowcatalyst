package ingestion

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// Failure categories (ing_failures.category). A payload failure is a record
// or payload the runtime could not take (what every failure meant before the
// category existed). A rejected failure is a notice: the platform refused the
// runtime's jobs request permanently (a target that no longer resolves, a
// paused connection's account gone), so nothing more will be made until
// someone acts; it shows on the ingestion until it is dismissed.
const (
	FailurePayload  = "payload"
	FailureRejected = "rejected"
)

// ValidFailureCategory reports whether c is a category a runtime may report.
func ValidFailureCategory(c string) bool { return c == FailurePayload || c == FailureRejected }

// Failure is one failure a runtime reported for an ingestion.
type Failure struct {
	ID           string    `json:"id"`
	IngestionID  string    `json:"ingestionId"`
	RuntimeID    *string   `json:"runtimeId,omitempty"`
	Version      *int      `json:"version,omitempty"`
	Kind         string    `json:"kind"`
	Category     string    `json:"category"`
	Message      string    `json:"message"`
	AfterRecords int64     `json:"afterRecords"`
	Excerpt      *string   `json:"excerpt,omitempty"`
	OccurredAt   time.Time `json:"occurredAt"`
	// CreatedAt is when the platform recorded it (retention counts from it).
	CreatedAt   time.Time  `json:"createdAt"`
	DismissedAt *time.Time `json:"dismissedAt,omitempty"`
	DismissedBy *string    `json:"dismissedBy,omitempty"`
}

// IDStr satisfies usecase.HasID.
func (f Failure) IDStr() string { return f.ID }

// Dismissed reports whether someone has dismissed the failure.
func (f Failure) Dismissed() bool { return f.DismissedAt != nil }

// Notice reports whether the failure is a notice that still shows: a
// rejected one nobody has dismissed.
func (f Failure) Notice() bool { return f.Category == FailureRejected && !f.Dismissed() }

// Dismiss records who dismissed the failure and when.
func (f *Failure) Dismiss(by string, at time.Time) {
	f.DismissedAt, f.DismissedBy = &at, &by
}

// FailurePage is one page of failures, newest first. Next is empty on the
// last page.
type FailurePage struct {
	Items []Failure
	Next  string
}

// FailureFilter narrows a failure list. The zero value is every failure.
type FailureFilter struct {
	// Undismissed keeps only the failures nobody has dismissed.
	Undismissed bool
	// Category keeps one category (payload or rejected); "" is both.
	Category string
	// After is the Next of the previous page.
	After string
	// Limit is 50 when zero, at most 200.
	Limit int
}

// ListFailures pages an ingestion's failures by id, newest first; after is the
// Next of the previous page. limit is 50 when zero, at most 200.
func (r *Repository) ListFailures(ctx context.Context, ingestionID, after string, limit int) (*FailurePage, error) {
	return r.ListFailuresFiltered(ctx, ingestionID, FailureFilter{After: after, Limit: limit})
}

// ListFailuresFiltered is ListFailures narrowed by f.
func (r *Repository) ListFailuresFiltered(ctx context.Context, ingestionID string, f FailureFilter) (*FailurePage, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, 200)
	// One more than the page, to know whether there is a next one.
	rows, err := r.q.IngestionFailureList(ctx, dbq.IngestionFailureListParams{
		IngestionID: ingestionID, After: f.After, Undismissed: f.Undismissed, Category: f.Category, Lim: int32(limit + 1),
	})
	if err != nil {
		return nil, fmt.Errorf("ingestion repo: list failures: %w", err)
	}
	page := &FailurePage{Items: make([]Failure, 0, min(len(rows), limit))}
	for i, row := range rows {
		if i == limit {
			page.Next = rows[limit-1].ID
			break
		}
		page.Items = append(page.Items, rowToFailure(row))
	}
	return page, nil
}

// FindFailure loads one of the ingestion's failures, nil when it has none by
// that id.
func (r *Repository) FindFailure(ctx context.Context, ingestionID, id string) (*Failure, error) {
	row, err := r.q.IngestionFailureFind(ctx, dbq.IngestionFailureFindParams{IngestionID: ingestionID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ingestion repo: find failure: %w", err)
	}
	f := rowToFailure(row)
	return &f, nil
}

func rowToFailure(row dbq.IngFailure) Failure {
	f := Failure{
		ID: row.ID, IngestionID: row.IngestionID, RuntimeID: row.RuntimeID, Kind: row.Kind, Category: row.Category,
		Message: row.Message, AfterRecords: row.AfterRecords, Excerpt: row.Excerpt, OccurredAt: row.OccurredAt,
		CreatedAt: row.CreatedAt, DismissedAt: row.DismissedAt, DismissedBy: row.DismissedBy,
	}
	if row.Version != nil {
		v := int(*row.Version)
		f.Version = &v
	}
	return f
}

// Notices is what an ingestion's undismissed notices add up to: how many,
// and the newest one's message.
type Notices struct {
	Count         int
	LatestMessage string
	LatestAt      time.Time
}

// NoticesOf reads the undismissed notices of the ingestions named, keyed by
// ingestion id; an ingestion without one is absent.
func (r *Repository) NoticesOf(ctx context.Context, ids []string) (map[string]Notices, error) {
	out := make(map[string]Notices, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.q.IngestionNotices(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("ingestion repo: notices: %w", err)
	}
	for _, row := range rows {
		out[row.IngestionID] = Notices{Count: int(row.Notices), LatestMessage: row.LatestMessage, LatestAt: row.LatestAt}
	}
	return out, nil
}

// FailureStore persists a failure's dismissal for the use-case envelope
// (usecasepgx.Persist[Failure]). Failures are written by the runtime's report
// and removed by retention, so dismissal is the only change it makes.
type FailureStore struct{ q *dbq.Queries }

// Failures returns the store of failure dismissals.
func (r *Repository) Failures() *FailureStore { return &FailureStore{q: r.q} }

// Persist writes the dismissal. A failure someone else dismissed since it was
// read is a conflict: one dismissal, one event.
func (s *FailureStore) Persist(ctx context.Context, f *Failure, tx *usecasepgx.DbTx) error {
	if f.DismissedAt == nil || f.DismissedBy == nil {
		return errors.New("ingestion repo: only a dismissal is persisted")
	}
	n, err := s.q.WithTx(tx.Inner()).IngestionFailureDismiss(ctx, dbq.IngestionFailureDismissParams{
		IngestionID: f.IngestionID, ID: f.ID, DismissedAt: f.DismissedAt, DismissedBy: f.DismissedBy,
	})
	if err != nil {
		return fmt.Errorf("ingestion repo: dismiss failure: %w", err)
	}
	if n == 0 {
		return usecase.Conflict("INGESTION_FAILURE_DISMISSED", "the failure was dismissed already")
	}
	return nil
}

// Delete is not a thing a caller does to a failure: retention removes them.
func (s *FailureStore) Delete(context.Context, *Failure, *usecasepgx.DbTx) error {
	return errors.New("ingestion repo: failures are removed by retention only")
}

// StripNUL removes the NUL characters a runtime may pass through from a
// payload: Postgres refuses them in text and in JSONB, and one would make the
// whole report fail.
func StripNUL(s string) string {
	if !strings.ContainsRune(s, 0) {
		return s
	}
	return strings.ReplaceAll(s, "\x00", "")
}

// StripNULPtr is StripNUL for an optional value.
func StripNULPtr(s *string) *string {
	if s == nil || !strings.ContainsRune(*s, 0) {
		return s
	}
	v := StripNUL(*s)
	return &v
}
