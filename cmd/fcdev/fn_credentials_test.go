package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFnCLICredentialsPathOverride(t *testing.T) {
	want := filepath.Join(t.TempDir(), "sub", "cred.json")
	t.Setenv(fnCLICredentialsEnv, want)
	got, err := fnCLICredentialsPath()
	require.NoError(t, err)
	require.Equal(t, want, got)

	require.NoError(t, writeFnCLICredentials(fnCredentials{PlatformURL: "http://x", RunnerURL: "http://r", ClientID: "id", ClientSecret: "s"}))
	_, err = os.Stat(want)
	require.NoError(t, err)

	t.Setenv("FC_PLATFORM_URL", "")
	t.Setenv("FC_RUNNER_URL", "")
	t.Setenv("FC_CLIENT_ID", "")
	t.Setenv("FC_CLIENT_SECRET", "")
	c, err := (&fnPlatformFlags{}).resolve()
	require.NoError(t, err)
	require.Equal(t, "id", c.ClientID)
	require.Equal(t, "http://x", c.PlatformURL)
}

func TestFnCLICredentialsPathDefault(t *testing.T) {
	t.Setenv(fnCLICredentialsEnv, "")
	got, err := fnCLICredentialsPath()
	require.NoError(t, err)
	require.Equal(t, "fng-cli.json", filepath.Base(got))
}
