package pushgw

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/framework/go/metrics"
	"github.com/flowcatalyst/framework/go/push"
)

func TestHistogramQuantile(t *testing.T) {
	h := Histogram{Bounds: []float64{0.01, 0.1, 1}, Counts: []uint64{0, 90, 9, 1}}
	assert.Equal(t, uint64(100), h.Total())
	p50, ok := h.Quantile(0.5)
	require.True(t, ok)
	assert.InDelta(t, 0.01+(0.1-0.01)*50.0/90.0, p50, 1e-9, "interpolated inside the bucket that holds the rank")
	p99, _ := h.Quantile(0.99)
	assert.InDelta(t, 0.1+(1-0.1)*9.0/9.0, p99, 1e-9)
	p100, _ := h.Quantile(1)
	assert.Equal(t, 1.0, p100, "the overflow counts as the last bound")

	_, ok = Histogram{}.Quantile(0.5)
	assert.False(t, ok, "no observations, no quantile")

	sum := h.Add(Histogram{Bounds: h.Bounds, Counts: []uint64{1, 1, 1, 1}})
	assert.Equal(t, []uint64{1, 91, 10, 2}, sum.Counts)
	assert.Equal(t, []uint64{0, 90, 9, 1}, h.Counts, "Add does not change its operands")
	assert.Equal(t, h.Counts, Histogram{}.Add(h).Counts)
	assert.Equal(t, h.Counts, h.Add(Histogram{Bounds: []float64{1}, Counts: []uint64{5, 5}}).Counts, "a histogram of other bounds is ignored")
}

func TestSumStatsAddsTheTasks(t *testing.T) {
	now := time.Now()
	l := SumStats([]TaskRow{
		{TaskID: "a", At: now.Add(-3 * time.Second), Stats: TaskStats{
			Driver: "postgres", Open: 2, ByApplication: map[string]int{"alpha": 2}, ByClient: map[string]int{"c1": 2}, Published: 5,
			Ingested: map[string]map[string]float64{"alpha": {"accepted": 4}},
		}},
		{TaskID: "b", At: now.Add(-1 * time.Second), Stats: TaskStats{
			Driver: "postgres", Open: 3, ByApplication: map[string]int{"alpha": 1, "beta": 2}, ByClient: map[string]int{"c1": 1, "c2": 2}, Published: 7,
			Ingested: map[string]map[string]float64{"alpha": {"accepted": 6, "refused": 1}},
		}},
	})
	assert.Equal(t, 2, l.Tasks)
	assert.Equal(t, 5, l.Open)
	assert.Equal(t, map[string]int{"alpha": 3, "beta": 2}, l.ByApplication)
	assert.Equal(t, map[string]int{"c1": 3, "c2": 2}, l.ByClient)
	assert.InDelta(t, 12, l.Published, 0)
	assert.Equal(t, map[string]map[string]float64{"alpha": {"accepted": 10, "refused": 1}}, l.Ingested)
	assert.Equal(t, "postgres", l.Driver)
	assert.WithinDuration(t, now.Add(-1*time.Second), l.Newest, 0)
	assert.Equal(t, 0, SumStats(nil).Tasks)

	mixed := SumStats([]TaskRow{{Stats: TaskStats{Driver: "postgres"}}, {Stats: TaskStats{Driver: "redis"}}})
	assert.Equal(t, "mixed", mixed.Driver)

	got := SortedCounts(map[string]int{"b": 2, "a": 2, "c": 9})
	assert.Equal(t, []Count{{"c", 9}, {"a", 2}, {"b", 2}}, got)
}

func streamToken(t *testing.T, client string, topics ...string) string {
	raw, err := json.Marshal(map[string]any{"v": 1, "c": client, "t": topics, "exp": 4102444800})
	require.NoError(t, err)
	return base64.RawURLEncoding.EncodeToString(raw) + ".sig"
}

func TestTokenFacts(t *testing.T) {
	apps, client, ok := tokenFacts(streamToken(t, "clt_one", "alpha:a:1", "alpha:b", "beta:x"))
	require.True(t, ok)
	assert.Equal(t, []string{"alpha", "beta"}, apps, "one entry per application, in topic order")
	assert.Equal(t, "clt_one", client)
	for name, st := range map[string]string{"no dot": "abc", "not base64": "!!.x", "not json": base64.RawURLEncoding.EncodeToString([]byte("x")) + ".s", "empty": ""} {
		_, _, ok := tokenFacts(st)
		assert.False(t, ok, name)
	}
	_, _, ok = tokenFacts(streamToken(t, "", "alpha:a"))
	assert.False(t, ok, "no client")
	_, _, ok = tokenFacts(streamToken(t, "c"))
	assert.False(t, ok, "no topics")
}

// A stream is counted while the module answers 200 and the request lasts; a refusal, another path
// or an unreadable token is never counted.
func TestStreamCounterCountsOpenStreams(t *testing.T) {
	c := newStreamCounter()
	release := make(chan struct{})
	opened := make(chan struct{}, 4)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("deny") != "" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_ = http.NewResponseController(w).Flush() // the module flushes through the controller
		opened <- struct{}{}
		<-release
	})
	srv := httptest.NewServer(c.wrap(inner))
	defer srv.Close()

	get := func(path, st string, extra string) *http.Response {
		res, err := http.Get(srv.URL + path + "?st=" + st + extra)
		require.NoError(t, err)
		return res
	}
	a := streamToken(t, "clt_one", "alpha:a:1", "beta:x")
	b := streamToken(t, "clt_two", "alpha:a:2")
	r1 := get("/api/push/stream", a, "")
	r2 := get("/api/push/stream", b, "")
	<-opened
	<-opened
	byApp, byClient := c.snapshot()
	assert.Equal(t, map[string]int{"alpha": 2, "beta": 1}, byApp)
	assert.Equal(t, map[string]int{"clt_one": 1, "clt_two": 1}, byClient)

	get("/api/push/stream", a, "&deny=1").Body.Close() // refused: not counted
	get("/api/push/keys", a, "").Body.Close()          // not a stream
	byApp, _ = c.snapshot()
	assert.Equal(t, 2, byApp["alpha"])

	close(release)
	_, _ = r1.Body.Read(make([]byte, 1))
	_ = r1.Body.Close()
	_ = r2.Body.Close()
	assert.Eventually(t, func() bool {
		byApp, byClient := c.snapshot()
		return len(byApp) == 0 && len(byClient) == 0
	}, 3*time.Second, 10*time.Millisecond, "closed streams are not counted")
}

// The collector reads the registry: gauges as they are, the relay lag, publishing and ingest as the
// difference over the window.
func TestStatsCollectorWindows(t *testing.T) {
	reg, err := metrics.NewRegistry()
	require.NoError(t, err)
	published, _ := reg.Counter("fw_push_published_total", "x")
	lag, _ := reg.Timer("fw_push_relay_lag_seconds", "x")
	ingest, _ := reg.Counter("fc_push_ingest_items_total", "x", "application", "result")
	open, _ := reg.Gauge("fw_push_streams_open", "x")

	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	sc := newStreamCounter()
	sc.add([]string{"alpha"}, "clt_one", 1)
	col := &statsCollector{driver: "postgres", reg: reg, streams: sc, now: func() time.Time { return now }}

	published.Add(10) // before the first reading: not in the window
	lag.Record(2 * time.Millisecond)
	ingest.With("alpha", "accepted").Add(3)
	first := col.collect()
	assert.Zero(t, first.Published, "the first reading has nothing before it")
	assert.Zero(t, first.Lag.Total())

	now = now.Add(10 * time.Second)
	published.Add(4)
	lag.Record(7 * time.Millisecond)
	lag.Record(300 * time.Millisecond)
	ingest.With("alpha", "accepted").Add(2)
	ingest.With("alpha", "refused").Inc()
	open.Set(1)
	st := col.collect()
	assert.InDelta(t, 4, st.Published, 0)
	assert.Equal(t, uint64(2), st.Lag.Total())
	assert.Equal(t, 1, st.Open)
	assert.Equal(t, "postgres", st.Driver)
	assert.Equal(t, map[string]int{"alpha": 1}, st.ByApplication)
	assert.Equal(t, map[string]map[string]float64{"alpha": {"accepted": 2, "refused": 1}}, st.Ingested)
	assert.InDelta(t, 10, st.WindowSecs, 0.001)
	p99, ok := st.Lag.Quantile(0.99)
	require.True(t, ok)
	assert.Greater(t, p99, 0.1)

	// after more than a minute the old readings fall out of the window
	now = now.Add(2 * time.Minute)
	later := col.collect()
	assert.Zero(t, later.Published, "a quiet minute publishes nothing")
	assert.Zero(t, later.Lag.Total())
	assert.Less(t, len(col.history), 4)

	// the row is JSON the console can read
	raw, err := json.Marshal(st)
	require.NoError(t, err)
	var back TaskStats
	require.NoError(t, json.Unmarshal(raw, &back))
	assert.Equal(t, st.Lag.Counts, back.Lag.Counts)
}

// fakeModuleLog is the module's log as ModuleLog reads it: the newest entries, in id order.
type fakeModuleLog struct {
	push.Log
	entries []push.Entry
}

func (f fakeModuleLog) Newest(_ context.Context, limit int) ([]push.Entry, error) {
	return f.entries[max(0, len(f.entries)-limit):], nil
}

// ModuleLog (the Redis driver's reader) answers the same queries as the SQL reader.
func TestModuleLogRecent(t *testing.T) {
	at := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	l := ModuleLog{Log: fakeModuleLog{entries: []push.Entry{
		{ID: 1, Topic: "alpha:order:1", Type: "order.updated", Client: "clt_acme", Data: `{"v":1}`, LoggedAt: at},
		{ID: 2, Topic: "beta:thing:1", Type: "thing.moved", Client: "clt_acme", Data: `{"v":2}`, LoggedAt: at},
		{ID: 3, Topic: "alpha:news", Type: "news.posted", Client: "", Data: `{"v":3}`, LoggedAt: at},
		{ID: 4, Topic: "alpha:order:2", Type: "order.created", Client: "clt_other", Data: `{"v":4}`, LoggedAt: at},
	}}}
	ids := func(q LogQuery) []int64 {
		rows, err := l.Recent(context.Background(), q)
		require.NoError(t, err)
		var out []int64
		for _, r := range rows {
			out = append(out, r.ID)
		}
		return out
	}
	assert.Equal(t, []int64{4, 3, 1}, ids(LogQuery{Application: "alpha"}), "newest first, one application")
	assert.Equal(t, []int64{3}, ids(LogQuery{Application: "alpha", Client: "_platform"}))
	assert.Equal(t, []int64{1}, ids(LogQuery{Application: "alpha", Client: "clt_acme"}))
	assert.Equal(t, []int64{4}, ids(LogQuery{Application: "alpha", Type: "order.created"}))
	assert.Equal(t, []int64{4, 1}, ids(LogQuery{Application: "alpha", Topic: "order"}))
	assert.Equal(t, []int64{1}, ids(LogQuery{Application: "alpha", BeforeID: 3, Limit: 5}))
	assert.Equal(t, []int64{4}, ids(LogQuery{Application: "alpha", Limit: 1}))
	assert.Empty(t, ids(LogQuery{Application: "alph"}))
}
