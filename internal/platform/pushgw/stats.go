package pushgw

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/framework/go/fw"
	"github.com/flowcatalyst/framework/go/metrics"
)

// The gateway's stats (spec section 11, console page 2): every gateway task writes one row of
// push_gateway_stats every StatsEvery, and the console sums the fresh rows. A row holds what the
// task can read of itself: the streams it holds open (per application and per client), and the
// last minute of the relay's lag histogram, of publishing and of ingest.

// StatsEvery is how often a task rewrites its stats row.
var StatsEvery = 10 * time.Second

// StatsFresh is how old a row may be and still count: three missed writes. A task that died
// stops counting within this long.
const StatsFresh = 30 * time.Second

// statsKeep is how long a dead task's row stays before a live task deletes it.
const statsKeep = 5 * time.Minute

// statsWindow is how many seconds of history a row's windowed values (lag, published, ingest)
// cover.
const statsWindow = 60 * time.Second

// TaskStats is what one gateway task writes. It is the JSON of push_gateway_stats.stats.
type TaskStats struct {
	// Driver is the log driver the task runs ("postgres" or "redis"): the console reads the log
	// of pushes the same way.
	Driver string `json:"driver"`
	// Open is the streams the module holds open on this task (fw_push_streams_open).
	Open int `json:"open"`
	// ByApplication and ByClient count the open streams by the application codes of their
	// topics (a stream spanning two applications counts for each) and by their client id (the
	// platform client is "_platform").
	ByApplication map[string]int `json:"byApplication"`
	ByClient      map[string]int `json:"byClient"`
	// Lag is the relay lag (the age of the oldest outbox row a relay run moved) of the last
	// minute, as histogram bucket counts (not cumulative).
	Lag Histogram `json:"lag"`
	// Published is the pushes the relay logged in the last minute.
	Published float64 `json:"published"`
	// Ingested is the pushes ingest answered in the last minute by application and result.
	Ingested map[string]map[string]float64 `json:"ingested"`
	// WindowSecs is how many seconds the windowed values cover (up to a minute after a start).
	WindowSecs float64 `json:"windowSecs"`
}

// Histogram is a latency histogram that sums: Bounds are the bucket upper bounds in seconds and
// Counts has one more entry than Bounds (the last is the overflow), not cumulative.
type Histogram struct {
	Bounds []float64 `json:"bounds"`
	Counts []uint64  `json:"counts"`
}

// Total is the number of observations.
func (h Histogram) Total() uint64 {
	var n uint64
	for _, c := range h.Counts {
		n += c
	}
	return n
}

// Add returns the sum of two histograms of the same bounds (the receiver's bounds win when they
// differ and the other is ignored: tasks of one build agree).
func (h Histogram) Add(o Histogram) Histogram {
	if len(h.Counts) == 0 {
		return Histogram{Bounds: o.Bounds, Counts: append([]uint64(nil), o.Counts...)}
	}
	out := Histogram{Bounds: h.Bounds, Counts: append([]uint64(nil), h.Counts...)}
	if len(o.Counts) != len(out.Counts) {
		return out
	}
	for i, c := range o.Counts {
		out.Counts[i] += c
	}
	return out
}

// Quantile estimates the q-quantile (0 to 1) in seconds by linear interpolation inside the
// bucket; ok is false for an empty histogram. An observation beyond the last bound counts as
// that bound.
func (h Histogram) Quantile(q float64) (seconds float64, ok bool) {
	total := h.Total()
	if total == 0 || len(h.Bounds) == 0 {
		return 0, false
	}
	rank := q * float64(total)
	var seen float64
	lower := 0.0
	for i, c := range h.Counts {
		upper := h.Bounds[len(h.Bounds)-1]
		if i < len(h.Bounds) {
			upper = h.Bounds[i]
		}
		if seen+float64(c) >= rank && c > 0 {
			if i >= len(h.Bounds) {
				return upper, true
			}
			return lower + (upper-lower)*(rank-seen)/float64(c), true
		}
		seen += float64(c)
		lower = upper
	}
	return h.Bounds[len(h.Bounds)-1], true
}

// ── counting the streams ──────────────────────────────────────────────

// streamCounter counts the open streams per application and per client. The module's hub keeps
// those counts but does not expose them, so the gateway counts at its HTTP edge: a GET of
// .../push/stream that the module answers 200 is a stream until the request returns.
type streamCounter struct {
	mu       sync.Mutex
	byApp    map[string]int
	byClient map[string]int
}

func newStreamCounter() *streamCounter {
	return &streamCounter{byApp: map[string]int{}, byClient: map[string]int{}}
}

func (c *streamCounter) add(apps []string, client string, d int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, a := range apps {
		if c.byApp[a] += d; c.byApp[a] <= 0 {
			delete(c.byApp, a)
		}
	}
	if c.byClient[client] += d; c.byClient[client] <= 0 {
		delete(c.byClient, client)
	}
}

func (c *streamCounter) snapshot() (byApp, byClient map[string]int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	byApp, byClient = make(map[string]int, len(c.byApp)), make(map[string]int, len(c.byClient))
	maps.Copy(byApp, c.byApp)
	maps.Copy(byClient, c.byClient)
	return byApp, byClient
}

// tokenFacts reads the client and the applications out of a stream token's payload without
// verifying it: the module verifies the token, and a stream is counted only when the module
// answered 200, so a forged token is never counted.
func tokenFacts(st string) (apps []string, client string, ok bool) {
	payload, _, found := strings.Cut(st, ".")
	if !found {
		return nil, "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return nil, "", false
	}
	var c struct {
		Client string   `json:"c"`
		Topics []string `json:"t"`
	}
	if json.Unmarshal(raw, &c) != nil || c.Client == "" || len(c.Topics) == 0 {
		return nil, "", false
	}
	seen := map[string]bool{}
	for _, t := range c.Topics {
		app, _, _ := strings.Cut(t, ":")
		if app != "" && !seen[app] {
			seen[app] = true
			apps = append(apps, app)
		}
	}
	return apps, c.Client, true
}

// wrap counts the streams of next, the module's handler.
func (c *streamCounter) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/push/stream") {
			next.ServeHTTP(w, r)
			return
		}
		apps, client, ok := tokenFacts(r.URL.Query().Get("st"))
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		cw := &countingWriter{ResponseWriter: w, onOpen: func() { c.add(apps, client, 1) }}
		defer func() {
			if cw.opened {
				c.add(apps, client, -1)
			}
		}()
		next.ServeHTTP(cw, r)
	})
}

// countingWriter calls onOpen once, when the response starts with status 200.
type countingWriter struct {
	http.ResponseWriter
	onOpen func()
	opened bool
	wrote  bool
}

func (w *countingWriter) WriteHeader(code int) {
	if !w.wrote {
		w.wrote = true
		if code == http.StatusOK {
			w.opened = true
			w.onOpen()
		}
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *countingWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

func (w *countingWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the underlying writer (the module flushes and sets
// deadlines through it).
func (w *countingWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// ── the writer ────────────────────────────────────────────────────────

// cumulative is one reading of the registry's counters, kept to take the windowed difference.
type cumulative struct {
	at        time.Time
	lag       []uint64 // non-cumulative bucket counts, as many as bounds + 1
	published float64
	ingested  map[string]map[string]float64
}

// statsCollector reads the task's stats.
type statsCollector struct {
	driver  string
	reg     *metrics.Registry
	streams *streamCounter
	history []cumulative
	now     func() time.Time
}

func (s *statsCollector) read() cumulative {
	c := cumulative{at: s.now(), ingested: map[string]map[string]float64{}}
	if s.reg == nil {
		return c
	}
	for _, f := range s.reg.Snapshot().Families {
		switch f.Name {
		case "fw_push_published_total":
			for _, sm := range f.Samples {
				c.published += sm.Value
			}
		case "fw_push_relay_lag_seconds":
			if len(f.Buckets) == 0 {
				continue
			}
			c.lag = make([]uint64, len(f.Buckets)+1)
			for _, sm := range f.Samples {
				var prev uint64
				for i, cum := range sm.CumulativeCounts {
					if i < len(f.Buckets) {
						c.lag[i] += cum - prev
					}
					prev = cum
				}
				if sm.Count >= prev {
					c.lag[len(f.Buckets)] += sm.Count - prev
				}
			}
		case "fc_push_ingest_items_total":
			for _, sm := range f.Samples {
				if len(sm.LabelValues) < 2 {
					continue
				}
				app, result := sm.LabelValues[0], sm.LabelValues[1]
				if c.ingested[app] == nil {
					c.ingested[app] = map[string]float64{}
				}
				c.ingested[app][result] += sm.Value
			}
		}
	}
	return c
}

func (s *statsCollector) openGauge() int {
	if s.reg == nil {
		return 0
	}
	for _, f := range s.reg.Snapshot().Families {
		if f.Name == "fw_push_streams_open" {
			var n float64
			for _, sm := range f.Samples {
				n += sm.Value
			}
			return int(n)
		}
	}
	return 0
}

// collect takes a reading and returns the task's stats: the windowed values are the difference
// to the oldest reading within statsWindow.
func (s *statsCollector) collect() TaskStats {
	cur := s.read()
	s.history = append(s.history, cur)
	for len(s.history) > 1 && cur.at.Sub(s.history[1].at) >= statsWindow {
		s.history = s.history[1:]
	}
	base := s.history[0]
	byApp, byClient := s.streams.snapshot()
	out := TaskStats{
		Driver: s.driver, Open: s.openGauge(), ByApplication: byApp, ByClient: byClient,
		Ingested: map[string]map[string]float64{}, WindowSecs: cur.at.Sub(base.at).Seconds(),
		Published: math.Max(0, cur.published-base.published),
	}
	if len(cur.lag) > 0 {
		out.Lag = Histogram{Bounds: metrics.DefaultBuckets, Counts: make([]uint64, len(cur.lag))}
		for i, c := range cur.lag {
			var b uint64
			if i < len(base.lag) {
				b = base.lag[i]
			}
			if c > b {
				out.Lag.Counts[i] = c - b
			}
		}
	}
	for app, results := range cur.ingested {
		for result, v := range results {
			if d := v - base.ingested[app][result]; d > 0 {
				if out.Ingested[app] == nil {
					out.Ingested[app] = map[string]float64{}
				}
				out.Ingested[app][result] = d
			}
		}
	}
	return out
}

// taskID names this task in push_gateway_stats: the host, and an id that differs per start so a
// restart on the same host does not inherit a dead task's row.
func newTaskID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "gateway"
	}
	if len(host) > 70 {
		host = host[:70]
	}
	var b [4]byte
	_, _ = rand.Read(b[:])
	return host + "-" + hex.EncodeToString(b[:])
}

// runStats writes the task's stats row until ctx ends, then deletes it. It also deletes the rows
// of tasks that stopped writing.
func (g *Gateway) runStats(ctx context.Context, log *slog.Logger, pool *pgxpool.Pool, clock fw.Clock) {
	col := &statsCollector{driver: g.driver(), reg: g.metrics, streams: g.streams, now: func() time.Time { return clock.Now() }}
	id := newTaskID()
	t := time.NewTicker(StatsEvery)
	defer t.Stop()
	write := func() {
		st := col.collect()
		if err := WriteStats(ctx, pool, id, st, clock.Now()); err != nil && ctx.Err() == nil {
			log.Warn("push gateway: writing the stats row failed", "err", err)
		}
		if _, err := PruneStats(ctx, pool, clock.Now()); err != nil && ctx.Err() == nil {
			log.Warn("push gateway: pruning the stats rows failed", "err", err)
		}
	}
	write()
	for {
		select {
		case <-ctx.Done():
			// best effort, on a context of its own: the row of a stopped task must not count
			dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
			_, _ = pool.Exec(dctx, `DELETE FROM push_gateway_stats WHERE task_id = $1`, id)
			cancel()
			return
		case <-t.C:
			write()
		}
	}
}

func (g *Gateway) driver() string {
	if d := g.env["PUSH_DRIVER"]; d != "" {
		return d
	}
	return "postgres"
}

// WriteStats upserts a task's stats row.
func WriteStats(ctx context.Context, pool *pgxpool.Pool, taskID string, st TaskStats, at time.Time) error {
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO push_gateway_stats (task_id, at, stats) VALUES ($1, $2, $3)
		ON CONFLICT (task_id) DO UPDATE SET at = EXCLUDED.at, stats = EXCLUDED.stats`,
		taskID, at.UTC(), raw)
	return err
}

// PruneStats deletes the rows of tasks that have not written for statsKeep.
func PruneStats(ctx context.Context, pool *pgxpool.Pool, now time.Time) (int64, error) {
	tag, err := pool.Exec(ctx, `DELETE FROM push_gateway_stats WHERE at < $1`, now.Add(-statsKeep).UTC())
	return tag.RowsAffected(), err
}

// ── reading ───────────────────────────────────────────────────────────

// TaskRow is a stats row as stored.
type TaskRow struct {
	TaskID string
	At     time.Time
	Stats  TaskStats
}

// Live is the sum of the fresh stats rows.
type Live struct {
	// Tasks is how many tasks reported within [StatsFresh].
	Tasks int
	// Newest is the newest row's time (zero with no rows).
	Newest time.Time
	// Driver is the log driver the tasks report ("" with no tasks; "mixed" if they differ).
	Driver string
	// Open is the streams open on all tasks.
	Open          int
	ByApplication map[string]int
	ByClient      map[string]int
	Lag           Histogram
	// Published is the pushes logged in the last minute, on all tasks.
	Published float64
	// Ingested is the last minute's ingest answers by application and result.
	Ingested map[string]map[string]float64
}

// SumStats sums rows into a Live.
func SumStats(rows []TaskRow) Live {
	l := Live{ByApplication: map[string]int{}, ByClient: map[string]int{}, Ingested: map[string]map[string]float64{}}
	for _, r := range rows {
		l.Tasks++
		if r.At.After(l.Newest) {
			l.Newest = r.At
		}
		switch {
		case l.Driver == "":
			l.Driver = r.Stats.Driver
		case l.Driver != r.Stats.Driver:
			l.Driver = "mixed"
		}
		l.Open += r.Stats.Open
		for k, v := range r.Stats.ByApplication {
			l.ByApplication[k] += v
		}
		for k, v := range r.Stats.ByClient {
			l.ByClient[k] += v
		}
		l.Lag = l.Lag.Add(r.Stats.Lag)
		l.Published += r.Stats.Published
		for app, results := range r.Stats.Ingested {
			if l.Ingested[app] == nil {
				l.Ingested[app] = map[string]float64{}
			}
			for res, v := range results {
				l.Ingested[app][res] += v
			}
		}
	}
	return l
}

// StatsRepo reads push_gateway_stats.
type StatsRepo struct{ Pool *pgxpool.Pool }

// Fresh returns the rows written within [StatsFresh] before now, oldest task id first.
func (r *StatsRepo) Fresh(ctx context.Context, now time.Time) ([]TaskRow, error) {
	rows, err := r.Pool.Query(ctx, `SELECT task_id, at, stats FROM push_gateway_stats WHERE at >= $1 ORDER BY task_id`, now.Add(-StatsFresh).UTC())
	if err != nil {
		return nil, fmt.Errorf("read push_gateway_stats: %w", err)
	}
	defer rows.Close()
	var out []TaskRow
	for rows.Next() {
		var tr TaskRow
		var raw []byte
		if err := rows.Scan(&tr.TaskID, &tr.At, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &tr.Stats); err != nil {
			continue // a row this version cannot read counts for nothing
		}
		out = append(out, tr)
	}
	return out, rows.Err()
}

// Live sums the fresh rows.
func (r *StatsRepo) Live(ctx context.Context, now time.Time) (Live, error) {
	rows, err := r.Fresh(ctx, now)
	if err != nil {
		return Live{}, err
	}
	return SumStats(rows), nil
}

// SortedCounts returns the entries of a count map largest first, ties by key.
func SortedCounts(m map[string]int) []Count {
	out := make([]Count, 0, len(m))
	for k, v := range m {
		out = append(out, Count{Key: k, N: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].N != out[j].N {
			return out[i].N > out[j].N
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// Count is one entry of a count map.
type Count struct {
	Key string
	N   int
}

// ErrNoGateway is what a gateway-only action answers when the gateway runs elsewhere.
var ErrNoGateway = errors.New("the push gateway does not run in this process")
