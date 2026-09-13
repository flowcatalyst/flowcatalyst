package server

import (
	"strings"
	"testing"
)

// A half-configured credential is a deployment mistake, and it is refused at
// the composition root rather than on the first fetch: otherwise the router
// 401s against its own platform on every attempt, for ever, with only a
// warning to show for it.
func TestNewRouterServer_RefusesHalfConfiguredCredentials(t *testing.T) {
	base := EnvCfg{
		RouterConfigURL:   "http://platform.test/api/dispatch/router-config",
		RouterPlatformURL: "http://platform.test",
	}

	t.Run("id without secret", func(t *testing.T) {
		cfg := base
		cfg.RouterClientID = "router-client"
		_, err := newRouterServer(cfg, nil)
		requireErrorContaining(t, err, "must be set together")
	})

	t.Run("secret without id", func(t *testing.T) {
		cfg := base
		cfg.RouterClientSecret = "shhh"
		_, err := newRouterServer(cfg, nil)
		requireErrorContaining(t, err, "must be set together")
	})

	// The credential belongs to one platform. Without naming it, "which of
	// several config URLs gets the token" would have to be positional.
	t.Run("credentials without a platform url", func(t *testing.T) {
		cfg := base
		cfg.RouterPlatformURL = ""
		cfg.RouterClientID = "router-client"
		cfg.RouterClientSecret = "shhh"
		_, err := newRouterServer(cfg, nil)
		requireErrorContaining(t, err, "FC_ROUTER_PLATFORM_URL")
	})
}

// Both set, with a platform URL, is the configured deployment — and no
// credentials at all is the unauthenticated router that existed before.
func TestNewRouterServer_AcceptsBothOrNeither(t *testing.T) {
	base := EnvCfg{
		RouterConfigURL:   "http://platform.test/api/dispatch/router-config",
		RouterPlatformURL: "http://platform.test",
	}

	withCreds := base
	withCreds.RouterClientID = "router-client"
	withCreds.RouterClientSecret = "shhh"
	srv, err := newRouterServer(withCreds, nil)
	if err != nil {
		t.Fatalf("a fully configured credential must be accepted: %v", err)
	}
	if srv.ConfigSource == nil || srv.ConfigSource.Credentials == nil {
		t.Fatal("the config source must carry the credential")
	}
	if srv.ConfigSource.CredentialOrigin != "http://platform.test" {
		t.Errorf("CredentialOrigin = %q, want the platform's origin", srv.ConfigSource.CredentialOrigin)
	}

	srv, err = newRouterServer(base, nil)
	if err != nil {
		t.Fatalf("no credentials must remain valid: %v", err)
	}
	if srv.ConfigSource != nil && srv.ConfigSource.Credentials != nil {
		t.Error("a router with no credentials must fetch unauthenticated")
	}
}

func requireErrorContaining(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error mentioning %q, got nil", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error = %v, want it to mention %q", err, want)
	}
}
