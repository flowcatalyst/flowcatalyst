//go:build integration

package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// The public check-domain, introspect and revoke routes take a per-IP limit
// (FC_RL_CHECK_DOMAIN_IP_PER_MIN, FC_RL_OAUTH_INTROSPECT_IP_PER_MIN,
// FC_RL_OAUTH_REVOKE_IP_PER_MIN). Wired through the real router.
func TestPublicRoutesAreRateLimitedPerIP(t *testing.T) {
	t.Setenv("FC_RL_CHECK_DOMAIN_IP_PER_MIN", "2")
	t.Setenv("FC_RL_OAUTH_INTROSPECT_IP_PER_MIN", "2")
	t.Setenv("FC_RL_OAUTH_REVOKE_IP_PER_MIN", "2")
	x := newXOHarness(t)
	salt := time.Now().UnixNano()

	type route struct {
		name, method, path, ctype, body string
	}
	routes := []route{
		{"check-domain POST", http.MethodPost, "/auth/check-domain", "application/json", `{"email":"a@limits.test"}`},
		{"check-domain GET", http.MethodGet, "/auth/check-domain?domain=limits.test", "", ""},
		{"introspect", http.MethodPost, "/oauth/introspect", "application/x-www-form-urlencoded", "token=nope"},
		{"revoke", http.MethodPost, "/oauth/revoke", "application/x-www-form-urlencoded", "token=nope"},
	}
	// check-domain's two methods share one budget; give each route family its
	// own address so the families do not draw on each other.
	family := map[string]uint16{
		"check-domain POST": 1, "check-domain GET": 1, "introspect": 2, "revoke": 3,
	}
	send := func(rt route, hop string) int {
		req := httptest.NewRequest(rt.method, rt.path, strings.NewReader(rt.body))
		if rt.ctype != "" {
			req.Header.Set("Content-Type", rt.ctype)
		}
		req.Header.Set("X-Forwarded-For", "9.9.9.9, "+hop) // rightmost hop is the key
		rec := httptest.NewRecorder()
		x.h.ServeHTTP(rec, req)
		return rec.Code
	}
	// The key must be a valid address (a non-IP entry falls back to the peer),
	// unique per run: documentation-range IPv6 with the salt in the low groups.
	ip := func(rt route, n int) string {
		return fmt.Sprintf("2001:db8:%x:%x:%x:%x::1", family[rt.name], n, uint16(salt>>16), uint16(salt))
	}

	for _, rt := range routes[:1] { // check-domain: POST then GET draw on one budget
		assert.NotEqual(t, http.StatusTooManyRequests, send(rt, ip(rt, 1)), "%s: first", rt.name)
	}
	assert.NotEqual(t, http.StatusTooManyRequests, send(routes[1], ip(routes[0], 1)), "check-domain GET: second shares the budget")
	assert.Equal(t, http.StatusTooManyRequests, send(routes[0], ip(routes[0], 1)), "check-domain: third from one IP")
	assert.Equal(t, http.StatusTooManyRequests, send(routes[1], ip(routes[0], 1)), "check-domain GET: also limited")
	assert.NotEqual(t, http.StatusTooManyRequests, send(routes[0], ip(routes[0], 2)), "check-domain: another IP unaffected")

	for _, rt := range routes[2:] {
		assert.NotEqual(t, http.StatusTooManyRequests, send(rt, ip(rt, 1)), "%s: first", rt.name)
		assert.NotEqual(t, http.StatusTooManyRequests, send(rt, ip(rt, 1)), "%s: second", rt.name)
		assert.Equal(t, http.StatusTooManyRequests, send(rt, ip(rt, 1)), "%s: third from one IP", rt.name)
		assert.NotEqual(t, http.StatusTooManyRequests, send(rt, ip(rt, 2)), "%s: another IP unaffected", rt.name)
	}
}

// Introspect and revoke are opt-in: with the default (0) a burst far past any
// small number is not limited.
func TestIntrospectAndRevokeUnlimitedByDefault(t *testing.T) {
	t.Setenv("FC_RL_CHECK_DOMAIN_IP_PER_MIN", "2") // proves the harness limiter is live
	x := newXOHarness(t)
	salt := time.Now().UnixNano()
	hop := fmt.Sprintf("2001:db8:77:0:%x:%x::1", uint16(salt>>16), uint16(salt))
	for _, path := range []string{"/oauth/introspect", "/oauth/revoke"} {
		for i := range 30 {
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("token=nope"))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("X-Forwarded-For", "9.9.9.9, "+hop)
			rec := httptest.NewRecorder()
			x.h.ServeHTTP(rec, req)
			if rec.Code == http.StatusTooManyRequests {
				t.Fatalf("%s request %d was limited with the default policy", path, i+1)
			}
		}
	}
}
