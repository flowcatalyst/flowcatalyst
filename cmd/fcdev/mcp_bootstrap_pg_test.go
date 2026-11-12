//go:build integration

package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// The bootstrapped MCP account holds exactly mcpBootstrapRole.
func TestBootstrapMCPCredentials_GrantsMCPBootstrapRole(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	t.Setenv("FLOWCATALYST_APP_KEY", key)
	t.Setenv("FC_MCP_CREDENTIALS_PATH", filepath.Join(t.TempDir(), "mcp-credentials.json"))
	ctx := context.Background()
	require.NoError(t, bootstrapMCPCredentials(ctx, testpg.Pool(t), "http://localhost:8080"))
	_, roles := bootstrapRoles(t, ctx, mcpLocalClientID)
	assert.Equal(t, []string{mcpBootstrapRole}, roles)
}
