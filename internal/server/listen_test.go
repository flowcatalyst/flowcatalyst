package server

import (
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/clientaddr"
	platformmw "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/middleware"
)

// T1
func TestListenAddrs(t *testing.T) {
	cases := []struct {
		bind string
		want []string
	}{
		{"", []string{":8080"}},
		{"loopback", []string{"127.0.0.1:8080", "[::1]:8080"}},
		{"0.0.0.0", []string{"0.0.0.0:8080"}},
		{"::", []string{"[::]:8080"}},
		{"fe80::1", []string{"[fe80::1]:8080"}},
		{"dev.local", []string{"dev.local:8080"}},
	}
	for _, tc := range cases {
		if got := listenAddrs(tc.bind, 8080); !slices.Equal(got, tc.want) {
			t.Errorf("listenAddrs(%q) = %v, want %v", tc.bind, got, tc.want)
		}
	}
}

// T2
func TestLoadEnv_APIBind(t *testing.T) {
	t.Setenv("FC_API_BIND", "")
	if got := LoadEnv().APIBind; got != "" {
		t.Errorf("APIBind unset = %q, want empty", got)
	}
	t.Setenv("FC_API_BIND", "0.0.0.0")
	if got := LoadEnv().APIBind; got != "0.0.0.0" {
		t.Errorf("APIBind = %q, want 0.0.0.0", got)
	}
}

// T6 (listener-opening helper; Run itself needs a database)
func TestOpenListenersLoopbackHasNoWildcard(t *testing.T) {
	lns, err := openListeners(BindLoopback, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer closeAll(lns)
	if len(lns) == 0 {
		t.Fatal("no listeners")
	}
	for _, ln := range lns {
		ta := ln.Addr().(*net.TCPAddr)
		if !ta.IP.IsLoopback() {
			t.Errorf("listener %s is not loopback", ln.Addr())
		}
		if ta.IP.IsUnspecified() {
			t.Errorf("listener %s is a wildcard", ln.Addr())
		}
	}
	if !lns[0].Addr().(*net.TCPAddr).IP.Equal(net.IPv4(127, 0, 0, 1)) {
		t.Errorf("first listener = %s, want 127.0.0.1", lns[0].Addr())
	}
	if len(lns) == 2 && lns[0].Addr().(*net.TCPAddr).Port != lns[1].Addr().(*net.TCPAddr).Port {
		t.Errorf("v4/v6 ports differ: %s vs %s", lns[0].Addr(), lns[1].Addr())
	}
}

func TestOpenListenersEmptyBindIsWildcard(t *testing.T) {
	lns, err := openListeners("", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer closeAll(lns)
	if len(lns) != 1 || !lns[0].Addr().(*net.TCPAddr).IP.IsUnspecified() {
		t.Errorf("empty bind listeners = %v, want one wildcard listener", lns)
	}
}

// T5: the real top-of-chain order (useEdgeMiddleware, which Run calls) records
// the connection's address, leaves RemoteAddr alone, and resolves the caller's
// address from X-Forwarded-For only, never from X-Real-IP or True-Client-IP.
func TestEdgeMiddlewareRecordsTCPPeer(t *testing.T) {
	var peer, remote, addr string
	r := chi.NewRouter()
	useEdgeMiddleware(r, 1)
	r.Get("/", func(w http.ResponseWriter, req *http.Request) {
		peer, _ = platformmw.PeerAddrFromContext(req.Context())
		remote = req.RemoteAddr
		addr = clientaddr.Of(req)
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.20:5555"
	req.Header.Set("X-Real-IP", "127.0.0.1")
	req.Header.Set("True-Client-IP", "127.0.0.2")
	r.ServeHTTP(httptest.NewRecorder(), req)
	if peer != "192.168.1.20:5555" {
		t.Errorf("recorded peer = %q, want 192.168.1.20:5555", peer)
	}
	if remote != "192.168.1.20:5555" {
		t.Errorf("RemoteAddr rewritten to %q", remote)
	}
	if addr != "192.168.1.20" {
		t.Errorf("client address = %q, want the peer 192.168.1.20", addr)
	}
}

func TestEdgeMiddlewareHops(t *testing.T) {
	for _, tc := range []struct {
		hops int
		want string
	}{{0, "192.168.1.20"}, {1, "203.0.113.5"}} {
		var addr string
		r := chi.NewRouter()
		useEdgeMiddleware(r, tc.hops)
		r.Get("/", func(w http.ResponseWriter, req *http.Request) { addr = clientaddr.Of(req) })
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "192.168.1.20:5555"
		req.Header.Set("X-Forwarded-For", "6.6.6.6, 203.0.113.5")
		r.ServeHTTP(httptest.NewRecorder(), req)
		if addr != tc.want {
			t.Errorf("hops %d: client address = %q, want %q", tc.hops, addr, tc.want)
		}
	}
}
