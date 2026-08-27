package auth

import "strings"

// AllApplicationsSentinel is the `applications` claim entry meaning "every
// application, present and future" — the same wildcard the `clients` claim
// uses for an anchor principal. Kept in sync with the mint side
// (authservice.appAccessOf).
const AllApplicationsSentinel = "*"

// ParseApplicationsClaim turns the wire form of the `applications` claim into
// the internal model: bare application ids plus an all-applications flag.
//
// The claim carries "id:code" pairs (mirroring `clients`), or the single "*"
// sentinel. Everything inside the platform reasons in bare ids — notably
// AuthContext.CanAccessApplication — so the pair form is split exactly once,
// here at the boundary where a token becomes an AuthContext.
//
// Tolerates the older bare-id form, so tokens minted before the pair shape
// existed keep working for their remaining TTL. An entry is split on its FIRST
// colon only, so an application code containing one survives intact in the
// discarded half.
func ParseApplicationsClaim(entries []string) (ids []string, allApplications bool) {
	if len(entries) == 0 {
		return nil, false
	}
	ids = make([]string, 0, len(entries))
	for _, e := range entries {
		if e == AllApplicationsSentinel {
			allApplications = true
			continue
		}
		if i := strings.Index(e, ":"); i > 0 {
			e = e[:i]
		}
		if e != "" {
			ids = append(ids, e)
		}
	}
	return ids, allApplications
}
