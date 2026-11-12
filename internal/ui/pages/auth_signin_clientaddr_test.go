package pages

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/clientaddr"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/ratelimit"
)

// callUpstream starts from a fresh context, so the address the edge resolved
// must be carried over: the upstream handler keys on the same caller as the
// browser's request, not on what the forwarded headers would give.
func TestCallUpstreamCarriesEdgeResolvedAddress(t *testing.T) {
	var seen string
	up := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		seen = ratelimit.ClientIP(req)
	})
	r := httptest.NewRequest(http.MethodPost, "/ui/auth/login", nil)
	r.RemoteAddr = "10.0.0.9:4444"
	r.Header.Set("X-Forwarded-For", "6.6.6.6, 1.1.1.1")
	r = r.WithContext(clientaddr.With(r.Context(), "203.0.113.77"))
	callUpstream(up, r, http.MethodPost, "/auth/login", map[string]string{"a": "b"})
	if seen != "203.0.113.77" {
		t.Errorf("upstream handler saw client address %q, want the edge-resolved 203.0.113.77", seen)
	}
}
