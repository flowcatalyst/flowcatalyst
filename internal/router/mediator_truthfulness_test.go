package router_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

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
