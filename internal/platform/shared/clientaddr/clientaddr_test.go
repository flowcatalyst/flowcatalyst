package clientaddr

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func req(remote string, xff ...string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = remote
	for _, v := range xff {
		r.Header.Add("X-Forwarded-For", v)
	}
	return r
}

func TestResolve(t *testing.T) {
	const peer = "10.0.0.9:4444"
	tests := []struct {
		name string
		r    *http.Request
		hops int
		want string
	}{
		{"hops 0 ignores XFF", req(peer, "1.1.1.1"), 0, "10.0.0.9"},
		{"hops 1 rightmost", req(peer, "9.9.9.9, 1.1.1.1"), 1, "1.1.1.1"},
		{"hops 1 forged leading entries", req(peer, "6.6.6.6, 7.7.7.7, 1.1.1.1"), 1, "1.1.1.1"},
		{"hops 2", req(peer, "6.6.6.6, 2.2.2.2, 1.1.1.1"), 2, "2.2.2.2"},
		{"hops 3", req(peer, "6.6.6.6, 2.2.2.2, 1.1.1.1"), 3, "6.6.6.6"},
		{"fewer entries than hops takes leftmost", req(peer, "2.2.2.2, 1.1.1.1"), 3, "2.2.2.2"},
		{"header absent", req(peer), 1, "10.0.0.9"},
		{"header absent hops 2", req(peer), 2, "10.0.0.9"},
		{"two header lines", req(peer, "6.6.6.6, 2.2.2.2", "1.1.1.1"), 2, "2.2.2.2"},
		{"two header lines hops 1", req(peer, "6.6.6.6, 2.2.2.2", "1.1.1.1"), 1, "1.1.1.1"},
		{"empty entry keeps positions", req(peer, "2.2.2.2,,"), 2, "10.0.0.9"},
		{"entry with port", req(peer, "1.2.3.4:5678"), 1, "1.2.3.4"},
		{"bare IPv6", req(peer, "2001:db8::1"), 1, "2001:db8::1"},
		{"bracketed IPv6", req(peer, "[2001:db8::1]"), 1, "2001:db8::1"},
		{"bracketed IPv6 with port", req(peer, "[2001:db8::1]:443"), 1, "2001:db8::1"},
		{"IPv4-mapped IPv6", req(peer, "::ffff:1.2.3.4"), 1, "1.2.3.4"},
		{"zone dropped", req(peer, "fe80::1%eth0"), 1, "fe80::1"},
		{"non-IP chosen entry falls back to peer", req(peer, "1.1.1.1, not-an-ip"), 1, "10.0.0.9"},
		{"IPv6 peer normalised", req("[::1]:5555"), 1, "::1"},
		{"IPv6 peer hops 0", req("[2001:db8::7]:5555", "1.1.1.1"), 0, "2001:db8::7"},
		{"mapped peer", req("[::ffff:10.0.0.9]:1"), 0, "10.0.0.9"},
		{"unparseable RemoteAddr kept", req(" bogus "), 0, "bogus"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Resolve(tc.r, tc.hops); got != tc.want {
				t.Errorf("Resolve = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveIgnoresRealIPHeaders(t *testing.T) {
	for _, hops := range []int{0, 1} {
		r := req("10.0.0.9:1")
		r.Header.Set("X-Real-IP", "6.6.6.6")
		r.Header.Set("True-Client-IP", "7.7.7.7")
		if got := Resolve(r, hops); got != "10.0.0.9" {
			t.Errorf("hops %d: Resolve = %q, want the peer", hops, got)
		}
	}
}

func TestMiddlewareAndOf(t *testing.T) {
	var got, remote string
	h := Middleware(1)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Headers that would now resolve differently: the context value wins.
		r.Header.Set("X-Forwarded-For", "8.8.8.8")
		got = Of(r)
		remote = r.RemoteAddr
	}))
	h.ServeHTTP(httptest.NewRecorder(), req("10.0.0.9:1", "1.1.1.1"))
	if got != "1.1.1.1" {
		t.Errorf("Of = %q, want the edge-resolved 1.1.1.1", got)
	}
	if remote != "10.0.0.9:1" {
		t.Errorf("RemoteAddr rewritten to %q", remote)
	}
}

func TestOfWithoutContextUsesDefault(t *testing.T) {
	if got := Of(req("10.0.0.9:1", "6.6.6.6, 1.1.1.1")); got != "1.1.1.1" {
		t.Errorf("Of = %q, want 1.1.1.1", got)
	}
	if _, ok := FromContext(req("10.0.0.9:1").Context()); ok {
		t.Error("FromContext reported a value on a bare request")
	}
}
