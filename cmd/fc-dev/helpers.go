package main

import (
	"os"
	"strconv"
	"strings"
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
