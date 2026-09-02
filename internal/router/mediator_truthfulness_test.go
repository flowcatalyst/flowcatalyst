package router_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/router"
)

// These cover the same ground as the shared conformance corpus, deliberately
// duplicated as plain Go tests: the corpus lives in the Java repo and the
// runner SKIPS when it is not checked out alongside, so relying on it alone
// would leave these behaviours unguarded in a standalone Go build.

// mediatorFor builds a mediator with its own breaker registry and warning
// service, and returns all three so a test can read back what was recorded.
func mediatorFor(t *testing.T) (*router.HTTPMediator, *router.BreakerRegistry, *router.WarningService) {
	t.Helper()
	breakers := router.NewBreakerRegistry(router.DefaultBreakerConfig())
	ws := router.NewWarningService(router.DefaultWarningServiceConfig())
	med := router.NewHTTPMediator(router.DevMediatorConfig(), breakers)
	med.SetWarnings(ws)
	t.Cleanup(med.Close)
	return med, breakers, ws
}

func httpMsg(target string) *common.Message {
	return &common.Message{ID: "evt_1", MediationType: common.MediationTypeHTTP, MediationTarget: target}
}

// The status is the target's own answer. Flattening every success to 200
// discarded information the router has no reason to hold an opinion about —
// and an operator reading a delivery log could not tell a 204 from a 200, nor
// notice a target that had switched to 202.
func TestSuccessCarriesTheRealStatus(t *testing.T) {
	for _, status := range []int{http.StatusCreated, http.StatusAccepted, http.StatusNoContent} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		}))
		med, _, _ := mediatorFor(t)

		out := med.Mediate(context.Background(), httpMsg(srv.URL))

		assert.Equal(t, common.MediationSuccess, out.Result)
		assert.Equalf(t, status, out.StatusCode, "a %d must be reported as a %d", status, status)
		srv.Close()
	}
}

// The flushGroup branch is the one that regressed: it was added later and
// built its outcome the same hard-coded way, so a flushing 201 reported 200.
func TestFlushGroupSuccessAlsoCarriesTheRealStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ack":true,"flushGroup":true,"delaySeconds":20}`))
	}))
	defer srv.Close()
	med, _, _ := mediatorFor(t)

	out := med.Mediate(context.Background(), httpMsg(srv.URL))

	assert.Equal(t, common.MediationSuccess, out.Result)
	assert.Equal(t, http.StatusCreated, out.StatusCode, "the flushing branch must carry the status too")
	assert.True(t, out.FlushGroup)
	assert.Equal(t, 20, out.DelaySeconds)
}

// A redirect the client does not follow is permanent: the target reproduces it
// identically every time, so the old retryable classification meant retrying a
// 301 for ever — and reporting status 0 while doing it.
func TestUnfollowedRedirectIsPermanent(t *testing.T) {
	for _, status := range []int{http.StatusMovedPermanently, http.StatusFound, http.StatusTemporaryRedirect} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Location", "http://elsewhere.invalid/hook")
			w.WriteHeader(status)
		}))
		med, breakers, ws := mediatorFor(t)

		out := med.Mediate(context.Background(), httpMsg(srv.URL))

		assert.Equalf(t, common.MediationErrorConfig, out.Result,
			"a %d must be terminal, not retried for ever", status)
		assert.Equal(t, status, out.StatusCode)
		assert.NotEmpty(t, ws.BySeverity(router.WarningError), "a permanent drop must warn")
		assert.Equal(t, uint64(1), breakers.Get(srv.URL).Stats().Successes,
			"the target answered, so it is reachable — the breaker must not trip")
		srv.Close()
	}
}

// The redirect must not be FOLLOWED, which is a separate property from how it
// is classified — and the more dangerous one.
//
// Go's http.Client follows up to ten redirects by default, and for 301/302/303
// it rewrites a POST to a GET and drops the body. So a target answering "301,
// go here instead" had its message delivered to that other URL as an empty GET,
// and if that URL answered 2xx the router recorded a successful delivery of a
// message whose payload nobody ever received.
func TestRedirectIsNotFollowed(t *testing.T) {
	var elsewhereHits atomic.Int64
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		elsewhereHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer elsewhere.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", elsewhere.URL)
		w.WriteHeader(http.StatusMovedPermanently)
	}))
	defer srv.Close()
	med, _, ws := mediatorFor(t)

	out := med.Mediate(context.Background(), httpMsg(srv.URL))

	assert.Zero(t, elsewhereHits.Load(),
		"the message must not be delivered to a URL nobody configured")
	assert.Equal(t, common.MediationErrorConfig, out.Result,
		"the redirect is reported as the misconfiguration it is")
	assert.Equal(t, http.StatusMovedPermanently, out.StatusCode)
	assert.NotEmpty(t, ws.BySeverity(router.WarningError))
}

// Every 4xx is a permanent ACK-drop, so every 4xx warns. Naming a handful and
// leaving the rest to a log line meant an operator heard about a 404 and not
// about a 422, for identical consequences.
func TestUnnamed4xxWarnsLikeTheNamedOnes(t *testing.T) {
	for _, status := range []int{http.StatusPaymentRequired, http.StatusMethodNotAllowed, http.StatusTeapot, http.StatusUnprocessableEntity} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		}))
		med, _, ws := mediatorFor(t)

		out := med.Mediate(context.Background(), httpMsg(srv.URL))

		require.Equal(t, common.MediationErrorConfig, out.Result)
		assert.Equal(t, status, out.StatusCode)
		assert.NotEmptyf(t, ws.BySeverity(router.WarningError),
			"a %d is dropped permanently and the warning is its only trace", status)
		srv.Close()
	}
}

// A rejection decided before anything went out is no evidence about the
// target's health in either direction. Recording one as a breaker success
// actively masks a failing endpoint: a misconfigured URL would hold the
// breaker closed over a host that is down.
func TestPreFlightRejectionsLeaveTheBreakerAlone(t *testing.T) {
	cases := []struct {
		name string
		msg  *common.Message
	}{
		{
			name: "unsupported mediation type",
			msg: &common.Message{
				ID: "evt_1", MediationType: common.MediationType("GRPC"),
				MediationTarget: "http://target.invalid/hook",
			},
		},
		{
			name: "target URL with no host",
			msg:  httpMsg("not-a-url"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			med, breakers, ws := mediatorFor(t)

			out := med.Mediate(context.Background(), tc.msg)

			require.Equal(t, common.MediationErrorConfig, out.Result)
			assert.True(t, out.PreFlight, "the outcome must record that no call was made")
			assert.NotEmpty(t, ws.BySeverity(router.WarningError),
				"it ACK-drops every message routed through it and an operator can fix it")

			st := breakers.Get(tc.msg.MediationTarget).Stats()
			assert.Zerof(t, st.Successes+st.Failures,
				"a call that never happened must not be recorded either way; got %+v", st)
		})
	}
}

// TestBreakerKeyIgnoresQueryAndFragment pins the circuit-breaker key to
// origin+path: two targets on the same endpoint that differ only by query
// string or fragment must share one breaker, so a genuinely dead endpoint
// accumulates its failures on a single key instead of starting a fresh
// window per distinct query string. A different path is a different
// breaker — the key must not over-collapse either.
func TestBreakerKeyIgnoresQueryAndFragment(t *testing.T) {
	med, breakers, _ := mediatorFor(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	targets := []string{
		srv.URL + "/hook?tenant=alpha",
		srv.URL + "/hook?tenant=beta&other=1",
		srv.URL + "/hook#section",
		srv.URL + "/hook",
	}
	for _, target := range targets {
		out := med.Mediate(context.Background(), httpMsg(target))
		require.Equal(t, common.MediationSuccess, out.Result)
	}

	assert.Equal(t, 1, breakers.Len(),
		"distinct query strings/fragments on the same origin+path must share one breaker")
	cb := breakers.Get(srv.URL + "/hook")
	assert.Equal(t, uint64(len(targets)), cb.Stats().Successes,
		"all deliveries must have landed on the same breaker key")

	// Negative control: a different path must NOT collapse into the same key.
	other := srv.URL + "/other"
	out := med.Mediate(context.Background(), httpMsg(other))
	require.Equal(t, common.MediationSuccess, out.Result)
	assert.Equal(t, 2, breakers.Len(), "a different path must get its own breaker")
}

// TestBreakerKeyFallsBackOnUnparseableTarget pins the fallback: a target
// string that url.Parse rejects outright keys the breaker by the raw string,
// same as before this change, rather than panicking or collapsing onto some
// other key.
//
// This target fails even earlier than HostKeyFromURL: http.NewRequestWithContext
// parses the URL itself and errors on the bad percent-encoding, so the
// outcome comes from the "build request" pre-flight path — see
// TestUnparseableTargetWarnsLikeAnyOtherPreFlightRejection for the
// classification/warning pin on that path.
func TestBreakerKeyFallsBackOnUnparseableTarget(t *testing.T) {
	med, breakers, _ := mediatorFor(t)
	// Invalid percent-encoding: net/url.Parse errors on this outright.
	target := "http://target.invalid/%zz"

	out := med.Mediate(context.Background(), httpMsg(target))
	require.Equal(t, common.MediationErrorConfig, out.Result, "an unparseable target is rejected pre-flight, permanently")
	require.True(t, out.PreFlight)

	assert.Equal(t, 1, breakers.Len())
	// Re-Get with the exact same raw string must hit the same entry (not
	// create a second one), proving the fallback key is the raw string.
	_ = breakers.Get(target)
	assert.Equal(t, 1, breakers.Len(), "fallback key must be the raw target string")
}

// TestUnparseableTargetWarnsLikeAnyOtherPreFlightRejection pins R-06 for the
// "build request" pre-flight path specifically: an outright-unparseable
// target string (distinct from "parses fine but has no host", which
// TestPreFlightRejectionsLeaveTheBreakerAlone already covers) must still
// warn and must still leave the breaker untouched.
func TestUnparseableTargetWarnsLikeAnyOtherPreFlightRejection(t *testing.T) {
	med, breakers, ws := mediatorFor(t)
	target := "http://target.invalid/%zz"

	out := med.Mediate(context.Background(), httpMsg(target))

	require.Equal(t, common.MediationErrorConfig, out.Result)
	assert.True(t, out.PreFlight, "the outcome must record that no call was made")
	assert.NotEmpty(t, ws.BySeverity(router.WarningError),
		"an unparseable target is a configuration mistake an operator can fix")

	st := breakers.Get(target).Stats()
	assert.Zerof(t, st.Successes+st.Failures,
		"a call that never happened must not be recorded either way; got %+v", st)
}

// TestServerErrorClassificationBoundary pins the R-57 boundary: 502/503/504
// mean "target unavailable" (never reached a working app) and are held at
// the broker with backoff; every OTHER 5xx means the app was reached and
// answered, and is a permanent ACK-drop with the same warning treatment as a
// 4xx. 501 keeps its pre-existing CRITICAL-severity permanent classification
// unchanged (see TestMediatorNotImplementedIsConfigError for that pin).
func TestServerErrorClassificationBoundary(t *testing.T) {
	cases := []struct {
		status        int
		wantRetryable bool // true: MediationErrorProcess, held/retried. false: MediationErrorConfig, permanent.
	}{
		{499, false}, // non-standard "client closed request"; already 4xx-range, sanity check
		{500, false}, // app ran and threw
		{501, false}, // app reached, endpoint not implemented (pre-existing, unchanged)
		{502, true},  // bad gateway — never reached a working app
		{503, true},  // service unavailable — never reached a working app
		{504, true},  // gateway timeout — never reached a working app
		{505, false}, // HTTP version not supported — app answered
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("status_%d", tc.status), func(t *testing.T) {
			var attempts atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				attempts.Add(1)
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()

			cfg := router.DevMediatorConfig()
			cfg.MaxRetries = 2
			cfg.RetryDelays = []time.Duration{1 * time.Millisecond}
			breakers := router.NewBreakerRegistry(router.DefaultBreakerConfig())
			ws := router.NewWarningService(router.DefaultWarningServiceConfig())
			med := router.NewHTTPMediator(cfg, breakers)
			med.SetWarnings(ws)
			t.Cleanup(med.Close)

			out := med.Mediate(context.Background(), httpMsg(srv.URL))

			assert.Equal(t, tc.status, out.StatusCode)
			st := breakers.Get(srv.URL).Stats()
			if tc.wantRetryable {
				assert.Equal(t, common.MediationErrorProcess, out.Result)
				assert.Equal(t, int32(2), attempts.Load(),
					"target-unavailable class must run the retry burst (MaxRetries=2)")
				assert.Equal(t, uint64(1), st.Failures, "unavailable class must count as a breaker failure")
				assert.Zero(t, st.Successes)
			} else {
				assert.Equal(t, common.MediationErrorConfig, out.Result)
				assert.Equal(t, int32(1), attempts.Load(),
					"a permanent rejection must not retry in-pipeline")
				assert.Equal(t, uint64(1), st.Successes,
					"the app answered, so it is reachable — must count as a breaker success")
				assert.Zero(t, st.Failures)
				// 501 is the pre-existing deliberate exception: it warns at
				// CRITICAL (a deployment/routing mistake), not ERROR like
				// every other permanent 5xx.
				severity := router.WarningError
				if tc.status == 501 {
					severity = router.WarningCritical
				}
				assert.NotEmptyf(t, ws.BySeverity(severity),
					"a permanent 5xx drop must warn (severity %v)", severity)
			}
		})
	}
}

// TestMediatorTransportFailureRetries pins the transport-failure leg of the
// R-57 "target unavailable" class alongside 502/503/504: a connection that
// accepts and then drops (no HTTP response at all) must retry the burst,
// just like a gateway error, and record a breaker failure — never a
// success, since no application ever answered.
func TestMediatorTransportFailureRetries(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	var accepts atomic.Int32
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			accepts.Add(1)
			_ = conn.Close() // drop it: no HTTP response ever sent
		}
	}()

	cfg := router.DevMediatorConfig()
	cfg.MaxRetries = 2
	cfg.RetryDelays = []time.Duration{1 * time.Millisecond}
	breakers := router.NewBreakerRegistry(router.DefaultBreakerConfig())
	med := router.NewHTTPMediator(cfg, breakers)
	t.Cleanup(med.Close)

	target := "http://" + ln.Addr().String() + "/hook"
	out := med.Mediate(context.Background(), httpMsg(target))

	assert.Equal(t, common.MediationErrorConnection, out.Result)
	assert.GreaterOrEqualf(t, accepts.Load(), int32(2),
		"transport failure must run the retry burst (MaxRetries=2), got %d attempts", accepts.Load())

	st := breakers.Get(target).Stats()
	assert.Equal(t, uint64(1), st.Failures, "transport failure must count as a breaker failure")
	assert.Zero(t, st.Successes)
}
