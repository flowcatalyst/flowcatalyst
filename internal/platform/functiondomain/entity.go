// Package functiondomain is the function-runner public-routes domain-claim
// aggregate (docs/function-runner-plan.md §8, "public routes"): a claimed
// hostname ZONE, owned by a client (or the platform, when nil), that a
// function's route hostname must be covered by. A claim is verified by
// being made — no DNS step (owner ruling) — and covers the zone itself and
// every hostname under it.
package functiondomain

import (
	"regexp"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// ZonePattern is the hostname-zone shape: one or more dot-separated DNS
// labels (lowercase letters/digits/hyphens, 1-63 chars each, no leading or
// trailing hyphen), matching what a route's own Hostname must also satisfy
// (function.HostnamePattern) — a zone is just a hostname claimed as a
// prefix boundary, not a different shape. No port, no wildcard: the
// character class excludes both.
var ZonePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*$`)

// MaxZoneLength is the DNS name length ceiling (RFC 1035).
const MaxZoneLength = 255

// ValidZone reports whether zone is a lowercase DNS name with no port and
// no wildcard, at most MaxZoneLength bytes.
func ValidZone(zone string) bool {
	return zone != "" && len(zone) <= MaxZoneLength && ZonePattern.MatchString(zone)
}

// Covers reports whether zone covers hostname: the zone itself, or any
// hostname under it (plan: "covers the zone itself and every hostname under
// it"). Both arguments are expected already-lowercased, trailing-dot-free
// hostnames (see function.NormalizeHostname) — Covers itself does no
// normalisation so callers control exactly what comparison is made.
func Covers(zone, hostname string) bool {
	if zone == hostname {
		return true
	}
	return strings.HasSuffix(hostname, "."+zone)
}

// SameOwner reports whether two client-scoping pointers name the same
// owner: both nil (platform), or both non-nil and equal.
func SameOwner(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// FunctionDomain is the aggregate root. Table: fng_domains.
type FunctionDomain struct {
	ID        string
	Zone      string
	ClientID  *string
	CreatedBy *string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// IDStr satisfies usecase.HasID.
func (d FunctionDomain) IDStr() string { return d.ID }

// New constructs a FunctionDomain claim.
func New(zone string, clientID *string) *FunctionDomain {
	now := time.Now().UTC()
	return &FunctionDomain{
		ID:        tsid.Generate(tsid.FunctionDomain),
		Zone:      zone,
		ClientID:  clientID,
		CreatedAt: now,
		UpdatedAt: now,
	}
}
