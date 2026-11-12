package middleware

import (
	"context"
	"net/http"
	"net/netip"
)

type peerAddrKey struct{}

// RecordPeerAddr stores the connection's original r.RemoteAddr in the request
// context. It must be mounted BEFORE any middleware that rewrites RemoteAddr
// from client-supplied headers, so that the recorded value is the TCP peer and
// cannot be spoofed. (The server's edge no longer rewrites RemoteAddr at all;
// the caller's address is resolved separately, see package clientaddr.)
func RecordPeerAddr(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), peerAddrKey{}, r.RemoteAddr)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// PeerAddrFromContext returns the address recorded by [RecordPeerAddr].
func PeerAddrFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(peerAddrKey{}).(string)
	return v, ok
}

// testHeadersAllowedFrom reports whether X-FC-Test-* headers may be honoured
// for this request: true when no peer was recorded (a handler driven directly,
// as tests do) or the recorded peer is a loopback address. An unparseable
// recorded value counts as not loopback.
func testHeadersAllowedFrom(r *http.Request) bool {
	peer, ok := PeerAddrFromContext(r.Context())
	if !ok {
		return true
	}
	ap, err := netip.ParseAddrPort(peer)
	if err != nil {
		return false
	}
	return ap.Addr().Unmap().IsLoopback()
}
