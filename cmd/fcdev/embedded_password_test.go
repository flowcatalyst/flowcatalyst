package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The default URL is the well-known one developers' own applications use. It
// must not change.
func TestEmbeddedDBURL(t *testing.T) {
	assert.Equal(t, "postgresql://postgres:postgres@localhost:15432/flowcatalyst?sslmode=disable",
		embeddedDBURL(defaultEmbeddedDBPassword, 15432))
	assert.Equal(t, "postgresql://postgres:s3cr%2Ft%40x@localhost:7000/flowcatalyst?sslmode=disable",
		embeddedDBURL("s3cr/t@x", 7000), "special characters are escaped")
}

// Every command that talks to the embedded database takes its password from
// the same flag/env resolution and builds its URL with embeddedDBURL.
func TestEmbeddedPasswordResolution_FlagBeatsEnvBeatsDefault(t *testing.T) {
	commands := map[string]func() *cobra.Command{
		"start": newStartCmd, "fresh": newFreshCmd, "init": newInitCmd, "db upgrade": newDBUpgradeCmd,
	}
	for name, mk := range commands {
		t.Run(name, func(t *testing.T) {
			t.Setenv(embeddedDBPasswordEnv, "")
			cmd := mk()
			assert.Equal(t, "postgres", resolveEmbeddedDBPassword(cmd), "default unchanged")

			t.Setenv(embeddedDBPasswordEnv, "from-env")
			assert.Equal(t, "from-env", resolveEmbeddedDBPassword(mk()))

			cmd = mk()
			require.NoError(t, cmd.Flags().Parse([]string{"--embedded-db-password", "from-flag"}))
			assert.Equal(t, "from-flag", resolveEmbeddedDBPassword(cmd), "the flag wins over the environment")
		})
	}
	t.Run("start carries it in its options", func(t *testing.T) {
		t.Setenv(embeddedDBPasswordEnv, "from-env")
		cmd := newStartCmd()
		require.NoError(t, cmd.Flags().Parse(nil))
		assert.Equal(t, "from-env", optsFromFlags(cmd).EmbeddedDBPassword)
	})
	t.Run("the flag default does not leak the environment into --help", func(t *testing.T) {
		t.Setenv(embeddedDBPasswordEnv, "from-env")
		assert.Empty(t, newStartCmd().Flags().Lookup("embedded-db-password").DefValue)
	})
}

// The embedded server is loopback only: the library passes no listen address,
// so we set it explicitly rather than rely on the generated postgresql.conf.
func TestEmbeddedPGListensOnLoopbackOnly(t *testing.T) {
	assert.Equal(t, map[string]string{"listen_addresses": "localhost"}, embeddedPGStartParameters)
}

func TestValidateEmbeddedDBPassword(t *testing.T) {
	for _, ok := range []string{"postgres", "Abc123!#$%&()*+,-./:;<=>?@[]^_`{|}~"} {
		assert.NoError(t, validateEmbeddedDBPassword(ok), ok)
	}
	for _, bad := range []string{"", "has space", "it's", `q"uote`, `back\slash`, "tab\t", "café"} {
		assert.Error(t, validateEmbeddedDBPassword(bad), bad)
	}
}

func fakeCluster(t *testing.T, dataPath string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(dataPath, "data"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dataPath, "data", "PG_VERSION"), []byte("18\n"), 0o644))
}

func TestPrepareEmbeddedPassword(t *testing.T) {
	t.Run("new directory, default password: no marker, any later start with the default works", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, prepareEmbeddedPassword(dir, "postgres"))
		assert.NoFileExists(t, filepath.Join(dir, embeddedPasswordMarker))
		fakeCluster(t, dir)
		require.NoError(t, prepareEmbeddedPassword(dir, "postgres"))
	})
	t.Run("new directory with a custom password: recorded, 0600, not the password itself", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, prepareEmbeddedPassword(dir, "hunter2hunter2"))
		b, err := os.ReadFile(filepath.Join(dir, embeddedPasswordMarker))
		require.NoError(t, err)
		assert.NotContains(t, string(b), "hunter2")
		st, err := os.Stat(filepath.Join(dir, embeddedPasswordMarker))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), st.Mode().Perm())

		fakeCluster(t, dir)
		require.NoError(t, prepareEmbeddedPassword(dir, "hunter2hunter2"), "same password")
		err = prepareEmbeddedPassword(dir, "postgres")
		require.Error(t, err, "default against a custom cluster")
		assert.Contains(t, err.Error(), "different password")
		assert.NotContains(t, err.Error(), "hunter2")
		require.Error(t, prepareEmbeddedPassword(dir, "another-one"))
	})
	t.Run("existing default cluster with no marker rejects a custom password", func(t *testing.T) {
		dir := t.TempDir()
		fakeCluster(t, dir)
		require.NoError(t, prepareEmbeddedPassword(dir, "postgres"))
		err := prepareEmbeddedPassword(dir, "custom-password")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--embedded-db-reset")
	})
	t.Run("a re-created directory forgets the old marker", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, prepareEmbeddedPassword(dir, "first-password"))
		require.NoError(t, prepareEmbeddedPassword(dir, "postgres")) // still no cluster
		assert.NoFileExists(t, filepath.Join(dir, embeddedPasswordMarker))
	})
}
