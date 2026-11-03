package main

import (
	"crypto/rand"
	"encoding/base64"
	"path/filepath"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/mcp"
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
