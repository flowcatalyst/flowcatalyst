package main

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// gaugeSample is one read of fc_ingestion_unheld; ok is false when the
// metrics endpoint did not answer (the platform frozen, say).
type gaugeSample struct {
	at    time.Time
	value float64
	ok    bool
}

// observer samples the unheld gauge every couple of seconds for the whole run.
type observer struct {
	url    string
	client *http.Client
	mu     sync.Mutex
	gauge  []gaugeSample
}

func startObserver(ctx context.Context, metricsURL string) *observer {
	o := &observer{url: metricsURL, client: &http.Client{Timeout: 2 * time.Second}}
	go func() {
		tick := time.NewTicker(2 * time.Second)
		defer tick.Stop()
		for {
			v, ok := o.read(ctx)
			o.mu.Lock()
			o.gauge = append(o.gauge, gaugeSample{at: wallNow(), value: v, ok: ok})
			o.mu.Unlock()
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
		}
	}()
	return o
}

func (o *observer) read(ctx context.Context) (float64, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.url, nil)
	if err != nil {
		return 0, false
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return 0, false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return 0, false
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	for sc.Scan() {
		line := sc.Text()
		if rest, found := strings.CutPrefix(line, "fc_ingestion_unheld "); found {
			v, err := strconv.ParseFloat(strings.TrimSpace(rest), 64)
			return v, err == nil
		}
	}
	return 0, false
}

// latest answers the newest sample that answered.
func (o *observer) latest() (gaugeSample, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, g := range slices.Backward(o.gauge) {
		if g.ok {
			return g, true
		}
	}
	return gaugeSample{}, false
}

// between answers the samples taken in [from, to].
func (o *observer) between(from, to time.Time) []gaugeSample {
	o.mu.Lock()
	defer o.mu.Unlock()
	var out []gaugeSample
	for _, s := range o.gauge {
		if !s.at.Before(from) && !s.at.After(to) {
			out = append(out, s)
		}
	}
	return out
}

// ── what the platform's database says ───────────────────────────────────────

// leases answers, for each of the ingestions, the runtime holding an
// unexpired, unreleased lease ("" for none).
func leases(ctx context.Context, db *pgxpool.Pool, ids []string) (map[string]string, error) {
	rows, err := db.Query(ctx, `SELECT ingestion_id, runtime_id FROM ing_leases
		WHERE ingestion_id = ANY($1) AND expires_at > now() AND NOT released`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for _, id := range ids {
		out[id] = ""
	}
	for rows.Next() {
		var ing, rt string
		if err := rows.Scan(&ing, &rt); err != nil {
			return nil, err
		}
		out[ing] = rt
	}
	return out, rows.Err()
}

// runtimeRow is a runtime as the platform last saw it.
type runtimeRow struct {
	lastSeen time.Time
	leftAt   *time.Time
	instance *string
}

func runtimeState(ctx context.Context, db *pgxpool.Pool, id string) (runtimeRow, bool, error) {
	var r runtimeRow
	err := db.QueryRow(ctx, `SELECT last_seen_at, left_at, instance_id FROM ing_runtimes WHERE runtime_id = $1`, id).
		Scan(&r.lastSeen, &r.leftAt, &r.instance)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return r, false, nil
		}
		return r, false, err
	}
	return r, true, nil
}

// liveRuntimes answers the runtimes the platform counts as live: polled within
// one lease and not left since.
func liveRuntimes(ctx context.Context, db *pgxpool.Pool, lease time.Duration) (map[string]bool, error) {
	rows, err := db.Query(ctx, `SELECT runtime_id FROM ing_runtimes
		WHERE last_seen_at > now() - make_interval(secs => $1) AND (left_at IS NULL OR left_at < last_seen_at)`, lease.Seconds())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// latestJobs answers, per ingestion code, how long ago (by the database's
// clock) its newest job was made; an ingestion with none in ten minutes is
// missing from the map.
func latestJobs(ctx context.Context, db *pgxpool.Pool) (map[string]time.Duration, error) {
	rows, err := db.Query(ctx, `SELECT source, extract(epoch FROM now() - max(created_at))
		FROM msg_dispatch_jobs
		WHERE source LIKE $1 AND created_at > now() - interval '10 minutes'
		GROUP BY source`, jobSourcePfx+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]time.Duration{}
	for rows.Next() {
		var src string
		var age float64
		if err := rows.Scan(&src, &age); err != nil {
			return nil, err
		}
		out[strings.TrimPrefix(src, jobSourcePfx)] = time.Duration(age * float64(time.Second))
	}
	return out, rows.Err()
}

// waitFor polls cond every second until it holds, ctx ends or timeout passes.
// what names the condition in the error.
func waitFor(ctx context.Context, timeout time.Duration, what string, cond func() (bool, string, error)) error {
	deadline := wallNow().Add(timeout)
	last := ""
	for {
		ok, why, err := cond()
		if err != nil {
			last = err.Error()
		} else {
			if ok {
				return nil
			}
			last = why
		}
		if wallNow().After(deadline) {
			return fmt.Errorf("timed out after %s waiting for %s (last: %s)", timeout.Round(time.Second), what, last)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
