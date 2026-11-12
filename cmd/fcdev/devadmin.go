package main

import (
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/seed"
)

// devSeedAdminEnv is the environment form of --seed-dev-admin.
const devSeedAdminEnv = "FC_DEV_SEED_ADMIN"

// Built-in development administrator. It is public (it is in the docs), so it
// is only ever created where the database is fcdev's own throwaway embedded
// one, or where the developer explicitly asks for it.
const (
	devAdminEmail    = "admin@flowcatalyst.local"
	devAdminPassword = "DevPassword123!"
	devAdminName     = "Local Admin"
)

func addSeedDevAdminFlag(cmd *cobra.Command) {
	cmd.Flags().Bool("seed-dev-admin", false, "create the built-in dev administrator ("+devAdminEmail+") when the database has no admin; default true for the embedded database, false with --database-url (env "+devSeedAdminEnv+")")
}

// decideSeedDevAdmin says whether the built-in dev administrator is created.
// An explicit setting (the flag, then the environment variable) always wins;
// otherwise it is created on the embedded database and not on an external one.
// Explicit FLOWCATALYST_BOOTSTRAP_ADMIN_* variables are separate: they are
// honoured on any database whatever this returns.
func decideSeedDevAdmin(embedded, flagSet, flagValue bool, envValue string) bool {
	if flagSet {
		return flagValue
	}
	if v, err := strconv.ParseBool(strings.TrimSpace(envValue)); err == nil && strings.TrimSpace(envValue) != "" {
		return v
	}
	switch strings.ToLower(strings.TrimSpace(envValue)) {
	case "yes", "on":
		return true
	case "no", "off":
		return false
	}
	return embedded
}

func resolveSeedDevAdmin(cmd *cobra.Command, embedded bool) bool {
	v, _ := cmd.Flags().GetBool("seed-dev-admin")
	return decideSeedDevAdmin(embedded, cmd.Flags().Changed("seed-dev-admin"), v, os.Getenv(devSeedAdminEnv))
}

// applyDevAdminDefaults pre-sets the bootstrap-admin variables so the seeder
// creates the dev administrator. When seedDev is false nothing is set: the
// seeder then creates an administrator only if the developer supplied
// FLOWCATALYST_BOOTSTRAP_ADMIN_EMAIL / _PASSWORD themselves.
func applyDevAdminDefaults(seedDev bool) {
	if seedDev {
		setEnvDefault(seed.EnvBootstrapEmail, devAdminEmail)
		setEnvDefault(seed.EnvBootstrapPassword, devAdminPassword)
		setEnvDefault(seed.EnvBootstrapName, devAdminName)
		return
	}
	if os.Getenv(seed.EnvBootstrapEmail) != "" && os.Getenv(seed.EnvBootstrapPassword) != "" {
		slog.Info("built-in dev administrator not seeded; using the bootstrap administrator from the environment",
			"email", os.Getenv(seed.EnvBootstrapEmail))
		return
	}
	slog.Info("built-in dev administrator not seeded (external database, or turned off): pass --seed-dev-admin (or " + devSeedAdminEnv +
		"=true) to create " + devAdminEmail + ", or set " + seed.EnvBootstrapEmail + " and " + seed.EnvBootstrapPassword)
}
