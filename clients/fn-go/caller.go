package fn

import (
	"context"
	"slices"
	"strings"
)

type ctxKey int

const (
	ctxKeyCaller ctxKey = iota
	ctxKeyInvocation
)

// Principal is a verified caller identity (plan §5.2: caller.kind ==
// "principal"). Its semantics mirror
// internal/platform/shared/auth.AuthContext on the platform side: Tier is
// "ANCHOR", "PARTNER" or "CLIENT"; Permissions entries may carry `*`
// segment wildcards matched by HasPermission.
type Principal struct {
	ID              string
	Type            string
	Tier            string
	Clients         []string
	Roles           []string
	Applications    []string
	AllApplications bool
	Permissions     []string
}

// IsAnchor reports whether the principal has anchor (platform-wide) scope.
func (p *Principal) IsAnchor() bool {
	return p != nil && p.Tier == "ANCHOR"
}

// HasRole reports whether the principal carries the exact role code.
func (p *Principal) HasRole(role string) bool {
	if p == nil {
		return false
	}
	return slices.Contains(p.Roles, role)
}

// CanAccessClient reports whether the principal may act on the given
// client: true for anchor principals, or when clientID is in Clients.
func (p *Principal) CanAccessClient(clientID string) bool {
	if p == nil {
		return false
	}
	if p.IsAnchor() {
		return true
	}
	return slices.Contains(p.Clients, clientID)
}

// CanAccessApplication reports whether the principal may act on the given
// application: true when it holds all-applications access, or when
// applicationID is in its explicit Applications list.
func (p *Principal) CanAccessApplication(applicationID string) bool {
	if p == nil {
		return false
	}
	if p.AllApplications {
		return true
	}
	return slices.Contains(p.Applications, applicationID)
}

// HasPermission reports whether the principal carries a permission
// satisfying code. A held permission may contain `*` segment wildcards
// (e.g. "platform:messaging:*:*" or the super-admin "platform:*:*:*"):
// such a permission matches any code with the same colon-separated segment
// count whose non-wildcard segments are equal. This mirrors
// internal/platform/shared/auth.permissionMatches exactly.
func (p *Principal) HasPermission(code string) bool {
	if p == nil {
		return false
	}
	for _, held := range p.Permissions {
		if permissionMatches(held, code) {
			return true
		}
	}
	return false
}

func permissionMatches(held, required string) bool {
	if held == required {
		return true
	}
	h := strings.Split(held, ":")
	r := strings.Split(required, ":")
	if len(h) != len(r) {
		return false
	}
	for i := range h {
		if h[i] != "*" && h[i] != r[i] {
			return false
		}
	}
	return true
}

// Caller describes who made this invocation (plan §5.2).
type Caller struct {
	kind      string
	principal *Principal
}

// Kind returns "webhook", "anonymous" or "principal".
func (c Caller) Kind() string { return c.kind }

// Principal returns the verified principal, or nil unless Kind() ==
// "principal" (i.e. the endpoint's auth is "platform").
func (c Caller) Principal() *Principal { return c.principal }

// CallerFrom returns the caller for the in-flight invocation. Outside a
// dispatched request (e.g. in a background goroutine that outlived the
// call) it returns the zero Caller ("anonymous"-shaped, with a nil
// Principal).
func CallerFrom(ctx context.Context) Caller {
	if c, ok := ctx.Value(ctxKeyCaller).(Caller); ok {
		return c
	}
	return Caller{kind: "anonymous"}
}
