package server

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
)

type fakeClients struct {
	byID  map[string]*auth.OAuthClient
	calls int
	err   error
}

func (f *fakeClients) FindByClientID(_ context.Context, id string) (*auth.OAuthClient, error) {
	f.calls++
	return f.byID[id], f.err
}

func TestFormDestination(t *testing.T) {
	portal := "pc_1"
	f := &fakeClients{byID: map[string]*auth.OAuthClient{
		"app1":     {ClientID: "app1", Active: true, RedirectURIs: []string{"https://app.example.com/cb", "https://*.tenants.example.com/*"}},
		"other":    {ClientID: "other", Active: true, RedirectURIs: []string{"https://other.example.org/cb"}},
		"off":      {ClientID: "off", Active: false, RedirectURIs: []string{"https://off.example.com/cb"}},
		"portal":   {ClientID: "portal", Active: true, PortalClientID: &portal, RedirectURIs: []string{"https://portal.example.com/cb"}},
		"loopback": {ClientID: "loopback", Active: true, RedirectURIs: []string{"http://localhost:8090/cb"}},
	}}
	fd := formDestination(f)
	ctx := context.Background()
	for _, c := range []struct {
		name, client, uri, want string
	}{
		{"registered", "app1", "https://app.example.com/cb", "https://app.example.com"},
		{"registered by wildcard", "app1", "https://a.tenants.example.com/x?y=1", "https://a.tenants.example.com"},
		{"loopback with port", "loopback", "http://localhost:8090/cb", "http://localhost:8090"},
		{"unregistered uri", "app1", "https://evil.example/cb", ""},
		{"crafted pair: another client's uri", "app1", "https://other.example.org/cb", ""},
		{"crafted pair: userinfo", "app1", "https://app.example.com@evil.example/cb", ""},
		{"crafted pair: wildcard cannot cross a dot", "app1", "https://a.b.tenants.example.com/x", ""},
		{"unknown client", "nope", "https://app.example.com/cb", ""},
		{"inactive client", "off", "https://off.example.com/cb", ""},
		{"portal client (enters through /portal/authorize)", "portal", "https://portal.example.com/cb", ""},
		{"non-http scheme", "app1", "javascript:alert(1)", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			origin, ok := fd(ctx, c.client, c.uri)
			assert.Equal(t, c.want, origin)
			assert.Equal(t, c.want != "", ok)
		})
	}
}

func TestFormDestinationCachesAndSurvivesErrors(t *testing.T) {
	f := &fakeClients{byID: map[string]*auth.OAuthClient{"app1": {ClientID: "app1", Active: true, RedirectURIs: []string{"https://app.example.com/cb"}}}}
	fd := formDestination(f)
	for range 5 {
		fd(context.Background(), "app1", "https://app.example.com/cb")
		fd(context.Background(), "crafted", "https://app.example.com/cb")
	}
	assert.Equal(t, 2, f.calls, "known and unknown clients are each looked up once")

	bad := &fakeClients{err: errors.New("db down")}
	fd = formDestination(bad)
	_, ok := fd(context.Background(), "app1", "https://app.example.com/cb")
	assert.False(t, ok, "a failed lookup adds nothing")
	fd(context.Background(), "app1", "https://app.example.com/cb")
	assert.Equal(t, 2, bad.calls, "a failure is not cached")
}
