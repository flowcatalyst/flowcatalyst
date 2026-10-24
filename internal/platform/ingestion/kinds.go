package ingestion

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
)

// RuntimeLiveWindow is how long after its last poll a runtime counts as live:
// runtimes poll every minute, so three missed polls.
const RuntimeLiveWindow = 3 * time.Minute

// RuntimeKind is a source kind a runtime reports (ing_runtimes.kinds): one
// built on an extension, or camel-route. The platform stores and lists it and
// enforces RequiredPermission; it knows nothing else about the kind.
type RuntimeKind struct {
	// Kind is the kind's name, written as the type of a custom source.
	Kind        string `json:"kind"`
	Description string `json:"description,omitempty"`
	// RequiredPermission, when set, is held by whoever creates or changes an
	// ingestion of this kind.
	RequiredPermission string `json:"requiredPermission,omitempty"`
	// Settings describes the kind's settings for the form; the platform passes
	// it through untouched.
	Settings json.RawMessage `json:"settings,omitempty"`
}

// builtInRequirements are the permissions a kind needs whatever a runtime
// reports, so a runtime that is down, or that reports nothing, cannot make a
// dangerous kind open. camel-route runs a route with the runtime's network,
// identity and environment.
var builtInRequirements = map[string]string{"camel-route": auth.PermIngestionRoute}

// LiveKinds returns the kinds the live runtimes report, one entry per kind
// (the first runtime's description wins; a required permission from any
// runtime is kept). A kind that does not parse is skipped and logged: one
// runtime's bad report must not hide the others.
func (r *Repository) LiveKinds(ctx context.Context) ([]RuntimeKind, error) {
	rows, err := r.q.IngestionRuntimeKindsSeenSince(ctx, time.Now().UTC().Add(-RuntimeLiveWindow))
	if err != nil {
		return nil, fmt.Errorf("ingestion repo: runtime kinds: %w", err)
	}
	var out []RuntimeKind
	index := map[string]int{}
	for _, raw := range rows {
		var reported []RuntimeKind
		if err := json.Unmarshal(raw, &reported); err != nil {
			slog.Warn("ingestion runtime reported kinds that do not parse", "err", err)
			continue
		}
		for _, k := range reported {
			if k.Kind == "" {
				continue
			}
			i, seen := index[k.Kind]
			if !seen {
				index[k.Kind] = len(out)
				out = append(out, k)
				continue
			}
			if out[i].RequiredPermission == "" {
				out[i].RequiredPermission = k.RequiredPermission
			}
		}
	}
	for i := range out {
		if perm, ok := builtInRequirements[out[i].Kind]; ok {
			out[i].RequiredPermission = perm
		}
	}
	return out, nil
}

// RequiredPermission is the permission an ingestion of the custom kind kind
// needs, "" for none: what the live runtimes report, or the built-in
// requirement for the kind.
func (r *Repository) RequiredPermission(ctx context.Context, kind string) (string, error) {
	if perm, ok := builtInRequirements[kind]; ok {
		return perm, nil
	}
	reported, err := r.LiveKinds(ctx)
	if err != nil {
		return "", err
	}
	for _, k := range reported {
		if k.Kind == kind {
			return k.RequiredPermission, nil
		}
	}
	return "", nil
}

// Failure is one failure a runtime reported for an ingestion.
type Failure struct {
	ID           string    `json:"id"`
	RuntimeID    *string   `json:"runtimeId,omitempty"`
	Version      *int      `json:"version,omitempty"`
	Kind         string    `json:"kind"`
	Message      string    `json:"message"`
	AfterRecords int64     `json:"afterRecords"`
	Excerpt      *string   `json:"excerpt,omitempty"`
	OccurredAt   time.Time `json:"occurredAt"`
}

// FailurePage is one page of failures, newest first. Next is empty on the
// last page.
type FailurePage struct {
	Items []Failure
	Next  string
}

// ListFailures pages an ingestion's failures by id, newest first; after is the
// Next of the previous page. limit is 50 when zero, at most 200.
func (r *Repository) ListFailures(ctx context.Context, ingestionID, after string, limit int) (*FailurePage, error) {
	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, 200)
	// One more than the page, to know whether there is a next one.
	rows, err := r.q.IngestionFailureList(ctx, dbq.IngestionFailureListParams{
		IngestionID: ingestionID, After: after, Lim: int32(limit + 1),
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
		f := Failure{
			ID: row.ID, RuntimeID: row.RuntimeID, Kind: row.Kind, Message: row.Message,
			AfterRecords: row.AfterRecords, Excerpt: row.Excerpt, OccurredAt: row.OccurredAt,
		}
		if row.Version != nil {
			v := int(*row.Version)
			f.Version = &v
		}
		page.Items = append(page.Items, f)
	}
	return page, nil
}
