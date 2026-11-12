package server

import (
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/go-chi/chi/v5"

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
// the connection's address even when RealIP is on and headers claim otherwise.
func TestEdgeMiddlewareRecordsTCPPeer(t *testing.T) {
	var peer, remote string
	r := chi.NewRouter()
	useEdgeMiddleware(r, true)
	r.Get("/", func(w http.ResponseWriter, req *http.Request) {
		peer, _ = platformmw.PeerAddrFromContext(req.Context())
		remote = req.RemoteAddr
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.20:5555"
	req.Header.Set("X-Forwarded-For", "127.0.0.1")
	req.Header.Set("X-Real-IP", "127.0.0.1")
	r.ServeHTTP(httptest.NewRecorder(), req)
	if peer != "192.168.1.20:5555" {
		t.Errorf("recorded peer = %q, want 192.168.1.20:5555", peer)
	}
	if remote == "192.168.1.20:5555" {
		t.Errorf("RealIP did not rewrite RemoteAddr (%q); the test proves nothing", remote)
	}
}
