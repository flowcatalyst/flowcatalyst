package netguard

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"testing"
	"time"
)

func TestCheckIP(t *testing.T) {
	strict := &Policy{}
	private := &Policy{AllowPrivate: true}
	loop := &Policy{AllowLoopback: true}
	cases := []struct {
		ip                 string
		strict, priv, loop bool // true = allowed
	}{
		{"8.8.8.8", true, true, true},
		{"2606:4700:4700::1111", true, true, true},
		{"127.0.0.1", false, false, true},
		{"::1", false, false, true},
		{"::ffff:127.0.0.1", false, false, true},
		{"10.1.2.3", false, true, false},
		{"172.16.0.1", false, true, false},
		{"192.168.1.1", false, true, false},
		{"100.64.0.1", false, true, false},
		{"fd12::1", false, true, false},
		{"169.254.169.254", false, false, false},
		{"::ffff:169.254.169.254", false, false, false},
		{"fe80::1", false, false, false},
		{"fd00:ec2::254", false, false, false},
		{"0.0.0.0", false, false, false},
		{"::", false, false, false},
		{"224.0.0.1", false, false, false},
	}
	for _, c := range cases {
		ip := netip.MustParseAddr(c.ip)
		for name, pc := range map[string]struct {
			p    *Policy
			want bool
		}{"strict": {strict, c.strict}, "private": {private, c.priv}, "loopback": {loop, c.loop}} {
			err := pc.p.CheckIP(ip)
			if (err == nil) != pc.want {
				t.Errorf("%s policy, %s: allowed=%v, want %v (err %v)", name, c.ip, err == nil, pc.want, err)
			}
			if err != nil && !errors.Is(err, ErrBlocked) {
				t.Errorf("%s: error does not wrap ErrBlocked: %v", c.ip, err)
			}
		}
	}
}

func TestValidateURL(t *testing.T) {
	p := &Policy{}
	ok := []string{"https://example.com/hook", "http://8.8.8.8:8080/x", "https://hooks.internal.example.com/a"}
	bad := []string{
		"", "ftp://example.com", "example.com/hook", "https://", "https://user:pw@example.com/",
		"http://localhost/x", "http://LOCALHOST:8080/x", "http://foo.localhost/x", "http://127.0.0.1/x",
		"http://[::1]/x", "http://169.254.169.254/latest/meta-data", "http://10.0.0.5/x", "http://0.0.0.0/",
		"http://[fd00:ec2::254]/",
	}
	for _, u := range ok {
		if err := p.ValidateURL(u); err != nil {
			t.Errorf("%q rejected: %v", u, err)
		}
	}
	for _, u := range bad {
		if err := p.ValidateURL(u); err == nil {
			t.Errorf("%q accepted", u)
		}
	}
	dev := &Policy{AllowLoopback: true, AllowPrivate: true}
	for _, u := range []string{"http://localhost:9000/x", "http://127.0.0.1/x", "http://10.0.0.5/x"} {
		if err := dev.ValidateURL(u); err != nil {
			t.Errorf("dev policy rejected %q: %v", u, err)
		}
	}
	if err := dev.ValidateURL("http://169.254.169.254/"); err == nil {
		t.Error("dev policy must still block cloud metadata")
	}
}

func TestAllowedHostSkipsChecks(t *testing.T) {
	p := &Policy{}
	p.AllowURL("http://localhost:8095/fn")
	if err := p.ValidateURL("http://localhost:8095/other"); err != nil {
		t.Errorf("allowed host rejected: %v", err)
	}
	if err := p.ValidateURL("http://localhost:9999/other"); err == nil {
		t.Error("a different port on localhost must stay blocked")
	}
}

func TestDialContextBlocksResolvedLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	defer srv.Close()
	base := &net.Dialer{Timeout: 2 * time.Second}

	client := func(p *Policy) *http.Client {
		return &http.Client{Transport: &http.Transport{DialContext: p.DialContext(base)}, Timeout: 3 * time.Second}
	}
	// The listener is on 127.0.0.1: the strict policy must refuse the connect
	// itself, not merely the URL.
	if _, err := client(&Policy{}).Get(srv.URL); err == nil || !errors.Is(err, ErrBlocked) {
		t.Fatalf("strict policy reached loopback (err=%v)", err)
	}
	// Reaching it by name must not get around the check.
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	if _, err := client(&Policy{}).Get("http://localhost:" + port); err == nil {
		t.Fatal("strict policy reached loopback via localhost")
	}
	if resp, err := client(&Policy{AllowLoopback: true}).Get(srv.URL); err != nil {
		t.Fatalf("loopback policy blocked: %v", err)
	} else {
		resp.Body.Close()
	}
	exempt := &Policy{}
	exempt.AllowURL(srv.URL)
	if resp, err := client(exempt).Get(srv.URL); err != nil {
		t.Fatalf("exempted host blocked: %v", err)
	} else {
		resp.Body.Close()
	}
	_ = context.Background()
}

func TestAllowedHostPatterns(t *testing.T) {
	p := &Policy{}
	p.AllowHost("*.fn.svc:8095")
	p.AllowURL("http://{pool}.runners.svc:8095")
	for _, u := range []string{"http://a.fn.svc:8095/x", "http://default.runners.svc:8095/x"} {
		if !p.hostAllowed(hostPortOfString(t, u)) {
			t.Errorf("%s not allowed", u)
		}
	}
	if p.hostAllowed("evil.example.com:8095") {
		t.Error("unrelated host allowed")
	}
}

func hostPortOfString(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return hostPortOf(u)
}

func TestFromEnv(t *testing.T) {
	t.Setenv("FC_DELIVERY_ALLOW_LOOPBACK", "true")
	t.Setenv("FC_DELIVERY_ALLOW_HOSTS", "a.example:1, b.example:2 ,")
	p := FromEnv()
	if !p.AllowLoopback || p.AllowPrivate || len(p.allowHosts) != 2 {
		t.Fatalf("got %+v", p)
	}
}
