package oauthapi

import (
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
)

func TestMatchLoopbackRedirectURI(t *testing.T) {
	reg := auth.FcdevCLIRedirectURIs
	cases := []struct {
		uri  string
		want bool
	}{
		{"http://127.0.0.1:54321/callback", true},
		{"http://127.0.0.1:1/callback", true},
		{"http://127.0.0.1:65535/callback", true},
		{"http://[::1]:8123/callback", true},
		// port is required, canonical and in range
		{"http://127.0.0.1/callback", false}, // the bare registered form is matched by the exact pass, not here
		{"http://127.0.0.1:0/callback", false},
		{"http://127.0.0.1:65536/callback", false},
		{"http://127.0.0.1:-1/callback", false},
		{"http://127.0.0.1:0080/callback", false},
		{"http://127.0.0.1:80a/callback", false},
		{"http://127.0.0.1:/callback", false},
		// host tricks
		{"http://127.0.0.1.evil.com:5000/callback", false},
		{"http://127.0.0.1.:5000/callback", false},
		{"http://evil.com:5000/callback", false},
		{"http://localhost:5000/callback", false},
		{"http://127.0.0.2:5000/callback", false},
		{"http://0.0.0.0:5000/callback", false},
		{"http://[::ffff:127.0.0.1]:5000/callback", false},
		{"http://[0:0:0:0:0:0:0:1]:5000/callback", false},
		{"http://127.1:5000/callback", false},
		{"http://2130706433:5000/callback", false},
		{"http://HTTP://127.0.0.1:5000/callback", false},
		// userinfo
		{"http://evil.com@127.0.0.1:5000/callback", false},
		{"http://127.0.0.1:5000@evil.com/callback", false},
		{"http://127.0.0.1@evil.com:5000/callback", false},
		{"http://user:pw@127.0.0.1:5000/callback", false},
		// scheme
		{"https://127.0.0.1:5000/callback", false},
		{"ftp://127.0.0.1:5000/callback", false},
		{"HTTP://127.0.0.1:5000/callback", false},
		// path variations
		{"http://127.0.0.1:5000/callback/", false},
		{"http://127.0.0.1:5000/callback/x", false},
		{"http://127.0.0.1:5000/other", false},
		{"http://127.0.0.1:5000/", false},
		{"http://127.0.0.1:5000", false},
		{"http://127.0.0.1:5000/callback/../callback", false},
		{"http://127.0.0.1:5000//callback", false},
		{"http://127.0.0.1:5000/%63allback", false},
		{"http://127.0.0.1:5000/Callback", false},
		{"http://127.0.0.1:5000/callback;x", false},
		// other components
		{"http://127.0.0.1:5000/callback?x=1", false},
		{"http://127.0.0.1:5000/callback?", false},
		{"http://127.0.0.1:5000/callback#f", false},
		{"http://127.0.0.1:5000/callback#", false},
		{"http://127.0.0.1:5000\\@evil.com/callback", false},
		{"http://127.0.0.1:5000/callback ", false},
		{" http://127.0.0.1:5000/callback", false},
		{"http://127.0.0.1:5000/call\nback", false},
		{"//127.0.0.1:5000/callback", false},
		{"", false},
	}
	for _, c := range cases {
		if got := MatchLoopbackRedirectURI(c.uri, reg); got != c.want {
			t.Errorf("MatchLoopbackRedirectURI(%q) = %v, want %v", c.uri, got, c.want)
		}
	}
	// A host the client did not register is refused even in an otherwise
	// valid shape: only the registered loopback literal(s) are variable-port.
	if MatchLoopbackRedirectURI("http://[::1]:5000/callback", []string{"http://127.0.0.1/callback"}) {
		t.Error("[::1] matched a registration that only names 127.0.0.1")
	}
	if MatchLoopbackRedirectURI("http://127.0.0.1:5000/callback", []string{"http://localhost/callback"}) {
		t.Error("a localhost registration must not enable 127.0.0.1")
	}
	// A registration that already pins a port, or uses another host, enables nothing.
	if MatchLoopbackRedirectURI("http://127.0.0.1:5000/callback", []string{"http://127.0.0.1:5000/callback"}) {
		t.Error("pinned-port registration is the exact matcher's business, not the variable-port rule")
	}
}

// The exception belongs to the fcdev client alone: another client registered
// with the very same port-less loopback URIs gets exact matching only.
func TestRedirectAllowedLoopbackIsFcdevOnly(t *testing.T) {
	fc := &auth.OAuthClient{ClientID: auth.FcdevCLIClientID, RedirectURIs: auth.FcdevCLIRedirectURIs}
	other := &auth.OAuthClient{ClientID: "someone-else", RedirectURIs: auth.FcdevCLIRedirectURIs}
	const uri = "http://127.0.0.1:50000/callback"
	if !redirectAllowed(fc, uri) {
		t.Error("fcdev client should accept a loopback port")
	}
	if redirectAllowed(other, uri) {
		t.Error("another client must not get variable-port loopback redirects")
	}
	if redirectAllowed(fc, "http://localhost:50000/callback") || redirectAllowed(fc, "https://127.0.0.1:50000/callback") {
		t.Error("fcdev client accepted a non-registered host/scheme")
	}
}
