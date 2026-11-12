package middleware

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/provider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

func testHeaderProvider(t *testing.T) *provider.Provider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	p, err := provider.NewProvider(provider.Config{Issuer: "flowcatalyst", Audience: "flowcatalyst", SigningKey: privPEM}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// T4: X-FC-Test-* headers are honoured only for a loopback (or unrecorded) peer.
func TestAuthenticatorTestHeadersOnlyFromLoopbackPeer(t *testing.T) {
	prov := testHeaderProvider(t)
	cases := []struct {
		name    string
		peer    string // "" = no recorded peer
		xff     string
		wantCtx bool
	}{
		{"ipv4 loopback", "127.0.0.1:5555", "", true},
		{"ipv6 loopback", "[::1]:5555", "", true},
		{"lan peer", "192.168.1.20:5555", "", false},
		{"v4-mapped loopback", "[::ffff:127.0.0.1]:5555", "", true},
		{"no recorded peer", "", "", true},
		{"spoofed xff from lan peer", "192.168.1.20:5555", "127.0.0.1", false},
		{"unparseable peer", "not-an-address", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got *auth.AuthContext
			h := Authenticator(AuthConfig{Provider: prov, AllowTestHeaders: true})(http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) { got = auth.FromContext(r.Context()) }))
			req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
			req.Header.Set("X-FC-Test-Principal", "prn_admin")
			req.Header.Set("X-FC-Test-Scope", "ANCHOR")
			if tc.xff != "" {
				req.Header.Set("X-Forwarded-For", tc.xff)
			}
			chain := h
			if tc.peer != "" {
				req.RemoteAddr = tc.peer
				chain = RecordPeerAddr(h)
			}
			chain.ServeHTTP(httptest.NewRecorder(), req)
			if tc.wantCtx && (got == nil || got.PrincipalID != "prn_admin") {
				t.Errorf("peer %q: AuthContext = %+v, want the header identity", tc.peer, got)
			}
			if !tc.wantCtx && got != nil {
				t.Errorf("peer %q: got AuthContext %+v, want none (test headers must be ignored)", tc.peer, got)
			}
		})
	}
}

// The recorder, mounted ahead of RealIP, keeps the connection's address even
// when the request carries X-Forwarded-For / X-Real-IP. (The server's real
// ordering is covered by internal/server's TestEdgeMiddlewareRecordsTCPPeer.)
func TestRecordPeerAddrBeforeRealIPKeepsTCPPeer(t *testing.T) {
	var peer, remote string
	r := chi.NewRouter()
	r.Use(RecordPeerAddr)
	r.Use(chimw.RealIP)
	r.Get("/", func(w http.ResponseWriter, req *http.Request) {
		peer, _ = PeerAddrFromContext(req.Context())
		remote = req.RemoteAddr
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.20:5555"
	req.Header.Set("X-Forwarded-For", "127.0.0.1")
	r.ServeHTTP(httptest.NewRecorder(), req)
	if peer != "192.168.1.20:5555" {
		t.Errorf("recorded peer = %q, want the TCP peer", peer)
	}
	if remote == "192.168.1.20:5555" {
		t.Errorf("RealIP did not rewrite RemoteAddr; the test proves nothing")
	}
}
