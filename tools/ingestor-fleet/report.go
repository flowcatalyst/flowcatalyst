package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type fedRow struct {
	id int64
	at time.Time
}

type jobRow struct {
	recordID int64 // -1 when the payload names none
	at       time.Time
	runtime  string
}

// data is what was fed and what jobs were made, per ingestion code.
type data struct {
	fed  map[string][]fedRow
	jobs map[string][]jobRow // ordered by created_at
}

// recordStats is the exactly-once count of one ingestion.
type recordStats struct {
	fed, jobs, duplicates, missing, unknown int
}

func (d data) stats(code string) recordStats {
	st := recordStats{fed: len(d.fed[code]), jobs: len(d.jobs[code])}
	per := map[int64]int{}
	for _, j := range d.jobs[code] {
		if j.recordID < 0 {
			st.unknown++
			continue
		}
		per[j.recordID]++
	}
	fed := map[int64]bool{}
	for _, f := range d.fed[code] {
		fed[f.id] = true
		if per[f.id] == 0 {
			st.missing++
		}
	}
	for id, n := range per {
		if n > 1 {
			st.duplicates += n - 1
		}
		if !fed[id] {
			st.unknown += n
		}
	}
	return st
}

func (t *test) collect(ctx context.Context) (data, error) {
	d := data{fed: map[string][]fedRow{}, jobs: map[string][]jobRow{}}
	for _, s := range t.specs {
		rows, err := t.src.Query(ctx, "SELECT id, fed_at FROM "+s.table+" ORDER BY id")
		if err != nil {
			return d, err
		}
		for rows.Next() {
			var r fedRow
			if err := rows.Scan(&r.id, &r.at); err != nil {
				rows.Close()
				return d, err
			}
			d.fed[s.code] = append(d.fed[s.code], r)
		}
		rows.Close()
		if rows.Err() != nil {
			return d, rows.Err()
		}
	}
	rows, err := t.plat.Query(ctx, `SELECT source, payload, created_at,
			coalesce((SELECT m->>'value' FROM jsonb_array_elements(CASE WHEN jsonb_typeof(metadata) = 'array' THEN metadata ELSE '[]'::jsonb END) m
			          WHERE m->>'key' = 'runtimeId' LIMIT 1), '?')
		FROM msg_dispatch_jobs WHERE source LIKE $1 AND created_at >= $2 ORDER BY created_at, id`,
		jobSourcePfx+"%", t.started.Add(-time.Hour))
	if err != nil {
		return d, err
	}
	defer rows.Close()
	for rows.Next() {
		var src string
		var payload *string
		var j jobRow
		if err := rows.Scan(&src, &payload, &j.at, &j.runtime); err != nil {
			return d, err
		}
		j.recordID = -1
		if payload != nil {
			var rec struct {
				ID *json.Number `json:"id"`
			}
			if json.Unmarshal([]byte(*payload), &rec) == nil && rec.ID != nil {
				if v, err := rec.ID.Int64(); err == nil {
					j.recordID = v
				}
			}
		}
		code := strings.TrimPrefix(src, jobSourcePfx)
		d.jobs[code] = append(d.jobs[code], j)
	}
	return d, rows.Err()
}

// jobRuntimesSince answers, per ingestion code, the runtimes that made its
// jobs since from (sorted).
func jobRuntimesSince(ctx context.Context, db *pgxpool.Pool, from time.Time) (map[string][]string, error) {
	rows, err := db.Query(ctx, `SELECT DISTINCT source,
			coalesce((SELECT m->>'value' FROM jsonb_array_elements(CASE WHEN jsonb_typeof(metadata) = 'array' THEN metadata ELSE '[]'::jsonb END) m
			          WHERE m->>'key' = 'runtimeId' LIMIT 1), '?')
		FROM msg_dispatch_jobs WHERE source LIKE $1 AND created_at >= $2`, jobSourcePfx+"%", from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var src, rt string
		if err := rows.Scan(&src, &rt); err != nil {
			return nil, err
		}
		code := strings.TrimPrefix(src, jobSourcePfx)
		out[code] = append(out[code], rt)
	}
	for k := range out {
		sort.Strings(out[k])
	}
	return out, rows.Err()
}

// windowStats is one ingestion during one fault.
type windowStats struct {
	MaxGap     time.Duration `json:"maxGapNanos"`
	GapAt      time.Time     `json:"gapEndedAt"`
	MaxLatency time.Duration `json:"maxLatencyNanos"`
	Fed        int           `json:"fed"`
	Holders    []string      `json:"holders"` // runtimes that made its jobs in the window, in turn
}

// windowOf measures one ingestion's jobs in w: the longest time without a job
// (from its last job before the fault), the longest a record fed in the
// window waited for its job, and which runtimes made its jobs, in turn. The
// latency counts records fed up to settle before the window's end: the next
// fault starts at once, and a record fed in the last moments of this window
// waits through that one.
func windowOf(code string, d data, w *window, settle time.Duration) windowStats {
	var ws windowStats
	jobs := d.jobs[code]
	prev := w.Start
	for _, j := range jobs {
		if j.at.Before(w.Start) {
			prev = j.at
		}
	}
	first := map[int64]time.Time{}
	for _, j := range jobs {
		if j.recordID >= 0 {
			if _, seen := first[j.recordID]; !seen {
				first[j.recordID] = j.at
			}
		}
		if j.at.Before(w.Start) || j.at.After(w.End) {
			continue
		}
		if gap := j.at.Sub(prev); gap > ws.MaxGap {
			ws.MaxGap, ws.GapAt = gap, j.at
		}
		prev = j.at
		if n := len(ws.Holders); n == 0 || ws.Holders[n-1] != j.runtime {
			ws.Holders = append(ws.Holders, j.runtime)
		}
	}
	if tail := w.End.Sub(prev); tail > ws.MaxGap {
		ws.MaxGap, ws.GapAt = tail, w.End
	}
	for _, f := range d.fed[code] {
		if f.at.Before(w.Start) || f.at.After(w.End) {
			continue
		}
		ws.Fed++
		if f.at.After(w.End.Add(-settle)) {
			continue
		}
		at, ok := first[f.id]
		if !ok {
			continue // counted as missing overall
		}
		if lat := at.Sub(f.at); lat > ws.MaxLatency {
			ws.MaxLatency = lat
		}
	}
	return ws
}

// report writes the report (stdout, report.txt, report.json) and answers
// whether an assertion failed.
func (t *test) report(d data) (bool, error) {
	var b strings.Builder
	var failures []string
	cfg := t.cfg
	short := func(rt string) string { return strings.TrimPrefix(rt, "fleet-") }
	secs := func(d time.Duration) string { return fmt.Sprintf("%.1fs", d.Seconds()) }

	fmt.Fprintf(&b, "\nIngestor fleet test, %s (ran %s)\n", t.started.UTC().Format(time.RFC3339), time.Since(t.started).Round(time.Second))
	fmt.Fprintf(&b, "lease %s, runtime poll %s, source every %s, %.1f records/s per source; %d ingestions (%d database-poll, %d api-poll); %d runtimes\n",
		cfg.lease, cfg.poll, cfg.sourceEvery, cfg.rate, len(t.specs), cfg.dbIngestions, len(t.specs)-cfg.dbIngestions, len(t.fleet.procs))

	type jsonWindow struct {
		*window
		Ingestions map[string]windowStats `json:"ingestions"`
	}
	var jw []jsonWindow
	for i, w := range t.windows {
		fmt.Fprintf(&b, "\n%d. %s\n", i+1, w.Title)
		fmt.Fprintf(&b, "   from +%s for %s; bound on the gap %s (%s)\n",
			w.Start.Sub(t.started).Round(time.Second), w.End.Sub(w.Start).Round(time.Second), secs(w.Bound), w.BoundWhy)
		for _, n := range w.Notes {
			fmt.Fprintf(&b, "   - %s\n", n)
		}
		fmt.Fprintf(&b, "   %-8s %9s %12s %5s  %-6s %s\n", "ingest", "max gap", "max latency", "fed", "result", "jobs made by, in turn")
		per := map[string]windowStats{}
		for _, s := range t.specs {
			ws := windowOf(s.code, d, w, 2*cfg.freshWithin)
			per[s.code] = ws
			result := "ok"
			if w.End.IsZero() {
				result = "n/a"
			} else if ws.MaxGap > w.Bound {
				result = "FAIL"
				failures = append(failures, fmt.Sprintf("%s: %s went %s without a job (bound %s)", w.Name, s.code, secs(ws.MaxGap), secs(w.Bound)))
			}
			holders := make([]string, len(ws.Holders))
			for i, h := range ws.Holders {
				holders[i] = short(h)
			}
			fmt.Fprintf(&b, "   %-8s %9s %12s %5d  %-6s %s\n", s.code, secs(ws.MaxGap), secs(ws.MaxLatency), ws.Fed, result, strings.Join(holders, " > "))
		}
		// The unheld gauge over the window.
		samples := t.obs.between(w.Start, w.End)
		for _, s := range samples {
			if !s.ok {
				w.GaugeMissed++
				continue
			}
			w.GaugeSamples++
			w.GaugeMax = max(w.GaugeMax, s.value)
		}
		switch w.GaugeExpect {
		case "zero":
			fmt.Fprintf(&b, "   fc_ingestion_unheld: max %.0f over %d samples (%d unanswered); expected 0\n", w.GaugeMax, w.GaugeSamples, w.GaugeMissed)
			if w.GaugeMax > 0 {
				failures = append(failures, fmt.Sprintf("%s: fc_ingestion_unheld rose to %.0f", w.Name, w.GaugeMax))
			}
		case "rise":
			rose, cleared := "never", "never"
			if w.GaugeRose != nil {
				rose = "+" + w.GaugeRose.Sub(w.Start).Round(time.Second).String()
			}
			if w.GaugeCleared != nil {
				cleared = "+" + w.GaugeCleared.Sub(w.Start).Round(time.Second).String()
			}
			fmt.Fprintf(&b, "   fc_ingestion_unheld: rose at %s to a max of %.0f (expected %d), back to 0 at %s\n", rose, w.GaugeMax, len(t.specs), cleared)
			if w.GaugeMax < float64(len(t.specs)) {
				failures = append(failures, fmt.Sprintf("%s: fc_ingestion_unheld rose only to %.0f, not %d", w.Name, w.GaugeMax, len(t.specs)))
			}
			if w.GaugeCleared == nil {
				failures = append(failures, w.Name+": fc_ingestion_unheld did not return to 0")
			}
		}
		for _, f := range w.Failures {
			failures = append(failures, w.Name+": "+f)
		}
		jw = append(jw, jsonWindow{window: w, Ingestions: per})
	}

	fmt.Fprintf(&b, "\nRecords (feed stopped at +%s)\n", t.feedStop.Sub(t.started).Round(time.Second))
	fmt.Fprintf(&b, "   %-8s %6s %6s %10s %8s %8s\n", "ingest", "fed", "jobs", "duplicates", "missing", "unknown")
	recs := map[string]recordStats{}
	for _, s := range t.specs {
		st := d.stats(s.code)
		recs[s.code] = st
		fmt.Fprintf(&b, "   %-8s %6d %6d %10d %8d %8d\n", s.code, st.fed, st.jobs, st.duplicates, st.missing, st.unknown)
		if st.duplicates > 0 || st.missing > 0 || st.unknown > 0 {
			failures = append(failures, fmt.Sprintf("records: %s has %d duplicate, %d missing and %d unknown job(s) for %d fed", s.code, st.duplicates, st.missing, st.unknown, st.fed))
		}
		if st.fed == 0 {
			failures = append(failures, "records: nothing was fed to "+s.code)
		}
	}
	for table, n := range t.feedErrs {
		fmt.Fprintf(&b, "   (the feeder failed %d insert(s) into %s; those records were never fed)\n", n, table)
	}

	fmt.Fprintf(&b, "\nAt the end\n")
	for _, s := range t.specs {
		fmt.Fprintf(&b, "   %-8s held by %-12s jobs while settling made by %v\n", s.code, t.final.Holders[s.code], t.final.JobsBy[s.code])
	}
	for _, f := range t.final.Failures {
		failures = append(failures, "end: "+f)
	}

	if len(failures) == 0 {
		fmt.Fprintf(&b, "\nRESULT: PASS\n")
	} else {
		fmt.Fprintf(&b, "\nRESULT: FAIL (%d)\n", len(failures))
		for _, f := range failures {
			fmt.Fprintf(&b, "   - %s\n", f)
		}
	}
	fmt.Print(b.String())
	if err := os.WriteFile(filepath.Join(cfg.state, "report.txt"), []byte(b.String()), 0o600); err != nil {
		return true, err
	}
	js, err := json.MarshalIndent(map[string]any{
		"started": t.started, "leaseSeconds": cfg.lease.Seconds(), "pollSeconds": cfg.poll.Seconds(),
		"sourceEverySeconds": cfg.sourceEvery.Seconds(), "rate": cfg.rate,
		"windows": jw, "records": recordsJSON(recs), "final": t.final, "failures": failures,
	}, "", "  ")
	if err != nil {
		return true, err
	}
	if err := os.WriteFile(filepath.Join(cfg.state, "report.json"), js, 0o600); err != nil {
		return true, err
	}
	return len(failures) > 0, nil
}

func recordsJSON(m map[string]recordStats) map[string]map[string]int {
	out := map[string]map[string]int{}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		s := m[k]
		out[k] = map[string]int{"fed": s.fed, "jobs": s.jobs, "duplicates": s.duplicates, "missing": s.missing, "unknown": s.unknown}
	}
	return out
}
