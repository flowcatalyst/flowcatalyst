package cors

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The push gateway's own tests (internal/platform/pushgw/cors_test.go) pin its rule; these pin the
// middleware's behaviour for any rule, with the field protocol's shape as the example.

type listSource struct {
	list  []string
	err   error
	calls int
}

func (s *listSource) GetAllowedOrigins(context.Context) ([]string, error) {
	s.calls++
	return s.list, s.err
}

var testRule = Rule{Methods: "POST", Headers: "Authorization, Content-Type, DPoP", Expose: "DPoP-Nonce, Retry-After"}

// reached is the handler behind the middleware: it marks the answer and sets the headers a field
// route sets.
var reached = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("X-Reached", "yes")
	w.Header().Set("DPoP-Nonce", "n-1")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"invalid_dpop_proof"}`))
})

func serve(h http.Handler, method, origin string, extra map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "/field/auth/login", nil)
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	for k, v := range extra {
		r.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func TestMiddlewarePreflightFromAnAllowedOrigin(t *testing.T) {
	h := Middleware(NewOrigins(&listSource{list: []string{"https://app.example.com"}}, nil), testRule)(reached)
	rec := serve(h, http.MethodOptions, "https://app.example.com", map[string]string{
		"Access-Control-Request-Method":  "POST",
		"Access-Control-Request-Headers": "content-type, dpop",
	})
	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, "https://app.example.com", rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "POST", rec.Header().Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "Authorization, Content-Type, DPoP", rec.Header().Get("Access-Control-Allow-Headers"))
	assert.Equal(t, "DPoP-Nonce, Retry-After", rec.Header().Get("Access-Control-Expose-Headers"))
	assert.Equal(t, "600", rec.Header().Get("Access-Control-Max-Age"))
	assert.Equal(t, []string{"Origin", "Access-Control-Request-Method", "Access-Control-Request-Headers"}, rec.Header().Values("Vary"))
	assert.Empty(t, rec.Header().Values("Access-Control-Allow-Credentials"))
	assert.Empty(t, rec.Header().Get("X-Reached"), "a preflight is answered by the middleware")
}

func TestMiddlewareActualRequestFromAnAllowedOrigin(t *testing.T) {
	h := Middleware(NewOrigins(&listSource{list: []string{"https://app.example.com/"}}, nil), testRule)(reached)
	rec := serve(h, http.MethodPost, "https://app.example.com", nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code, "the handler's own answer")
	assert.Equal(t, "yes", rec.Header().Get("X-Reached"))
	assert.Equal(t, "https://app.example.com", rec.Header().Get("Access-Control-Allow-Origin"), "a trailing slash in the table is ignored")
	assert.Equal(t, "DPoP-Nonce, Retry-After", rec.Header().Get("Access-Control-Expose-Headers"))
	assert.Equal(t, []string{"Origin"}, rec.Header().Values("Vary"))
	assert.Empty(t, rec.Header().Values("Access-Control-Allow-Credentials"))
	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Methods"), "methods and headers are for the preflight only")
}

func TestMiddlewareOriginNotInTheTable(t *testing.T) {
	h := Middleware(NewOrigins(&listSource{list: []string{"https://app.example.com"}}, nil), testRule)(reached)

	rec := serve(h, http.MethodOptions, "https://evil.example", map[string]string{"Access-Control-Request-Method": "POST"})
	assert.Equal(t, http.StatusForbidden, rec.Code, "the preflight is refused")
	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Empty(t, rec.Header().Get("X-Reached"))

	rec = serve(h, http.MethodPost, "https://evil.example", nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code, "an actual request is the handler's business")
	assert.Equal(t, "yes", rec.Header().Get("X-Reached"))
	for k := range rec.Header() {
		assert.NotContains(t, k, "Access-Control-", "no CORS header at all")
	}
	assert.Equal(t, []string{"Origin"}, rec.Header().Values("Vary"), "the answer depends on the origin")

	// Origins compare exactly: "*" does not allow everyone, and a host pattern the console accepts
	// (https://*.example.com) matches no page.
	h = Middleware(NewOrigins(&listSource{list: []string{"*", "https://*.example.com"}}, nil), testRule)(reached)
	for _, o := range []string{"https://evil.example", "https://app.example.com"} {
		rec = serve(h, http.MethodOptions, o, map[string]string{"Access-Control-Request-Method": "POST"})
		assert.Equal(t, http.StatusForbidden, rec.Code, o)
	}
}

func TestMiddlewareWithoutAnOriginHeaderChangesNothing(t *testing.T) {
	src := &listSource{list: []string{"https://app.example.com"}}
	h := Middleware(NewOrigins(src, nil), testRule)(reached)
	for _, method := range []string{http.MethodPost, http.MethodOptions} {
		want := serve(reached, method, "", nil)
		got := serve(h, method, "", nil)
		assert.Equal(t, want.Code, got.Code, method)
		assert.Equal(t, want.Header(), got.Header(), method)
		assert.Equal(t, want.Body.Bytes(), got.Body.Bytes(), method)
	}
	assert.Zero(t, src.calls, "the table is not even read")
}

func TestOriginsAddedAndRemovedApplyAfterTheCacheTime(t *testing.T) {
	src := &listSource{list: []string{"https://old.example.com"}}
	now := time.Now()
	h := Middleware(NewOrigins(src, func() time.Time { return now }), testRule)(reached)
	preflight := func(origin string) int {
		return serve(h, http.MethodOptions, origin, map[string]string{"Access-Control-Request-Method": "POST"}).Code
	}
	require.Equal(t, http.StatusNoContent, preflight("https://old.example.com"))
	require.Equal(t, http.StatusForbidden, preflight("https://new.example.com"))

	src.list = []string{"https://new.example.com"} // one added, one removed in the console
	now = now.Add(OriginCacheTTL)
	assert.Equal(t, http.StatusNoContent, preflight("https://old.example.com"), "cached until the TTL has passed")
	assert.Equal(t, http.StatusForbidden, preflight("https://new.example.com"))
	assert.Equal(t, 1, src.calls)

	now = now.Add(time.Second)
	assert.Equal(t, http.StatusForbidden, preflight("https://old.example.com"), "a removed origin stops")
	assert.Equal(t, http.StatusNoContent, preflight("https://new.example.com"), "an added origin applies")
	assert.Equal(t, 2, src.calls)
}

func TestOriginsKeepTheLastTableWhenAReadFails(t *testing.T) {
	src := &listSource{list: []string{"https://app.example.com"}}
	now := time.Now()
	o := NewOrigins(src, func() time.Time { return now })
	require.True(t, o.Allows(context.Background(), "https://app.example.com"))

	now = now.Add(OriginCacheTTL + time.Second)
	src.err = errors.New("database down")
	assert.True(t, o.Allows(context.Background(), "https://app.example.com"), "the last known table stays")
	assert.True(t, o.Allows(context.Background(), "https://app.example.com"))
	assert.Equal(t, 2, src.calls, "a failed read is not retried for every request")

	assert.False(t, NewOrigins(&listSource{err: errors.New("down")}, nil).Allows(context.Background(), "https://app.example.com"),
		"no table read yet: no origin is allowed")
}
