package api_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/router"
	routerapi "github.com/flowcatalyst/flowcatalyst-go/internal/router/api"
)

// headerGate admits a request carrying X-Test-Debug: yes, and nothing else —
// a stand-in for the server's real gate (router Basic auth, or an anchor
// bearer), which internal/server tests on its own.
func headerGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Test-Debug") != "yes" {
			http.Error(w, "denied", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func debugServer(t *testing.T, s *routerapi.State) *httptest.Server {
	t.Helper()
	r := chi.NewRouter()
	r.Route("/router", func(sub chi.Router) { routerapi.MountDebug(sub, s, headerGate) })
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

func get(t *testing.T, url string, allowed bool) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	if allowed {
		req.Header.Set("X-Test-Debug", "yes")
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	var b strings.Builder
	_, _ = io.Copy(&b, resp.Body)
	return resp.StatusCode, b.String()
}

// The debug surface is never mounted without a gate.
func TestMountDebugRefusesToMountWithoutAGate(t *testing.T) {
	assert.Panics(t, func() { routerapi.MountDebug(chi.NewRouter(), &routerapi.State{}, nil) })
}

// Every debug path goes through the gate.
func TestDebugEndpointsAreGated(t *testing.T) {
	srv := debugServer(t, &routerapi.State{})
	for _, p := range []string{
		"/router/debug/", "/router/debug/dump", "/router/debug/vars",
		"/router/debug/pprof/", "/router/debug/pprof/goroutine?debug=2", "/router/debug/pprof/heap",
		"/router/debug/pprof/cmdline",
	} {
		code, _ := get(t, srv.URL+p, false)
		assert.Equal(t, http.StatusUnauthorized, code, "%s must not be reachable without the gate's credential", p)
	}
}

// The dump names the message and group each worker is on, lists the held
// groups, and then carries every goroutine's stack.
func TestDebugDumpNamesWhatEachWorkerIsOn(t *testing.T) {
	srv := debugServer(t, &routerapi.State{
		Mediating: stubMediatingProvider{entries: []router.MediatingEntry{
			{MessageID: "msg-stuck", PoolCode: "ORD", Group: "order-42", Queue: "q-orders", Target: "http://t", MediatedAt: nowMinus(90)},
		}},
		BlockedGroups: stubBlockedGroups{rows: []router.GroupInfo{
			{PoolCode: "ORD", Group: "order-42", Buffered: 3, Working: true},
		}},
	})
	code, body := get(t, srv.URL+"/router/debug/dump", true)
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, body, "Mediating: 1 worker(s)")
	assert.Contains(t, body, "msg-stuck")
	assert.Contains(t, body, "order-42")
	assert.Contains(t, body, "q-orders")
	assert.Contains(t, body, "Message groups holding buffered work: 1")
	assert.Contains(t, body, "== Goroutines")
	assert.Contains(t, body, "goroutine ", "the full stack dump follows")
}

func TestDebugPprofAndExpvar(t *testing.T) {
	srv := debugServer(t, &routerapi.State{})
	code, body := get(t, srv.URL+"/router/debug/pprof/goroutine?debug=2", true)
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, body, "goroutine ")

	code, body = get(t, srv.URL+"/router/debug/pprof/", true)
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, body, "heap")

	code, _ = get(t, srv.URL+"/router/debug/pprof/heap", true)
	assert.Equal(t, http.StatusOK, code)
	code, _ = get(t, srv.URL+"/router/debug/pprof/no-such-profile", true)
	assert.Equal(t, http.StatusNotFound, code)

	code, body = get(t, srv.URL+"/router/debug/vars", true)
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, body, `"memstats"`)
	assert.Contains(t, body, `"router"`)
}

// Importing net/http/pprof and expvar registers their handlers on
// http.DefaultServeMux; this package empties that mux, so nothing that ever
// served it by mistake would expose them anonymously.
func TestDefaultServeMuxDoesNotServeTheDebugHandlers(t *testing.T) {
	for _, p := range []string{"/debug/pprof/", "/debug/vars"} {
		rec := httptest.NewRecorder()
		http.DefaultServeMux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		assert.Equal(t, http.StatusNotFound, rec.Code, p)
	}
}

type stubEventCounters struct{ ev router.RouterEventCounts }

func (s stubEventCounters) EventCounters() router.RouterEventCounts { return s.ev }

// The scrape now carries the Go runtime and process collectors and the
// event-time counters the contract defines.
func TestPrometheusHandler_RuntimeAndEventCounters(t *testing.T) {
	pools := stubPoolStatsProvider{stats: []router.PoolStats{{
		PoolCode: "demo", Concurrency: 1,
		Metrics: &common.EnhancedPoolMetrics{TotalSuccess: 7, TotalFailure: 2},
	}}}
	state := &routerapi.State{
		PoolStats: pools, Mocks: routerapi.NewMockState(),
		EventCounters: stubEventCounters{ev: router.RouterEventCounts{
			Pools: []router.PoolEventCounts{{
				Pool: "demo", Submitted: 12,
				Processed: map[string]uint64{"SUCCESS": 7, "ERROR_CONFIG": 1, "ERROR_CONNECTION": 1},
				Rejected:  map[string]uint64{"capacity": 2, "released": 1},
			}},
			Consumers: []router.ConsumerEventCounts{{
				Queue: "https://sqs.eu-west-1.amazonaws.com/1/q-orders", Polls: 40,
				Errors: map[string]uint64{"poll": 3, "panic": 1},
			}},
			PanicsRecovered: 1,
		}},
	}
	rec := httptest.NewRecorder()
	routerapi.PrometheusHandler(state).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	body := rec.Body.String()
	for _, want := range []string{
		"go_goroutines ",
		"go_memstats_heap_alloc_bytes ",
		`fc_messages_submitted_total{pool="demo"} 12`,
		`fc_messages_rejected_total{pool="demo",reason="capacity"} 2`,
		`fc_messages_rejected_total{pool="demo",reason="released"} 1`,
		`fc_messages_processed_total{pool="demo",result="SUCCESS",success="true"} 7`,
		`fc_messages_processed_total{pool="demo",result="ERROR_CONFIG",success="false"} 1`,
		`fc_consumer_polls_total{queue="q-orders"} 40`,
		`fc_consumer_errors_total{queue="q-orders",type="poll"} 3`,
		`fc_consumer_errors_total{queue="q-orders",type="panic"} 1`,
		"fc_router_panics_recovered_total 1",
	} {
		assert.Contains(t, body, want)
	}
	assert.NotContains(t, body, `fc_messages_processed_total{pool="demo",success="true"}`,
		"one label set for the family")
}
