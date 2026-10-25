package ingestion

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
)

// SecretFetch is when a runtime last fetched one of the secrets an ingestion
// binds (owner ruling 2026-10-25): a runtime fetching its ingestion's bound
// secrets is security telemetry, not an audited config reveal, and this row
// is what the pages and the API show of it.
type SecretFetch struct {
	IngestionID string `json:"ingestionId"`
	SecretName  string `json:"secretName"`
	RuntimeID   string `json:"runtimeId"`
	// Version is the live version the last fetch was made under.
	Version int `json:"version"`
	// PrincipalID is the runtime's service account that made it.
	PrincipalID   *string   `json:"principalId,omitempty"`
	LastFetchedAt time.Time `json:"lastFetchedAt"`
	FetchCount    int64     `json:"fetchCount"`
}

// RecordSecretFetch notes a successful fetch of the secret name of the
// ingestion by the runtime, by the database's clock: the row of (ingestion,
// name, runtime) is created or its time, version and principal replaced and
// its count raised.
func (r *Repository) RecordSecretFetch(ctx context.Context, ingestionID, name, runtimeID string, version int, principalID string) error {
	var principal *string
	if principalID != "" {
		principal = &principalID
	}
	if err := r.q.IngestionSecretFetchRecord(ctx, dbq.IngestionSecretFetchRecordParams{
		IngestionID: ingestionID, SecretName: name, RuntimeID: runtimeID, Version: int32(version), PrincipalID: principal,
	}); err != nil {
		return fmt.Errorf("ingestion repo: record secret fetch: %w", err)
	}
	return nil
}

// ListSecretFetches returns the ingestion's secret fetch rows, by secret name
// and then newest first.
func (r *Repository) ListSecretFetches(ctx context.Context, ingestionID string) ([]SecretFetch, error) {
	rows, err := r.q.IngestionSecretFetchList(ctx, ingestionID)
	if err != nil {
		return nil, fmt.Errorf("ingestion repo: list secret fetches: %w", err)
	}
	out := make([]SecretFetch, 0, len(rows))
	for _, row := range rows {
		out = append(out, SecretFetch{
			IngestionID: row.IngestionID, SecretName: row.SecretName, RuntimeID: row.RuntimeID, Version: int(row.Version),
			PrincipalID: row.PrincipalID, LastFetchedAt: row.LastFetchedAt, FetchCount: row.FetchCount,
		})
	}
	return out, nil
}

// BoundSecret is one secret name an ingestion binds, or that a runtime
// fetched while an earlier live version bound it, with its fetches.
type BoundSecret struct {
	Name string
	// Bound is true for a name the live definition names: a runtime may fetch
	// it now. A name only fetched before is false.
	Bound   bool
	Fetches []SecretFetch
}

// BoundSecrets lists the secrets the ingestion's live definition binds (none
// while nothing is live) in name order, each with its fetch rows, followed by
// the names that were fetched but are no longer bound.
func BoundSecrets(i *Ingestion, fetches []SecretFetch) []BoundSecret {
	var names []string
	if i.Version > 0 {
		names = SecretNames(i.Definition.Source)
		slices.Sort(names)
		names = slices.Compact(names)
	}
	out := make([]BoundSecret, 0, len(names))
	for _, n := range names {
		out = append(out, BoundSecret{Name: n, Bound: true})
	}
	for _, f := range fetches {
		k := slices.IndexFunc(out, func(b BoundSecret) bool { return b.Name == f.SecretName })
		if k < 0 {
			out = append(out, BoundSecret{Name: f.SecretName})
			k = len(out) - 1
		}
		out[k].Fetches = append(out[k].Fetches, f)
	}
	return out
}

// MetricsRegistry holds the ingestion feature's Prometheus series, in a
// registry of its own like every other subsystem's; the server mounts it on
// the metrics listener's /metrics.
//
//	fc_ingestion_secret_fetch_total{outcome}  runtime fetches of bound secrets: success, refused, error
//
// Labels are the outcome only: which ingestion, secret and runtime is in the
// security log line each fetch writes ("security.ingestion_secret_fetch"),
// never in a label.
var MetricsRegistry = prometheus.NewRegistry()

// The outcomes of a runtime's secret fetch.
const (
	SecretFetchSuccess = "success"
	SecretFetchRefused = "refused"
	SecretFetchError   = "error"
)

var secretFetches = newSecretFetchCounter(MetricsRegistry)

func newSecretFetchCounter(reg prometheus.Registerer) *prometheus.CounterVec {
	v := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "fc_ingestion_secret_fetch_total",
		Help: "Ingestion runtime fetches of the secrets an ingestion's live definition binds, by outcome (success, refused, error).",
	}, []string{"outcome"})
	for _, o := range []string{SecretFetchSuccess, SecretFetchRefused, SecretFetchError} {
		v.WithLabelValues(o)
	}
	reg.MustRegister(v)
	return v
}

// CountSecretFetch counts one runtime secret fetch with the outcome.
func CountSecretFetch(outcome string) { secretFetches.WithLabelValues(outcome).Inc() }
