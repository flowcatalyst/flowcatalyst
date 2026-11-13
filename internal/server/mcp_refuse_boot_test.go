package server

import (
	"context"
	"strings"
	"testing"

	platformmw "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/middleware"
)

// The server does not start with the MCP listener bound beyond loopback and
// no caller token: Run returns the error before it touches the pool or binds
// anything (a nil pool here would panic if it got further).
func TestRunRefusesUnauthenticatedWideMCPBind(t *testing.T) {
	// Run sets the process-wide session cookie setting; put it back so the
	// integration tests that follow see the default (not secure).
	t.Cleanup(func() { platformmw.SetSessionCookieSecure(false) })
	t.Setenv("FC_MCP_ENABLED", "true")
	t.Setenv("FC_MCP_BIND", "0.0.0.0")
	t.Setenv("FC_MCP_AUTH_TOKEN", "")

	err := Run(context.Background(), nil, LoadEnv(), RunOptions{})
	if err == nil {
		t.Fatal("Run started with MCP on 0.0.0.0 and no FC_MCP_AUTH_TOKEN")
	}
	if !strings.Contains(err.Error(), "FC_MCP_AUTH_TOKEN") {
		t.Fatalf("error does not name the variable to set: %v", err)
	}
}
