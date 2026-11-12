package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/netguard"
)

// withStrictPolicies resets the two package-level policies to the strict,
// exemption-free state for one test and restores them afterwards.
func withStrictPolicies(t *testing.T) (customer, internal *netguard.Policy) {
	t.Helper()
	d, i := *netguard.Default, *netguard.Internal
	*netguard.Default, *netguard.Internal = netguard.Policy{}, netguard.Policy{}
	t.Cleanup(func() { *netguard.Default, *netguard.Internal = d, i })
	return netguard.Default, netguard.Internal
}

// The platform's own endpoints are exempt on the internal policy only: a
// customer-supplied URL naming the API listener, the dispatch callback or the
// runner is refused like any other loopback address.
func TestApplyDeliveryPolicy_ExemptsTheRunnerOnTheInternalPolicyOnly(t *testing.T) {
	cfg := EnvCfg{
		FunctionsRunnerURL:         "http://127.0.0.1:8095",
		DispatchProcessingEndpoint: "http://localhost:8080/api/dispatch/process",
	}
	customer, internal := withStrictPolicies(t)
	cfg.ApplyDeliveryPolicy()

	for _, u := range []string{
		"http://localhost:8080/router/messages",
		"http://127.0.0.1:8095/fn/x",
		"http://localhost:8080/api/dispatch/process",
	} {
		err := customer.ValidateURL(u)
		require.Error(t, err, u)
		assert.ErrorIs(t, err, netguard.ErrBlocked, u)
	}
	assert.NoError(t, internal.ValidateURL("http://127.0.0.1:8095/fn/x"), "the runner is exempt on the internal policy")
	assert.Error(t, internal.ValidateURL("http://localhost:8080/api/dispatch/process"),
		"nothing but the router dials the dispatch callback, so it is exempt nowhere")
	assert.Error(t, internal.ValidateURL("http://localhost:8080/router/messages"))
	assert.NoError(t, customer.ValidateURL("https://example.com/hook"))
	assert.NoError(t, internal.ValidateURL("https://example.com/hook"))
}

// The dial refuses the runner's own address for a customer, even when that
// address was handed to ApplyDeliveryPolicy; the internal dialer connects.
func TestApplyDeliveryPolicy_CustomerDialerRefusesTheRunner(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	t.Cleanup(srv.Close)

	customer, internal := withStrictPolicies(t)
	EnvCfg{FunctionsRunnerURL: srv.URL}.ApplyDeliveryPolicy()
	addr := srv.Listener.Addr().String()

	_, err := customer.DialContext(&net.Dialer{Timeout: 2 * time.Second})(context.Background(), "tcp", addr)
	require.Error(t, err)
	assert.True(t, errors.Is(err, netguard.ErrBlocked), "customer dial: %v", err)

	conn, err := internal.DialContext(&net.Dialer{Timeout: 2 * time.Second})(context.Background(), "tcp", addr)
	require.NoError(t, err)
	_ = conn.Close()
}
