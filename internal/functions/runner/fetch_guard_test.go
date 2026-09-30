package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/netguard"
)

func fetchCaps(policy *netguard.Policy, allow ...string) *capabilities {
	return &capabilities{
		ver:  &version{describe: &abi.Describe{HTTPAllow: allow}},
		http: newHTTPClientWith(policy),
	}
}

func doFetch(t *testing.T, c *capabilities, target string) (abi.HTTPResponse, []byte, *abi.Error) {
	t.Helper()
	meta, _ := json.Marshal(abi.HTTPRequest{URL: target})
	out, body, aerr := c.fetch(context.Background(), meta, nil)
	var resp abi.HTTPResponse
	if aerr == nil {
		if err := json.Unmarshal(out, &resp); err != nil {
			t.Fatal(err)
		}
	}
	return resp, body, aerr
}

// localhostURL rewrites a 127.0.0.1 test-server URL to the name "localhost",
// which is allowlisted by name yet resolves to loopback.
func localhostURL(t *testing.T, srv *httptest.Server) (target, hostPort string) {
	t.Helper()
	u, _ := url.Parse(srv.URL)
	hostPort = "localhost:" + u.Port()
	return "http://" + hostPort + "/", hostPort
}

func TestFetchAllowlistedNameResolvingToLoopbackIsBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("secret")) }))
	defer srv.Close()
	target, hostPort := localhostURL(t, srv)

	_, _, aerr := doFetch(t, fetchCaps(&netguard.Policy{}, hostPort), target)
	if aerr == nil || aerr.Code != abi.CodeNotAllowed {
		t.Fatalf("aerr = %+v, want NOT_ALLOWED from the dial guard", aerr)
	}
}

func TestFetchLoopbackOnlyWithDevPolicy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("hi")) }))
	defer srv.Close()
	target, hostPort := localhostURL(t, srv)

	resp, body, aerr := doFetch(t, fetchCaps(&netguard.Policy{AllowLoopback: true}, hostPort), target)
	if aerr != nil || resp.Status != 200 || string(body) != "hi" {
		t.Fatalf("resp=%+v body=%q aerr=%+v", resp, body, aerr)
	}
}

func TestFetchIPLiteralsAndMetadataBlocked(t *testing.T) {
	for _, target := range []string{"http://169.254.169.254/latest/meta-data", "http://127.0.0.1:9/", "http://10.0.0.1/"} {
		u, _ := url.Parse(target)
		// Even when the (unvalidated) allowlist names the literal, the dial refuses.
		_, _, aerr := doFetch(t, fetchCaps(&netguard.Policy{}, u.Host), target)
		if aerr == nil || aerr.Code != abi.CodeNotAllowed {
			t.Errorf("%s: aerr = %+v, want NOT_ALLOWED", target, aerr)
		}
	}
}

func TestFetchRedirectToPrivateAddressBlocked(t *testing.T) {
	// The redirect target is allowlisted by name, so only the dial guard stops it.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data", http.StatusFound)
	}))
	defer srv.Close()
	target, hostPort := localhostURL(t, srv)

	policy := &netguard.Policy{AllowLoopback: true} // first hop is fine
	_, _, aerr := doFetch(t, fetchCaps(policy, hostPort, "169.254.169.254"), target)
	if aerr == nil || aerr.Code != abi.CodeNotAllowed || !strings.Contains(aerr.Message, "link-local") {
		t.Fatalf("aerr = %+v, want NOT_ALLOWED link-local", aerr)
	}
}

func TestFetchResponseCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(make([]byte, maxHTTPResponseBytes+1))
	}))
	defer srv.Close()
	target, hostPort := localhostURL(t, srv)

	_, _, aerr := doFetch(t, fetchCaps(&netguard.Policy{AllowLoopback: true}, hostPort), target)
	if aerr == nil || aerr.Code != abi.CodeTooLarge {
		t.Fatalf("aerr = %+v, want TOO_LARGE", aerr)
	}
}

func TestHostAllowedPorts(t *testing.T) {
	cases := []struct {
		allow  string
		target string
		want   bool
	}{
		{"api.acme.com", "https://api.acme.com/x", true},
		{"api.acme.com", "http://api.acme.com/x", true},
		{"api.acme.com", "https://api.acme.com:443/x", true},
		{"api.acme.com", "https://api.acme.com:8443/x", false},
		{"api.acme.com", "http://api.acme.com:5432/", false},
		{"api.acme.com:8443", "https://api.acme.com:8443/", true},
		{"api.acme.com:8443", "https://api.acme.com/", false},
		{"*.acme.com", "https://a.acme.com/", true},
		{"*.acme.com", "https://a.acme.com:9200/", false},
		{"*.acme.com", "https://acme.com/", false},
	}
	for _, c := range cases {
		u, _ := url.Parse(c.target)
		if got := hostAllowed([]string{c.allow}, u); got != c.want {
			t.Errorf("hostAllowed(%q, %q) = %v, want %v", c.allow, c.target, got, c.want)
		}
	}
}
