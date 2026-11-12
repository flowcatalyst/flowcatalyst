package secretref

import (
	"log/slog"
	"os"
	"strings"
	"time"
)

// Defaults of the resolver's cache (docs/external-secret-references.md §4).
const (
	DefaultTTL         = 5 * time.Minute
	DefaultMaxStale    = time.Hour
	DefaultNegativeTTL = 30 * time.Second
)

// Config is the resolver's settings.
type Config struct {
	// Allow is the prefixes of whole references a reference must start with
	// (FC_SECRET_REFERENCE_ALLOW). An entry may hold {app} and {client}: it
	// then admits only the owning row's own application/client at those
	// path segments (see Owner, matchEntry). Empty turns the feature off.
	Allow []string
	// TTL is how long a resolved value is served without asking AWS again.
	TTL time.Duration
	// MaxStale is how long past its TTL a value is still served while AWS
	// cannot be reached.
	MaxStale time.Duration
	// NegativeTTL is how long a failure is remembered, so a loop that asks
	// every few seconds does not ask AWS every few seconds.
	NegativeTTL time.Duration
}

// ConfigFromEnv reads FC_SECRET_REFERENCE_ALLOW, FC_SECRET_REFERENCE_TTL and
// FC_SECRET_REFERENCE_MAX_STALE (Go durations, e.g. "5m"; an unparseable or
// non-positive one is logged and the default used).
func ConfigFromEnv() Config {
	allow := splitAllow(os.Getenv("FC_SECRET_REFERENCE_ALLOW"))
	WarnLooseEntries(allow)
	return Config{
		Allow:       allow,
		TTL:         durationEnv("FC_SECRET_REFERENCE_TTL", DefaultTTL),
		MaxStale:    durationEnv("FC_SECRET_REFERENCE_MAX_STALE", DefaultMaxStale),
		NegativeTTL: DefaultNegativeTTL,
	}
}

func splitAllow(v string) []string {
	var out []string
	for p := range strings.SplitSeq(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func durationEnv(name string, def time.Duration) time.Duration {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		slog.Warn("secret reference: ignoring an invalid duration", "name", name, "value", v, "default", def.String())
		return def
	}
	return d
}

// withDefaults fills the zero durations.
func (c Config) withDefaults() Config {
	if c.TTL <= 0 {
		c.TTL = DefaultTTL
	}
	if c.MaxStale <= 0 {
		c.MaxStale = DefaultMaxStale
	}
	if c.NegativeTTL <= 0 {
		c.NegativeTTL = DefaultNegativeTTL
	}
	return c
}
