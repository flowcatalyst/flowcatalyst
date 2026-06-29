package main

import (
	"bufio"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

func envStrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// envFirst returns the value of the first set (non-empty) env var in keys,
// falling back to def. Used where a setting has several accepted env names
// (e.g. the Rust/SDK aliases for the outbox DB type and URL).
func envFirst(def string, keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return def
}

func envIntDefault(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// setEnvDefault sets an env var only when it isn't already populated.
// Used to seed dev defaults (bootstrap admin email/password) without
// trampling an explicit operator override.
func setEnvDefault(key, value string) {
	if os.Getenv(key) == "" {
		_ = os.Setenv(key, value)
	}
}

func envBoolDefault(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}

// loadDotEnv reads KEY=VALUE pairs from a dotenv file into the process
// environment WITHOUT overriding variables already set (an explicit env var
// always wins). A missing file is a no-op — it's a local-dev convenience.
//
// Supports `#` comments, blank lines, an optional leading `export `, and
// single/double-quoted values.
func loadDotEnv(path string) {
	if path == "" {
		return
	}
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")

		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		if n := len(val); n >= 2 {
			if (val[0] == '"' && val[n-1] == '"') || (val[0] == '\'' && val[n-1] == '\'') {
				val = val[1 : n-1]
			}
		}
		if key == "" {
			continue
		}
		if _, ok := os.LookupEnv(key); ok {
			continue // never override an explicit env var
		}
		_ = os.Setenv(key, val)
	}
}

// resolveEnvFlag returns the value the user explicitly passed via the flag,
// else the env var (which may have just been loaded from a dotenv file), else
// the flag's baked default. Needed because flag defaults are computed when the
// command tree is built — before loadDotEnv runs.
func resolveEnvFlag(cmd *cobra.Command, flag, env string) string {
	if cmd.Flags().Changed(flag) {
		v, _ := cmd.Flags().GetString(flag)
		return v
	}
	if v := os.Getenv(env); v != "" {
		return v
	}
	v, _ := cmd.Flags().GetString(flag)
	return v
}

// resolveEnvFlagMulti is resolveEnvFlag with several accepted env names tried
// in order (e.g. a poller-specific name falling back to the app's shared
// FLOWCATALYST_* var). An explicitly-passed flag still wins.
func resolveEnvFlagMulti(cmd *cobra.Command, flag string, envs ...string) string {
	if cmd.Flags().Changed(flag) {
		v, _ := cmd.Flags().GetString(flag)
		return v
	}
	for _, env := range envs {
		if v := os.Getenv(env); v != "" {
			return v
		}
	}
	v, _ := cmd.Flags().GetString(flag)
	return v
}

// resolveEnvFlagInt is the int counterpart of resolveEnvFlag.
func resolveEnvFlagInt(cmd *cobra.Command, flag, env string) int {
	if cmd.Flags().Changed(flag) {
		v, _ := cmd.Flags().GetInt(flag)
		return v
	}
	if s := os.Getenv(env); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			return n
		}
	}
	v, _ := cmd.Flags().GetInt(flag)
	return v
}
