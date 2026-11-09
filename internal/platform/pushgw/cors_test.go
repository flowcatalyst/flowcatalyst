package pushgw

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

type fakeOrigins struct {
	list  []string
	err   error
	calls int
}

func (f *fakeOrigins) GetAllowedOrigins(context.Context) ([]string, error) {
	f.calls++
	return f.list, f.err
}

func corsServer(src OriginSource) (*CORS, http.Handler) {
	c := NewCORS(src)
	return c, c.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Reached", "yes")
		w.WriteHeader(http.StatusOK)
	}))
}

func do(h http.Handler, method, origin string, extra map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "/api/push/subscriptions", nil)
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

func TestCORSAllowedOrigin(t *testing.T) {
	_, h := corsServer(&fakeOrigins{list: []string{"https://app.example.com/"}})
	rec := do(h, http.MethodPost, "https://app.example.com", nil)
	assert.Equal(t, "https://app.example.com", rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Contains(t, rec.Header().Values("Vary"), "Origin")
	assert.Equal(t, "yes", rec.Header().Get("X-Reached"))
	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Credentials"), "never credentials: the session cookie is not usable cross-origin")
}

func TestCORSPreflight(t *testing.T) {
	_, h := corsServer(&fakeOrigins{list: []string{"https://app.example.com"}})
	rec := do(h, http.MethodOptions, "https://app.example.com", map[string]string{"Access-Control-Request-Method": "POST"})
	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, "https://app.example.com", rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Contains(t, rec.Header().Get("Access-Control-Allow-Headers"), "Authorization")
	assert.Contains(t, rec.Header().Get("Access-Control-Allow-Headers"), "DPoP")
	assert.Contains(t, rec.Header().Get("Access-Control-Allow-Methods"), "POST")
	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Credentials"))
	assert.Empty(t, rec.Header().Get("X-Reached"), "a preflight is answered here")
}

func TestCORSOriginNotInTheTable(t *testing.T) {
	_, h := corsServer(&fakeOrigins{list: []string{"https://app.example.com"}})
	rec := do(h, http.MethodPost, "https://evil.example", nil)
	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "yes", rec.Header().Get("X-Reached"), "the request itself is the handler's business")
	rec = do(h, http.MethodOptions, "https://evil.example", map[string]string{"Access-Control-Request-Method": "POST"})
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORSWithoutOriginHeaderAddsNothing(t *testing.T) {
	_, h := corsServer(&fakeOrigins{list: []string{"https://app.example.com"}})
	rec := do(h, http.MethodPost, "", nil)
	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Empty(t, rec.Header().Values("Vary"))
}

func TestCORSCachesTheTableAndSurvivesAFailedRead(t *testing.T) {
	src := &fakeOrigins{list: []string{"https://app.example.com"}}
	c, h := corsServer(src)
	now := time.Now()
	c.now = func() time.Time { return now }
	do(h, http.MethodPost, "https://app.example.com", nil)
	do(h, http.MethodPost, "https://app.example.com", nil)
	assert.Equal(t, 1, src.calls, "one read serves the TTL")

	now = now.Add(originCacheTTL + time.Second)
	src.err = errors.New("database down")
	rec := do(h, http.MethodPost, "https://app.example.com", nil)
	assert.Equal(t, "https://app.example.com", rec.Header().Get("Access-Control-Allow-Origin"), "the last known table stays")
	assert.Equal(t, 2, src.calls)
	do(h, http.MethodPost, "https://app.example.com", nil)
	assert.Equal(t, 2, src.calls, "a failed read is not retried for every request")
}

func TestCORSNeverAllowsAnOriginWhenTheFirstReadFails(t *testing.T) {
	_, h := corsServer(&fakeOrigins{err: errors.New("database down")})
	rec := do(h, http.MethodPost, "https://app.example.com", nil)
	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
}
