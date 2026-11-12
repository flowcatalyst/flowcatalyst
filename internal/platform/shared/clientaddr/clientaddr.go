// Package clientaddr works out the address of the caller of an HTTP request.
// Every per-address decision (rate limits, login backoff, the address recorded
// on a login attempt) uses this one rule.
//
// The rule is set by a number of trusted proxy hops (FC_HTTP_TRUSTED_PROXY_HOPS):
//
//   - 0: no proxy sits in front. The address is the connection's peer and
//     X-Forwarded-For is ignored.
//   - N >= 1: N proxies we run sit in front, and each appends the address it saw
//     to X-Forwarded-For. The address is the Nth entry from the right; with
//     fewer than N entries it is the leftmost; with no header it is the peer.
//
// True-Client-IP and X-Real-IP are never read.
//
// The one known weakness of a hop count: with N >= 2, a caller who can reach an
// inner proxy directly can supply the outer entries itself. Inner proxies must
// only accept traffic from the outer one.
package clientaddr

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// DefaultTrustedProxyHops is the hop count used when none is configured: one
// load balancer in front.
const DefaultTrustedProxyHops = 1

type ctxKey struct{}

// With returns ctx carrying addr as the resolved client address.
func With(ctx context.Context, addr string) context.Context {
	return context.WithValue(ctx, ctxKey{}, addr)
}

// FromContext returns the address stored by [With] or [Middleware].
func FromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(ctxKey{}).(string)
	return v, ok
}

// Of returns the client address of r: the value resolved at the edge when the
// request carries one, else [Resolve] with [DefaultTrustedProxyHops] (a handler
// driven directly, as tests do).
func Of(r *http.Request) string {
	if v, ok := FromContext(r.Context()); ok {
		return v
	}
	return Resolve(r, DefaultTrustedProxyHops)
}

// Middleware resolves the client address once, at the edge, and stores it in
// the request context. It does not rewrite r.RemoteAddr.
func Middleware(hops int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(With(r.Context(), Resolve(r, hops))))
		})
	}
}

// Resolve applies the rule described in the package comment.
func Resolve(r *http.Request, hops int) string {
	peer := peerOf(r.RemoteAddr)
	if hops <= 0 {
		return peer
	}
	lines := r.Header.Values("X-Forwarded-For")
	if len(lines) == 0 {
		return peer
	}
	var entries []string
	for _, line := range lines {
		for p := range strings.SplitSeq(line, ",") {
			entries = append(entries, strings.TrimSpace(p))
		}
	}
	i := max(len(entries)-hops, 0)
	if a, ok := parseAddr(entries[i]); ok {
		return a
	}
	return peer
}

// peerOf is the host part of a RemoteAddr, normalised. An unparseable value is
// returned trimmed.
func peerOf(remote string) string {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	if a, ok := parseAddr(host); ok {
		return a
	}
	if i := strings.LastIndexByte(remote, ':'); i >= 0 {
		remote = remote[:i]
	}
	return strings.TrimSpace(remote)
}

// parseAddr accepts an IP address with an optional port, in brackets or not,
// and returns its normalised form.
func parseAddr(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	if ap, err := netip.ParseAddrPort(s); err == nil {
		return norm(ap.Addr()), true
	}
	if a, err := netip.ParseAddr(s); err == nil {
		return norm(a), true
	}
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		if a, err := netip.ParseAddr(s[1 : len(s)-1]); err == nil {
			return norm(a), true
		}
	}
	return "", false
}

func norm(a netip.Addr) string {
	return a.Unmap().WithZone("").String()
}
