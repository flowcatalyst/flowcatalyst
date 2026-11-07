package ingestion

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"
)

// UnheldGrace is how long past one lease a live polled ingestion may go
// without an unexpired lease before it counts as unheld: a survivor takes an
// expired lease at its next poll, a minute at most, so longer than that means
// no runtime is taking it.
const UnheldGrace = 2 * time.Minute

// UnheldAfter is how long a live polled ingestion goes without an unexpired
// lease before it is unheld: one lease and UnheldGrace.
func UnheldAfter(lease time.Duration) time.Duration { return lease + UnheldGrace }

// UnheldInterval is how often RunUnheldGauge recounts.
const UnheldInterval = time.Minute

// Unheld finds the live polled (single-owner) ingestions no runtime has held
// for longer than UnheldAfter(lease): every one when ids is nil, else those of
// ids. It answers, for each, the time since which none has. That time is the
// later of the end of its last lease and the ingestion's updated_at (a resume
// or a promote is an update, and an ingestion that was paused, or never live,
// owed no runtime anything before it); with no lease row, updated_at alone.
// An ingestion whose settings are in a config entry that is missing or invalid
// is not run anywhere by design and is not unheld (it shows "Needs settings").
// A zero now is the database's clock.
func (r *Repository) Unheld(ctx context.Context, ids []string, now time.Time, lease time.Duration) (map[string]time.Time, error) {
	out := map[string]time.Time{}
	if ids != nil && len(ids) == 0 {
		return out, nil
	}
	if lease <= 0 {
		lease = DefaultLease
	}
	err := pgx.BeginTxFunc(ctx, r.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)
		now, err := clock(ctx, q, now)
		if err != nil {
			return err
		}
		var only *[]string
		if ids != nil {
			only = &ids
		}
		live, err := liveIngestions(ctx, tx, nil, nil, only)
		if err != nil {
			return err
		}
		if live, err = resolveListed(ctx, q, live); err != nil {
			return err
		}
		var polled []ListItem
		polledIDs := []string{}
		for _, item := range live {
			if item.Definition.Source.SingleOwner() {
				polled = append(polled, item)
				polledIDs = append(polledIDs, item.ID)
			}
		}
		if len(polled) == 0 {
			return nil
		}
		leases, err := q.IngestionLeasesOf(ctx, polledIDs)
		if err != nil {
			return fmt.Errorf("leases: %w", err)
		}
		ends := make(map[string]time.Time, len(leases))
		for _, l := range leases {
			ends[l.IngestionID] = l.ExpiresAt.UTC()
		}
		for _, item := range polled {
			since := item.UpdatedAt.UTC()
			if end, leased := ends[item.ID]; leased {
				if end.After(now) {
					continue // held
				}
				if end.After(since) {
					since = end
				}
			}
			if now.Sub(since) > UnheldAfter(lease) {
				out[item.ID] = since
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("ingestion repo: unheld: %w", err)
	}
	return out, nil
}

// UnheldNotice is the runtime notice of an unheld ingestion: it is not running,
// and nothing says so but this.
func UnheldNotice(since time.Time) string {
	return "not running: no runtime has held it since " + since.UTC().Format(time.RFC3339)
}

var unheldGauge = newUnheldGauge(MetricsRegistry)

func newUnheldGauge(reg prometheus.Registerer) prometheus.Gauge {
	g := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "fc_ingestion_unheld",
		Help: "Live polled (single-owner) ingestions no runtime has held for longer than one lease and two minutes: not running anywhere.",
	})
	reg.MustRegister(g)
	return g
}

// RunUnheldGauge counts the unheld ingestions (fc_ingestion_unheld) now and
// then every interval, until ctx ends. Every platform replica counts the same
// rows, so each reports the same value; it is a read, not leader-gated. A
// failed count leaves the last value and is logged.
func RunUnheldGauge(ctx context.Context, repo *Repository, lease, interval time.Duration) {
	if interval <= 0 {
		interval = UnheldInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := CountUnheld(ctx, repo, lease); err != nil && ctx.Err() == nil {
			slog.Warn("ingestion unheld count failed", "err", err)
		}
		select {
		case <-ctx.Done(): // shutdown
			return
		case <-ticker.C: // time to recount
		}
	}
}

// UnheldLogIDs caps the ids the "ingestions unheld" line names; the rest are
// counted in its "more" attribute.
const UnheldLogIDs = 20

// CountUnheld sets fc_ingestion_unheld to the number of unheld ingestions now,
// and, when there are any, logs one WARN line "ingestions unheld" with the
// count, up to UnheldLogIDs ids (sorted) and how many more there are. The
// metric is not scraped everywhere; a log metric filter on that line (count is
// a top-level number) can drive an alarm. Nothing is logged for none. On error
// the gauge keeps its last value.
func CountUnheld(ctx context.Context, repo *Repository, lease time.Duration) error {
	unheld, err := repo.Unheld(ctx, nil, time.Time{}, lease)
	if err != nil {
		return err
	}
	unheldGauge.Set(float64(len(unheld)))
	if len(unheld) > 0 {
		ids := make([]string, 0, len(unheld))
		for id := range unheld {
			ids = append(ids, id)
		}
		slices.Sort(ids)
		more := max(len(ids)-UnheldLogIDs, 0)
		ids = ids[:len(ids)-more]
		slog.Warn("ingestions unheld", "count", len(unheld), "ids", ids, "more", more)
	}
	return nil
}
