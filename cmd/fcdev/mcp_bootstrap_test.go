package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/mcp"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/seed"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
)

func TestHealMCPCredentialsFileRewritesAMismatchedFile(t *testing.T) {
	t.Setenv("FC_MCP_CREDENTIALS_PATH", filepath.Join(t.TempDir(), "mcp-credentials.json"))
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	enc, err := encryption.New(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}
	ref, err := enc.Encrypt("the-real-secret")
	if err != nil {
		t.Fatal(err)
	}
	// Another instance overwrote the file with its own client.
	if err := mcp.WriteCredentialsFile(mcpLocalClientID, "someone-elses-secret", "http://localhost:18480"); err != nil {
		t.Fatal(err)
	}
	if err := healMCPCredentialsFile(enc, &ref, "http://localhost:8080"); err != nil {
		t.Fatal(err)
	}
	id, s, u, ok := mcp.ReadCredentialsFile()
	if !ok || id != mcpLocalClientID || s != "the-real-secret" || u != "http://localhost:8080" {
		t.Fatalf("not healed: %q %q %q %v", id, s, u, ok)
	}
	// A matching file is left alone (and a missing secret is an error, not a silent skip).
	if err := healMCPCredentialsFile(enc, &ref, "http://localhost:8080"); err != nil {
		t.Fatal(err)
	}
	if err := healMCPCredentialsFile(enc, nil, "http://localhost:8080"); err == nil {
		t.Fatal("want an error for a client with no stored secret")
	}
}

// The permissions the MCP tools and resources need, from the endpoints they
// call (internal/mcp/tools.go, resources.go): event types, subscriptions,
// applications, roles (and their per-application list), and the developer
// OpenAPI read. /api/me and /api/me/applications need only authentication.
var mcpRequiredPermissions = []string{
	"platform:messaging:event-type:view",
	"platform:messaging:subscription:view",
	"platform:admin:application:view",
	"platform:iam:role:view",
	"platform:developer:application-openapi:view",
}

// The local MCP account is granted the least seeded role that covers every
// read it makes. None does today, so it keeps super-admin; this pins why, and
// fails (so the grant can be narrowed) the day a seeded role covers the set.
func TestMCPBootstrapRole(t *testing.T) {
	missing := func(r role.Role) []string {
		var out []string
		for _, want := range mcpRequiredPermissions {
			if !slices.Contains(r.Permissions, want) {
				out = append(out, want)
			}
		}
		return out
	}
	var super role.Role
	for _, r := range seed.PlatformRoles() {
		if r.Name == "platform:super-admin" {
			super = r
			continue
		}
		if len(missing(r)) == 0 {
			t.Errorf("seeded role %s now covers every permission the MCP tools need: grant it (mcpBootstrapRole) instead of super-admin", r.Name)
		}
		if r.Name == "platform:viewer" {
			if got := missing(r); !slices.Equal(got, []string{"platform:developer:application-openapi:view"}) {
				t.Errorf("platform:viewer is expected to lack only the developer OpenAPI view; missing %v", got)
			}
		}
	}
	if mcpBootstrapRole != super.Name {
		t.Errorf("mcpBootstrapRole = %q, want %q while no narrower seeded role covers the MCP reads", mcpBootstrapRole, super.Name)
	}
	if !slices.Contains(super.Permissions, "platform:*:*:*") {
		t.Errorf("super-admin must hold the wildcard, has %v", super.Permissions)
	}
}

// `fcdev mcp --http` applies the same rule as the fc-server subsystem: no
// network-reachable listener without a bearer token, and loopback still works
// without one.
func TestServeMCPHTTPBindAndBearer(t *testing.T) {
	srv := mcp.New(mcp.Config{BaseURL: "http://127.0.0.1:1"})

	err := serveMCPHTTP(context.Background(), srv, "0.0.0.0:0", "http://x", "")
	if err == nil || !strings.Contains(err.Error(), "FC_MCP_AUTH_TOKEN") {
		t.Fatalf("0.0.0.0 without a token must be refused with a clear error, got %v", err)
	}

	// Loopback without a token starts (and stops when the context ends).
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serveMCPHTTP(ctx, srv, "127.0.0.1:0", "http://x", "") }()
	time.Sleep(200 * time.Millisecond)
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("loopback without a token must start: %v", err)
	}

	// Non-loopback with a token starts.
	ctx2, cancel2 := context.WithCancel(context.Background())
	done2 := make(chan error, 1)
	go func() { done2 <- serveMCPHTTP(ctx2, srv, "0.0.0.0:0", "http://x", "tok") }()
	time.Sleep(200 * time.Millisecond)
	cancel2()
	if err := <-done2; err != nil {
		t.Fatalf("0.0.0.0 with a token must start: %v", err)
	}
}
