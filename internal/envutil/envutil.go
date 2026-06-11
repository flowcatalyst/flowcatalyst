// Package envutil holds the shared environment-variable parsing helpers
// the per-package FromEnv constructors use. Each helper preserves the
// exact semantics of the local clone it replaced — defaults on parse
// failure, never on principle. The full variable reference lives in
// docs/environment-variables.md.
package envutil

import (
	"os"
	"strconv"
)

// Or returns the variable's value, or def when unset/empty.
func Or(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

// Int parses the variable as an int, returning def when unset or
// unparseable.
func Int(name string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil {
		return v
	}
	return def
}

// Uint32 parses the variable as a uint32, returning def when unset,
// unparseable, or out of range.
func Uint32(name string, def uint32) uint32 {
	if v, err := strconv.ParseUint(os.Getenv(name), 10, 32); err == nil {
		return uint32(v)
	}
	return def
}

// Uint parses the variable as a uint64 in ok-form: (0, false) when
// unset or unparseable.
func Uint(name string) (uint64, bool) {
	v := os.Getenv(name)
	if v == "" {
		return 0, false
	}
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}
