package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `fcdev init` must resolve the field-encryption key exactly as the running
// instance does, or the client secret it mints cannot be verified.
func TestResolveAppKey(t *testing.T) {
	t.Run("env wins and nothing is written", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("FLOWCATALYST_APP_KEY", "from-env")
		got, err := resolveAppKey(filepath.Join(dir, "embedded-pg"))
		if err != nil || got != "from-env" {
			t.Fatalf("got %q, %v", got, err)
		}
		if _, err := os.Stat(filepath.Join(dir, "app-key")); !os.IsNotExist(err) {
			t.Fatalf("app-key file must not be created when env is set: %v", err)
		}
	})

	t.Run("persisted file is used", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("FLOWCATALYST_APP_KEY", "")
		if err := os.WriteFile(filepath.Join(dir, "app-key"), []byte("from-file\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := resolveAppKey(filepath.Join(dir, "embedded-pg"))
		if err != nil || got != "from-file" {
			t.Fatalf("got %q, %v", got, err)
		}
		if os.Getenv("FLOWCATALYST_APP_KEY") != "from-file" {
			t.Fatal("resolved key must be exported for encryption.FromEnv")
		}
	})

	t.Run("generated once, persisted 0600, then stable", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("FLOWCATALYST_APP_KEY", "")
		first, err := resolveAppKey(filepath.Join(dir, "embedded-pg"))
		if err != nil || strings.TrimSpace(first) == "" {
			t.Fatalf("got %q, %v", first, err)
		}
		fi, err := os.Stat(filepath.Join(dir, "app-key"))
		if err != nil || fi.Mode().Perm() != 0o600 {
			t.Fatalf("stat: %v mode=%v", err, fi)
		}
		t.Setenv("FLOWCATALYST_APP_KEY", "")
		second, err := resolveAppKey(filepath.Join(dir, "embedded-pg"))
		if err != nil || second != first {
			t.Fatalf("second resolve %q != first %q (%v)", second, first, err)
		}
	})
}
