//go:build integration

package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// isolatedPGHome points HOME / the cache dirs at a temp dir so the test never
// touches the developer's embedded-Postgres runtime (embedded-postgres wipes
// its runtime directory on Start), and reuses an already-downloaded binary
// archive via symlink. Skips when none is cached rather than downloading.
func isolatedPGHome(t *testing.T) {
	t.Helper()
	realHome, err := os.UserHomeDir()
	require.NoError(t, err)
	src := filepath.Join(realHome, ".embedded-postgres-go")
	entries, _ := os.ReadDir(src)
	home := t.TempDir()
	dst := filepath.Join(home, ".embedded-postgres-go")
	require.NoError(t, os.MkdirAll(dst, 0o755))
	linked := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".txz" {
			require.NoError(t, os.Symlink(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())))
			linked++
		}
	}
	if linked == 0 {
		t.Skip("no cached embedded-postgres binary archive; not downloading in a test")
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("FC_FN_CLI_CREDENTIALS_PATH", filepath.Join(home, "fng-cli.json"))
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func listening(port int) bool {
	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// A boot that fails (or panics) after the embedded Postgres is up must stop it.
func TestRunStart_StopsEmbeddedPGWhenBootFailsAfterItStarts(t *testing.T) {
	for name, inject := range map[string]func() error{
		"error": func() error { return errors.New("injected boot failure") },
		"panic": func() error { panic("injected boot panic") },
	} {
		t.Run(name, func(t *testing.T) {
			isolatedPGHome(t)
			pgPort := freeTCPPort(t)
			dataDir := filepath.Join(t.TempDir(), "pg")

			up := false
			afterEmbeddedPGStart = func() error {
				up = listening(pgPort)
				return inject()
			}
			t.Cleanup(func() { afterEmbeddedPGStart = func() error { return nil } })

			cmd := newStartCmd()
			cmd.SetArgs([]string{
				"--embedded-db-port", fmt.Sprint(pgPort),
				"--embedded-db-path", dataDir,
				"--pid-file", filepath.Join(t.TempDir(), "fcdev.pid"),
				"--api-port", fmt.Sprint(freeTCPPort(t)),
				"--metrics-port", fmt.Sprint(freeTCPPort(t)),
			})
			func() {
				defer func() { _ = recover() }() // the panic case propagates after the defers ran
				_ = cmd.Execute()
			}()

			require.True(t, up, "postgres was up when the failure was injected")
			require.False(t, listening(pgPort), "postgres must be stopped after a failed boot")
		})
	}
}
