package client_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/go-sdk/client"
)

// step is one scripted answer: a status and body, or an action that does
// something other than answer (drop the connection, stall).
type step struct {
	status int
	body   string
	action func(w http.ResponseWriter, r *http.Request)
}

func answer(status int, body string) step { return step{status: status, body: body} }

// hangUp reads the request, then closes the connection without answering:
// the request was sent and the answer is lost.
var hangUp = step{action: func(w http.ResponseWriter, r *http.Request) {
	_, _ = io.ReadAll(r.Body)
	conn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		return
	}
	_ = conn.Close()
}}

// stall answers only after the client has given up (its timeout is shorter).
var stall = step{action: func(_ http.ResponseWriter, r *http.Request) {
	_, _ = io.ReadAll(r.Body)
	select {
	case <-r.Context().Done():
	case <-time.After(5 * time.Second):
	}
}}

// scripted serves the steps in order, one per request, and counts the
// requests. A request past the script fails the test.
type scripted struct {
	*httptest.Server
	mu       sync.Mutex
	steps    []step
	requests []string // "METHOD /path"
}

func newScripted(t *testing.T, steps ...step) *scripted {
	t.Helper()
	s := &scripted{steps: steps}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		n := len(s.requests)
		s.requests = append(s.requests, r.Method+" "+r.URL.Path)
		s.mu.Unlock()
		if n >= len(s.steps) {
			t.Errorf("unscripted request %d: %s %s", n+1, r.Method, r.URL.Path)
			w.WriteHeader(http.StatusTeapot)
			return
		}
		st := s.steps[n]
		if st.action != nil {
			st.action(w, r)
			return
		}
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(st.status)
		_, _ = w.Write([]byte(st.body))
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *scripted) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.requests)
}

func retrying(url string, opts ...client.Option) *client.FlowCatalystClient {
	return client.New(url, append([]client.Option{client.WithRetry(3, time.Millisecond)}, opts...)...)
}

// ─── Repeatable methods ──────────────────────────────────────────────

func TestRetryGET502Then200(t *testing.T) {
	srv := newScripted(t, answer(502, `bad gateway`), answer(200, `{"ok":"yes"}`))
	var out map[string]string
	require.NoError(t, retrying(srv.URL).Get(context.Background(), "/api/x", &out))
	assert.Equal(t, "yes", out["ok"])
	assert.Equal(t, 2, srv.count())
}

func TestRetryGET500Then200(t *testing.T) {
	srv := newScripted(t, answer(500, `{"error":"INTERNAL"}`), answer(200, `{}`))
	require.NoError(t, retrying(srv.URL).Get(context.Background(), "/api/x", nil))
	assert.Equal(t, 2, srv.count())
}

func TestRetryPUT503Then200(t *testing.T) {
	srv := newScripted(t, answer(503, ``), answer(204, ``))
	require.NoError(t, retrying(srv.URL).Put(context.Background(), "/api/x/1", map[string]string{"a": "b"}, nil))
	assert.Equal(t, 2, srv.count())
}

func TestRetryDELETE502Then404Succeeds(t *testing.T) {
	srv := newScripted(t, answer(502, ``), answer(404, `{"error":"NOT_FOUND"}`))
	out := map[string]string{"kept": "as is"}
	require.NoError(t, retrying(srv.URL).Delete(context.Background(), "/api/x/1", &out))
	assert.Equal(t, 2, srv.count())
	assert.Equal(t, map[string]string{"kept": "as is"}, out, "a 404 after an unknown outcome leaves out untouched")
}

func TestRetryDELETEHangUpThen404Succeeds(t *testing.T) {
	srv := newScripted(t, hangUp, answer(404, ``))
	require.NoError(t, retrying(srv.URL).Delete(context.Background(), "/api/x/1", nil))
	assert.Equal(t, 2, srv.count())
}

func TestRetryDELETE404FirstIsNotFound(t *testing.T) {
	srv := newScripted(t, answer(404, `{"error":"NOT_FOUND"}`))
	err := retrying(srv.URL).Delete(context.Background(), "/api/x/1", nil)
	var apiErr *client.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.True(t, apiErr.IsNotFound())
	assert.False(t, client.IsOutcomeUnknown(err))
	assert.Equal(t, 1, srv.count())
}

func TestRetryDELETE429Then404IsNotFound(t *testing.T) {
	// 429 says the first attempt was not processed: the 404 is a real one.
	srv := newScripted(t, answer(429, ``), answer(404, ``))
	err := retrying(srv.URL).Delete(context.Background(), "/api/x/1", nil)
	var apiErr *client.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.True(t, apiErr.IsNotFound())
	assert.Equal(t, 2, srv.count())
}

func TestRetryGETExhaustedReturnsAPIError(t *testing.T) {
	srv := newScripted(t, answer(502, ``), answer(503, ``), answer(504, `last`))
	err := retrying(srv.URL).Get(context.Background(), "/api/x", nil)
	var apiErr *client.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 504, apiErr.StatusCode)
	assert.False(t, client.IsOutcomeUnknown(err), "a repeatable request reports its last answer")
	assert.Equal(t, 3, srv.count())
}

// ─── POST: not repeatable ────────────────────────────────────────────

func TestRetryPOST502IsOutcomeUnknown(t *testing.T) {
	for _, status := range []int{502, 503, 504} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := newScripted(t, answer(status, `gateway says no`), answer(201, `{"id":"x"}`))
			err := retrying(srv.URL).Post(context.Background(), "/api/things", map[string]string{"a": "b"}, nil)
			require.Error(t, err)
			assert.Equal(t, 1, srv.count(), "the POST is not sent again")

			var unknown *client.OutcomeUnknownError
			require.ErrorAs(t, err, &unknown)
			assert.True(t, client.IsOutcomeUnknown(err))
			assert.Equal(t, http.MethodPost, unknown.Method)
			assert.Equal(t, srv.URL+"/api/things", unknown.URL)
			assert.Equal(t, status, unknown.StatusCode)
			assert.Equal(t, "gateway says no", unknown.Body)
			assert.Contains(t, err.Error(), "may have been applied and was not repeated")

			var apiErr *client.APIError
			require.ErrorAs(t, err, &apiErr, "old errors.As code still sees the status")
			assert.Equal(t, status, apiErr.StatusCode)
		})
	}
}

func TestRetryPOST500IsReturnedNotRetried(t *testing.T) {
	srv := newScripted(t, answer(500, `{"error":"INTERNAL","message":"boom"}`), answer(201, `{}`))
	err := retrying(srv.URL).Post(context.Background(), "/api/things", nil, nil)
	var apiErr *client.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 500, apiErr.StatusCode)
	assert.False(t, client.IsOutcomeUnknown(err))
	assert.Equal(t, 1, srv.count())
}

func TestRetryPATCHIsNotRepeatable(t *testing.T) {
	srv := newScripted(t, answer(502, ``), answer(200, `{}`))
	err := retrying(srv.URL).Patch(context.Background(), "/api/things/1", map[string]string{"a": "b"}, nil)
	assert.True(t, client.IsOutcomeUnknown(err))
	assert.Equal(t, 1, srv.count())
}

func TestRetryPOST429And408Then200(t *testing.T) {
	for _, status := range []int{429, 408, 425} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := newScripted(t, answer(status, ``), answer(201, `{"id":"x"}`))
			var out client.CreatedResponse
			require.NoError(t, retrying(srv.URL).Post(context.Background(), "/api/things", nil, &out))
			assert.Equal(t, "x", out.ID)
			assert.Equal(t, 2, srv.count())
		})
	}
}

func TestRetryPOSTSyncEndpoint502Then200(t *testing.T) {
	srv := newScripted(t,
		answer(502, ``),
		answer(200, `{"applicationCode":"orders","created":0,"updated":0,"deleted":0,"syncedCodes":["orders:admin"]}`))
	res, err := retrying(srv.URL).Roles().Sync(context.Background(), "orders",
		&client.SyncRolesRequest{Roles: []client.SyncRoleItem{{Name: "admin"}}}, false)
	require.NoError(t, err)
	assert.Equal(t, []string{"orders:admin"}, res.SyncedCodes)
	assert.Equal(t, []string{"POST /api/applications/orders/roles/sync", "POST /api/applications/orders/roles/sync"}, srv.requests)
}

func TestRetryPOSTSyncEndpointHangUpThen200(t *testing.T) {
	srv := newScripted(t, hangUp, answer(200, `{"applicationCode":"orders","syncedCodes":[]}`))
	_, err := retrying(srv.URL).EventTypes().Sync(context.Background(), "orders", &client.SyncEventTypesRequest{}, false)
	require.NoError(t, err)
	assert.Equal(t, 2, srv.count())
}

func TestRetryReadOnlyPOSTs502Then200(t *testing.T) {
	t.Run("field token introspect", func(t *testing.T) {
		srv := newScripted(t, answer(502, ``), answer(200, `{"active":false}`))
		_, err := retrying(srv.URL).FieldTokens().Introspect(context.Background(), "tok")
		require.NoError(t, err)
		assert.Equal(t, 2, srv.count())
	})
	t.Run("router check-batch", func(t *testing.T) {
		srv := newScripted(t, answer(503, ``), answer(200, `{"m1":true}`))
		got, err := retrying(srv.URL).Router().InPipelineBatch(context.Background(), []string{"m1"})
		require.NoError(t, err)
		assert.True(t, got["m1"])
		assert.Equal(t, 2, srv.count())
	})
	t.Run("PostRepeatable", func(t *testing.T) {
		srv := newScripted(t, answer(500, ``), answer(200, `{}`))
		require.NoError(t, retrying(srv.URL).PostRepeatable(context.Background(), "/api/custom/query", nil, nil))
		assert.Equal(t, 2, srv.count())
	})
}

// The defect as reported: the platform committed the provisioning, the
// gateway answered 502, and the retry was refused 409 ALREADY_PROVISIONED,
// losing the one-time secrets. Now exactly one request is made.
func TestRetryProvisionServiceAccount502IsOneRequestOutcomeUnknown(t *testing.T) {
	srv := newScripted(t,
		answer(502, `<html>502 Bad Gateway</html>`),
		answer(409, `{"error":"ALREADY_PROVISIONED","message":"Application already has a service account"}`))
	c := client.New(srv.URL) // the default retry configuration

	res, err := c.Applications().ProvisionServiceAccount(context.Background(), "app_1")
	assert.Nil(t, res)
	require.Error(t, err)
	assert.Equal(t, []string{"POST /api/applications/app_1/provision-service-account"}, srv.requests,
		"exactly one request; the scripted 409 is never consumed")

	var unknown *client.OutcomeUnknownError
	require.ErrorAs(t, err, &unknown)
	assert.Equal(t, 502, unknown.StatusCode)
	var apiErr *client.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.False(t, apiErr.IsConflict())
}

// ─── Transport failures ──────────────────────────────────────────────

// closedAddr is an address nothing listens on (connection refused).
func closedAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	return addr
}

// firstDialTo is an HTTP client whose first dial goes through first, and
// later dials reach the server.
func firstDialTo(t *testing.T, first func(ctx context.Context, network, addr string) (net.Conn, error)) *http.Client {
	t.Helper()
	var dials atomic.Int32
	tr := &http.Transport{DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		if dials.Add(1) == 1 {
			return first(ctx, network, addr)
		}
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}}
	t.Cleanup(tr.CloseIdleConnections)
	return &http.Client{Transport: tr}
}

func TestRetryPOSTConnectionRefusedThen200(t *testing.T) {
	srv := newScripted(t, answer(201, `{"id":"x"}`))
	refused := closedAddr(t)
	hc := firstDialTo(t, func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, refused)
	})
	var out client.CreatedResponse
	require.NoError(t, retrying(srv.URL, client.WithHTTPClient(hc)).Post(context.Background(), "/api/things", nil, &out))
	assert.Equal(t, "x", out.ID)
	assert.Equal(t, 1, srv.count(), "the refused attempt never reached the server")
}

func TestRetryPOSTConnectionRefusedEveryTimeIsNotOutcomeUnknown(t *testing.T) {
	c := retrying("http://"+closedAddr(t), client.WithHTTPClient(&http.Client{Transport: &http.Transport{}}))
	err := c.Post(context.Background(), "/api/things", nil, nil)
	require.Error(t, err)
	assert.False(t, client.IsOutcomeUnknown(err), "nothing was sent")
}

func TestRetryPOSTDNSFailureIsNotOutcomeUnknown(t *testing.T) {
	c := retrying("http://flowcatalyst-sdk-test.invalid", client.WithRetry(2, time.Millisecond))
	err := c.Post(context.Background(), "/api/things", nil, nil)
	require.Error(t, err)
	var dnsErr *net.DNSError
	assert.ErrorAs(t, err, &dnsErr)
	assert.False(t, client.IsOutcomeUnknown(err))
}

func TestRetryPOSTConnectTimeoutThen200(t *testing.T) {
	srv := newScripted(t, answer(201, `{"id":"x"}`))
	// The first dial hangs until the attempt's timeout: no connection, so
	// nothing was sent.
	hc := firstDialTo(t, func(ctx context.Context, _, _ string) (net.Conn, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	c := retrying(srv.URL, client.WithHTTPClient(hc), client.WithTimeout(100*time.Millisecond))
	require.NoError(t, c.Post(context.Background(), "/api/things", nil, nil))
	assert.Equal(t, 1, srv.count())
}

func TestRetryPOSTHangUpAfterSendIsOutcomeUnknown(t *testing.T) {
	srv := newScripted(t, hangUp, answer(201, `{"id":"x"}`))
	err := retrying(srv.URL).Post(context.Background(), "/api/things", map[string]string{"a": "b"}, nil)
	require.Error(t, err)
	assert.Equal(t, 1, srv.count())

	var unknown *client.OutcomeUnknownError
	require.ErrorAs(t, err, &unknown)
	assert.Equal(t, 0, unknown.StatusCode)
	assert.Error(t, unknown.Err, "the transport error is kept")
	var apiErr *client.APIError
	assert.False(t, errors.As(err, &apiErr))
}

func TestRetryPOSTReadTimeoutIsOutcomeUnknown(t *testing.T) {
	srv := newScripted(t, stall, answer(201, `{"id":"x"}`))
	c := retrying(srv.URL, client.WithTimeout(100*time.Millisecond))
	err := c.Post(context.Background(), "/api/things", nil, nil)
	require.Error(t, err)
	assert.True(t, client.IsOutcomeUnknown(err))
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, 1, srv.count())
}

func TestRetryGETReadTimeoutThen200(t *testing.T) {
	srv := newScripted(t, stall, answer(200, `{"ok":"yes"}`))
	c := retrying(srv.URL, client.WithTimeout(100*time.Millisecond))
	var out map[string]string
	require.NoError(t, c.Get(context.Background(), "/api/x", &out))
	assert.Equal(t, "yes", out["ok"])
	assert.Equal(t, 2, srv.count())
}

func TestRetryCallerCancellationIsNotRetried(t *testing.T) {
	srv := newScripted(t, stall, answer(200, `{}`))
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := retrying(srv.URL).Get(ctx, "/api/x", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.False(t, client.IsOutcomeUnknown(err))
	assert.Equal(t, 1, srv.count())
}

// ─── Configuration ───────────────────────────────────────────────────

func TestRetryOffWithZeroOrOneAttempt(t *testing.T) {
	for _, attempts := range []int{0, 1} {
		srv := newScripted(t, answer(503, ``), answer(200, `{}`))
		err := client.New(srv.URL, client.WithRetry(attempts, time.Millisecond)).Get(context.Background(), "/api/x", nil)
		var apiErr *client.APIError
		require.ErrorAs(t, err, &apiErr, "attempts=%d", attempts)
		assert.Equal(t, 503, apiErr.StatusCode)
		assert.Equal(t, 1, srv.count(), "attempts=%d makes exactly one request", attempts)
	}
}

func TestRetryOutcomeUnknownWithoutRetries(t *testing.T) {
	// With retries off a POST's 502 is still reported as outcome unknown.
	srv := newScripted(t, answer(502, ``))
	err := client.New(srv.URL, client.WithRetry(1, 0)).Post(context.Background(), "/api/things", nil, nil)
	assert.True(t, client.IsOutcomeUnknown(err))
}
