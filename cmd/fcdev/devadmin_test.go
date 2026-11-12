package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/seed"
)

func TestDecideSeedDevAdmin(t *testing.T) {
	cases := []struct {
		name      string
		embedded  bool
		flagSet   bool
		flagValue bool
		env       string
		want      bool
	}{
		{"embedded, nothing set: seeded (unchanged)", true, false, false, "", true},
		{"external, nothing set: not seeded", false, false, false, "", false},
		{"external, env true: seeded", false, false, false, "true", true},
		{"external, env 1: seeded", false, false, false, "1", true},
		{"external, env yes: seeded", false, false, false, "yes", true},
		{"external, flag true: seeded", false, true, true, "", true},
		{"external, flag true beats env false", false, true, true, "false", true},
		{"external, flag false beats env true", false, true, false, "true", false},
		{"embedded, flag false: not seeded", true, true, false, "", false},
		{"embedded, env false: not seeded", true, false, false, "false", false},
		{"embedded, env off: not seeded", true, false, false, "off", false},
		{"embedded, garbage env: default", true, false, false, "maybe", true},
		{"external, garbage env: default", false, false, false, "maybe", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, decideSeedDevAdmin(tc.embedded, tc.flagSet, tc.flagValue, tc.env))
		})
	}
}

func clearBootstrapEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{seed.EnvBootstrapEmail, seed.EnvBootstrapPassword, seed.EnvBootstrapName} {
		t.Setenv(k, "")
	}
}

// What the decision does to the seeder's environment: the dev administrator is
// set up only when chosen, and a developer's own bootstrap variables are never
// overwritten or dropped.
func TestApplyDevAdminDefaults(t *testing.T) {
	t.Run("seeded: the dev administrator", func(t *testing.T) {
		clearBootstrapEnv(t)
		applyDevAdminDefaults(true)
		assert.Equal(t, "admin@flowcatalyst.local", os.Getenv(seed.EnvBootstrapEmail))
		assert.Equal(t, "DevPassword123!", os.Getenv(seed.EnvBootstrapPassword))
	})
	t.Run("not seeded: nothing is set, so the seeder creates no administrator", func(t *testing.T) {
		clearBootstrapEnv(t)
		applyDevAdminDefaults(false)
		assert.Empty(t, os.Getenv(seed.EnvBootstrapEmail))
		assert.Empty(t, os.Getenv(seed.EnvBootstrapPassword))
	})
	t.Run("not seeded but the developer set the bootstrap variables: honoured", func(t *testing.T) {
		clearBootstrapEnv(t)
		t.Setenv(seed.EnvBootstrapEmail, "me@example.com")
		t.Setenv(seed.EnvBootstrapPassword, "my-own-password-1")
		applyDevAdminDefaults(false)
		assert.Equal(t, "me@example.com", os.Getenv(seed.EnvBootstrapEmail))
		assert.Equal(t, "my-own-password-1", os.Getenv(seed.EnvBootstrapPassword))
	})
	t.Run("seeded with the developer's variables: theirs win", func(t *testing.T) {
		clearBootstrapEnv(t)
		t.Setenv(seed.EnvBootstrapEmail, "me@example.com")
		t.Setenv(seed.EnvBootstrapPassword, "my-own-password-1")
		applyDevAdminDefaults(true)
		assert.Equal(t, "me@example.com", os.Getenv(seed.EnvBootstrapEmail))
		assert.Equal(t, "my-own-password-1", os.Getenv(seed.EnvBootstrapPassword))
	})
}

func TestStartResolvesSeedDevAdminFromDatabaseURL(t *testing.T) {
	t.Setenv(devSeedAdminEnv, "")
	cmd := newStartCmd()
	require.NoError(t, cmd.Flags().Parse(nil))
	assert.True(t, optsFromFlags(cmd).SeedDevAdmin, "embedded")

	cmd = newStartCmd()
	require.NoError(t, cmd.Flags().Parse([]string{"--database-url", "postgres://x/y"}))
	assert.False(t, optsFromFlags(cmd).SeedDevAdmin, "external")

	cmd = newStartCmd()
	require.NoError(t, cmd.Flags().Parse([]string{"--database-url", "postgres://x/y", "--seed-dev-admin"}))
	assert.True(t, optsFromFlags(cmd).SeedDevAdmin, "external, asked for")
}
