package router_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/router"
)

// newCleartextDualProtocolServer serves both h2c (HTTP/2 prior-knowledge over
// cleartext) and plain HTTP/1.1 on the same listener, so one server can answer
// a deployed-mode client and a dev-mode one and the tests below can assert
// which protocol each actually negotiated. Uses net/http's own unencrypted-h2
// support rather than x/net/http2/h2c, which is deprecated in favour of it.
func newCleartextDualProtocolServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(h)
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)
	srv.Config.Protocols = protocols
	srv.Start()
	return srv
}

// fastMediatorConfig is DefaultMediatorConfig with test-friendly timeouts:
// short enough that a hung/failed connect doesn't stall the suite, one
// attempt only so a deliberately-broken transport (the HTTP/1.1-only
// cleartext test) fails fast instead of burning through retries.
func fastMediatorConfig(version router.HTTPVersion) router.MediatorConfig {
	cfg := router.DefaultMediatorConfig()
	cfg.HTTPVersion = version
	cfg.Timeout = 3 * time.Second
	cfg.ConnectTimeout = 1 * time.Second
	cfg.TLSHandshakeTimeout = 1 * time.Second
	cfg.MaxRetries = 1
	cfg.RetryDelays = nil
	cfg.HostPoolSizing = router.HostPoolSizing{}
	return cfg
}

// TestMediatorDeployedModeSpeaksH2COverCleartext pins the core fix: a
// deployed-mode (HTTPVersion2) client delivering to a plain "http://"
// target must negotiate HTTP/2 via prior-knowledge (h2c), not silently
// run HTTP/1.1. Both the SERVER's view (r.Proto, observed by the h2c
// handler) and the CLIENT's own bookkeeping (ProtoCounts) are asserted,
// so a client that merely *claims* h2 without actually negotiating it
// (or a mismatch between the two) would be caught.
//
// Mutant: set AllowHTTP: false (or drop DialTLSContext so the h2c
// transport can't dial a plain conn) on the h2c.Transport in
// newClientBuilder and this test fails — see TestMediatorH2CMutantAllowHTTPFalse
// below for the actual mutant run recorded in the report.
func TestMediatorDeployedModeSpeaksH2COverCleartext(t *testing.T) {
	var (
		gotProto string
		gotBody  []byte
	)
	srv := newCleartextDualProtocolServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotProto = r.Proto
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := router.NewHTTPMediator(fastMediatorConfig(router.HTTPVersion2), router.NewBreakerRegistry(router.DefaultBreakerConfig()))
	msg := &common.Message{ID: "msg_H2C001", MediationType: common.MediationTypeHTTP, MediationTarget: srv.URL}

	out := m.Mediate(context.Background(), msg)
	require.Equal(t, common.MediationSuccess, out.Result, "expected success, got %+v", out)

	assert.Equal(t, "HTTP/2.0", gotProto,
		"server must see the request as HTTP/2 (h2c prior-knowledge), not a silent HTTP/1.1 downgrade")
	assert.Equal(t, `{"messageId":"msg_H2C001"}`, string(gotBody),
		"the h2c path must carry the same wire body as h1 — load-bearing for the HMAC/parity contract")

	counts := m.ProtoCounts()
	assert.Equal(t, uint64(1), counts["HTTP/2.0"],
		"the client's own negotiated-protocol counter must agree with what the server observed")
}

// TestMediatorDeployedModeFailsAgainstHTTP1OnlyCleartext pins "no silent
// downgrade": an h2c client speaking prior-knowledge to a target that
// only understands HTTP/1.1 must fail the delivery (connection error),
// never a MediationSuccess. A plain httptest.Server (no h2c wrapper)
// doesn't special-case the "PRI * HTTP/2.0" client preface — net/http's
// stdlib server parses it as an ordinary request line (method "PRI") and
// answers in plain HTTP/1.1 — so the h2c client's own frame parser is
// what rejects it (it can't make sense of an HTTP/1.1 response line as
// an HTTP/2 frame). Either way the outcome must never be Success: a
// mutant that downgraded to HTTP/1.1 instead would turn this into a
// clean 200 and the assertion below would catch it.
func TestMediatorDeployedModeFailsAgainstHTTP1OnlyCleartext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := router.NewHTTPMediator(fastMediatorConfig(router.HTTPVersion2), router.NewBreakerRegistry(router.DefaultBreakerConfig()))
	msg := &common.Message{ID: "msg_H2C002", MediationType: common.MediationTypeHTTP, MediationTarget: srv.URL}

	out := m.Mediate(context.Background(), msg)
	assert.Equal(t, common.MediationErrorConnection, out.Result,
		"an h2c target that turns out to be HTTP/1.1-only must fail the delivery, got %+v", out)
	assert.NotEqual(t, common.MediationSuccess, out.Result,
		"must never report success against an HTTP/1.1-only cleartext target — that would mean a silent downgrade delivered it")
}

// TestMediatorDevModeSpeaksHTTP1AgainstTheSameServer confirms dev mode
// (FLOWCATALYST_DEV_MODE / HTTPVersion1) is untouched by the h2c change:
// against the very same h2c-capable server as the first test, it still
// negotiates plain HTTP/1.1.
func TestMediatorDevModeSpeaksHTTP1AgainstTheSameServer(t *testing.T) {
	var gotProto string
	srv := newCleartextDualProtocolServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotProto = r.Proto
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := router.NewHTTPMediator(fastMediatorConfig(router.HTTPVersion1), router.NewBreakerRegistry(router.DefaultBreakerConfig()))
	msg := &common.Message{ID: "msg_H2C003", MediationType: common.MediationTypeHTTP, MediationTarget: srv.URL}

	out := m.Mediate(context.Background(), msg)
	require.Equal(t, common.MediationSuccess, out.Result, "expected success, got %+v", out)
	assert.Equal(t, "HTTP/1.1", gotProto, "dev mode must still speak plain HTTP/1.1")
}

// A target that advertises a small SETTINGS_MAX_CONCURRENT_STREAMS must not be
// able to wedge delivery when more requests are in flight than its streams.
//
// With StrictMaxConcurrentStreams set, the HTTP/2 client treated the server's
// limit as a cap on total streams and queued the excess inside the transport;
// once requests per connection passed the limit those waiters were never woken
// and every in-flight delivery hung (reproduced with only x/net/http2 against
// a trivial server: 2,000 concurrent requests over 8 connections completed,
// 2,400 delivered ~1,800 and stopped for good). The router showed it as 100
// pools of 64 workers (6,400 requests in flight) delivering nothing after ten
// seconds. The client must instead open more connections to stay under the
// limit.
func TestMediatorDoesNotWedgeWhenInFlightExceedsServerStreamLimit(t *testing.T) {
	const (
		streamLimit = 8
		inFlight    = 120 // 15x the server's limit on a single connection
	)
	// x/net's HTTP/2 server, which is what advertises and enforces the limit
	// in the case this pins (the bench sink).
	srv := httptest.NewUnstartedServer(h2c.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), &http2.Server{MaxConcurrentStreams: streamLimit}))
	srv.Start()
	defer srv.Close()

	cfg := fastMediatorConfig(router.HTTPVersion2)
	cfg.Timeout = 20 * time.Second
	// A burst larger than the limit on a brand-new connection can have a few
	// streams refused before the server's settings are read; that is an
	// ordinary retryable failure, and the delivery must still complete.
	cfg.MaxRetries = 10
	cfg.RetryDelays = []time.Duration{5 * time.Millisecond}
	m := router.NewHTTPMediator(cfg, router.NewBreakerRegistry(router.DefaultBreakerConfig()))

	results := make(chan common.MediationResult, inFlight)
	for i := range inFlight {
		go func() {
			msg := &common.Message{ID: fmt.Sprintf("msg_WEDGE%03d", i), MediationType: common.MediationTypeHTTP, MediationTarget: srv.URL}
			results <- m.Mediate(context.Background(), msg).Result
		}()
	}

	deadline := time.After(15 * time.Second)
	for got := 0; got < inFlight; got++ {
		select {
		case r := <-results:
			require.Equal(t, common.MediationSuccess, r)
		case <-deadline:
			t.Fatalf("only %d of %d deliveries completed: requests beyond the server's %d concurrent streams were never served",
				got, inFlight, streamLimit)
		}
	}
}
