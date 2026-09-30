package server

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/http2"
)

func TestLoadFunctionRunnerEnv(t *testing.T) {
	t.Setenv("FC_FUNCTIONS_ENABLED", "true")
	t.Setenv("FC_FUNCTIONS_TRUSTED_PROXIES", "10.0.0.0/8, 192.168.1.7, nonsense")
	c := LoadFunctionRunnerEnv(EnvCfg{PlatformEnabled: true, APIPort: 8080})
	if !c.Enabled || c.Port != DefaultFunctionRunnerPort || c.Pool != "default" || c.PlatformURL != "http://127.0.0.1:8080" {
		t.Fatalf("config = %+v", c)
	}
	want := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("192.168.1.7/32")}
	if len(c.TrustedProxies) != 2 || c.TrustedProxies[0] != want[0] || c.TrustedProxies[1] != want[1] {
		t.Fatalf("trusted proxies = %v", c.TrustedProxies)
	}
	if c.SetMemoryLimit {
		t.Error("GOMEMLIMIT set while the platform shares the process")
	}
	if c := LoadFunctionRunnerEnv(EnvCfg{}); !c.SetMemoryLimit || c.PlatformURL != "" {
		t.Errorf("a runner-only process: %+v", c)
	}
}

// TestCleartextHTTP2Protocols: a listener with these protocols answers the
// router's h2c prior-knowledge requests and plain HTTP/1.1 alike. Without
// h2c, every delivery the router mediated to an http:// platform callback or
// function runner failed with "http2: frame too large".
func TestCleartextHTTP2Protocols(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{
		Handler:   http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, r.Proto) }),
		Protocols: cleartextHTTP2Protocols(),
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	url := "http://" + ln.Addr().String() + "/"

	h2c := &http.Client{Transport: &http2.Transport{
		AllowHTTP: true,
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
	}}
	for name, c := range map[string]*http.Client{"h2c": h2c, "http/1.1": {}} {
		resp, err := c.Get(url)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		want := map[string]string{"h2c": "HTTP/2.0", "http/1.1": "HTTP/1.1"}[name]
		if string(body) != want {
			t.Errorf("%s: served as %q, want %q", name, body, want)
		}
	}
}

// A client that stalls mid-body is cut off by the server's ReadTimeout.
func TestRunnerServerCutsOffSlowBody(t *testing.T) {
	if s := newRunnerServer(":0", http.NewServeMux()); s.ReadTimeout != runnerReadTimeout || s.ReadTimeout <= 0 || s.IdleTimeout <= 0 {
		t.Fatalf("timeouts not configured: read %v idle %v", s.ReadTimeout, s.IdleTimeout)
	}
	s := newRunnerServer("127.0.0.1:0", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusRequestTimeout)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	s.ReadTimeout = 300 * time.Millisecond
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = s.Serve(ln) }()
	t.Cleanup(func() { _ = s.Close() })

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, _ = io.WriteString(conn, "POST /x HTTP/1.1\r\nHost: a\r\nContent-Length: 1000\r\n\r\nabc")
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	start := time.Now()
	b, _ := io.ReadAll(conn) // returns when the server closes the connection
	if el := time.Since(start); el > 3*time.Second {
		t.Fatalf("slow body held the connection for %v", el)
	}
	if strings.Contains(string(b), " 200 ") {
		t.Fatalf("stalled body was served: %q", b)
	}
}
