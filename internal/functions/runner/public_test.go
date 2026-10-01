package runner

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
)

func TestPublicEntry(t *testing.T) {
	h := start(t, fnDoc(ver(1, control.RoleLive), ver(2, control.RoleAlias+"qa")))
	h.cp.push(control.Desired{
		Pool: "default", Functions: []control.Function{fnDoc(ver(1, control.RoleLive), ver(2, control.RoleAlias+"qa"))},
		Routes: []control.Route{
			{Hostname: "api.acme.com", PathPrefix: "/", Address: "app.hello"},
			{Hostname: "api.acme.com", PathPrefix: "/v2", Address: "app.hello", Alias: "qa"},
			{Hostname: "Other.Acme.com.", PathPrefix: "/hello/", Address: "app.hello"},
		},
	})
	h.waitServing("app.hello", 1)
	h.waitReady("app.hello", 2)
	pub := httptest.NewServer(h.r.PublicHandler())
	defer pub.Close()

	call := func(host, path string) (int, abi.Request) {
		t.Helper()
		req, _ := http.NewRequest("GET", pub.URL+path, nil)
		req.Host = host
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var m abi.Request
		_ = json.NewDecoder(resp.Body).Decode(&m)
		return resp.StatusCode, m
	}
	if code, m := call("api.acme.com", "/meta"); code != 200 || m.Version != 1 || m.Path != "/meta" || !m.Public || m.Host != "api.acme.com" {
		t.Errorf("root route: %d %+v", code, m)
	}
	if code, m := call("api.acme.com:443", "/v2/meta"); code != 200 || m.Version != 2 || m.Path != "/meta" {
		t.Errorf("longest prefix to the qa alias, prefix stripped: %d v%d %s", code, m.Version, m.Path)
	}
	if code, m := call("other.acme.com", "/hello/meta"); code != 200 || m.Path != "/meta" {
		t.Errorf("case-insensitive host, trailing-slash prefix: %d %s", code, m.Path)
	}
	if code, _ := call("other.acme.com", "/helloworld"); code != 404 {
		t.Errorf("a prefix matches whole segments only: %d", code)
	}
	if code, _ := call("unknown.com", "/meta"); code != 404 {
		t.Errorf("unknown host: %d", code)
	}
	if code, _ := call("api.acme.com", "/fn/app.hello@v2/meta"); code != 404 {
		t.Errorf("the private path form reached a function publicly: %d", code)
	}
}

func TestClientAddr(t *testing.T) {
	r := &Runner{cfg: Config{TrustedProxies: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}}}
	cases := []struct {
		peer, xff string
		public    bool
		want      string
	}{
		{"203.0.113.9:5000", "1.2.3.4", true, "203.0.113.9"},             // untrusted peer: header ignored
		{"10.0.0.5:5000", "1.2.3.4", true, "1.2.3.4"},                    // trusted proxy names the client
		{"10.0.0.5:5000", "6.6.6.6, 1.2.3.4, 10.0.0.9", true, "1.2.3.4"}, // stop at the first untrusted hop
		{"10.0.0.5:5000", "1.2.3.4", false, "10.0.0.5"},                  // private entry: the peer
	}
	for _, c := range cases {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = c.peer
		req.Header.Set("X-Forwarded-For", c.xff)
		if got := r.clientAddr(req, c.public); got != c.want {
			t.Errorf("peer %s xff %q public %v: %s, want %s", c.peer, c.xff, c.public, got, c.want)
		}
	}
}

func TestRouteTableAliasPrefixes(t *testing.T) {
	tbl := buildRoutes([]control.Route{
		{Hostname: "myapp.acme.com", PathPrefix: "/", Address: "app.hello", AliasPrefixes: []string{"qa", "staging"}},
		{Hostname: "closed.acme.com", PathPrefix: "/", Address: "app.closed"},
		{Hostname: "qa-exact.acme.com", PathPrefix: "/", Address: "app.exact"},
		{Hostname: "exact.acme.com", PathPrefix: "/", Address: "app.other", AliasPrefixes: []string{"qa"}},
		{Hostname: "staging-myapp.acme.com", PathPrefix: "/only", Address: "app.staged"},
		{Hostname: "my-app.acme.com", PathPrefix: "/", Address: "app.hyph", AliasPrefixes: []string{"qa"}},
	})
	cases := []struct {
		name, host, path string
		wantOK           bool
		wantAddr, alias  string
	}{
		{"exact host is live", "myapp.acme.com", "/x", true, "app.hello", ""},
		{"opted-in prefix serves the alias", "qa-myapp.acme.com", "/x", true, "app.hello", "qa"},
		{"exact host with no matching path does not fall through to derivation", "staging-myapp.acme.com:443", "/x", false, "", ""},
		{"second opted-in prefix on a path-matched base", "staging-myapp.acme.com", "/only/x", true, "app.staged", ""},
		{"prefix not opted in", "dev-myapp.acme.com", "/x", false, "", ""},
		{"route without prefixes is exact only", "qa-closed.acme.com", "/x", false, "", ""},
		{"an exact route beats the derivation", "qa-exact.acme.com", "/x", true, "app.exact", ""},
		{"hyphenated base: split at the first hyphen", "qa-my-app.acme.com", "/x", true, "app.hyph", "qa"},
		{"one level only", "qa-staging-myapp.acme.com", "/x", false, "", ""},
		{"no hyphen, no derivation", "unknown.acme.com", "/x", false, "", ""},
		{"case and trailing dot", "QA-MyApp.Acme.com.", "/x", true, "app.hello", "qa"},
	}
	for _, c := range cases {
		rt, _, ok := tbl.match(c.host, c.path)
		if ok != c.wantOK || rt.Address != c.wantAddr || rt.Alias != c.alias {
			t.Errorf("%s: match(%q) = ok %v addr %q alias %q; want ok %v addr %q alias %q",
				c.name, c.host, ok, rt.Address, rt.Alias, c.wantOK, c.wantAddr, c.alias)
		}
	}
}
