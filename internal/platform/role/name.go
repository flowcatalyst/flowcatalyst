package role

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// Role-name and permission shape (docs/platform-bug-fixes.md F18).
//
// A role name is the canonical "{applicationCode}:{roleName}" (see New): two or
// more colon-separated, non-empty segments. A multi-colon name such as
// "logistics_portal:dashboard:user" is tolerated (ShortName round-trips it) so
// existing definitions stay valid. Each segment is ASCII letters, digits and
// "_", "." and "-": no whitespace of any kind (the ASCII or the Unicode kind),
// no control or zero-width characters, nothing that can render as a
// look-alike of a real name. Case is not constrained here (the SDK sync
// lowercases, the admin create does not, and both exist in stored data); the
// shape rule only has to make a name unambiguous.
//
// A value that fails is REJECTED, never trimmed: silently repairing
// "example: dispatcher" would hide the caller's bug, and a trimmed name would
// no longer be the string the caller sent back in the audit trail.

// InvalidNameCode is the 400 code for a role name of the wrong shape.
const InvalidNameCode = "INVALID_ROLE_NAME"

// InvalidPermissionCode is the 400 code for a permission string of the wrong
// shape.
const InvalidPermissionCode = "INVALID_PERMISSION"

// ValidateName reports whether name is a well-formed canonical role name.
// The error is a 400 usecase validation, INVALID_ROLE_NAME, quoting the value.
func ValidateName(name string) error {
	if !validSegments(name, 2, false) {
		return usecase.Validation(InvalidNameCode, fmt.Sprintf(
			"Invalid role name %q: expected '{application}:{role}' with letters, digits, '_', '.' and '-' only, and no spaces",
			name))
	}
	return nil
}

// ValidateNames validates every name, returning the first offender's error.
func ValidateNames(names []string) error {
	for _, n := range names {
		if err := ValidateName(n); err != nil {
			return err
		}
	}
	return nil
}

// ValidatePermission reports whether p is a well-formed permission string:
// one or more colon-separated, non-empty segments of letters, digits, "_",
// ".", "-" or the "*" wildcard, with no whitespace. 400 INVALID_PERMISSION.
func ValidatePermission(p string) error {
	if !validSegments(p, 1, true) {
		return usecase.Validation(InvalidPermissionCode, fmt.Sprintf(
			"Invalid permission %q: colon-separated segments of letters, digits, '_', '.', '-' or '*', and no spaces", p))
	}
	return nil
}

// ValidatePermissions validates every permission, returning the first
// offender's error.
func ValidatePermissions(perms []string) error {
	for _, p := range perms {
		if err := ValidatePermission(p); err != nil {
			return err
		}
	}
	return nil
}

func validSegments(s string, minSegments int, allowStar bool) bool {
	segs := strings.Split(s, ":")
	if len(segs) < minSegments {
		return false
	}
	for _, seg := range segs {
		if seg == "" {
			return false
		}
		for i := 0; i < len(seg); i++ {
			c := seg[i]
			switch {
			case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
				c == '_', c == '.', c == '-':
			case c == '*' && allowStar:
			default:
				return false // includes every non-ASCII byte
			}
		}
	}
	return true
}

// Name is a role name that has been through the write boundary: a
// RoleAssignment holds one, so an assignment cannot be built from a raw string
// (docs/token-claims-followups.md §4, part 2). The ways to get one:
//
//   - [ResolveName]: shape-checked AND looked up, the result being the stored
//     role's own canonical name. Every assignment write path uses this.
//   - [ParseName]: shape-checked only, for paths that deliberately accept a
//     name no role bears yet.
//   - [Role.CanonicalName]: from a role already in hand.
//   - [StoredName]: persistence reads only; it validates nothing.
//
// A type holds against code, not against psql; that trade is accepted.
type Name struct{ v string }

// String returns the name; "" for the zero Name.
func (n Name) String() string { return n.v }

// IsZero reports whether n is the zero Name.
func (n Name) IsZero() bool { return n.v == "" }

// MarshalJSON encodes the name as a JSON string.
func (n Name) MarshalJSON() ([]byte, error) { return json.Marshal(n.v) }

// UnmarshalJSON reads a stored name without validating it (it reads what was
// persisted; the write boundary is ParseName/ResolveName).
func (n *Name) UnmarshalJSON(b []byte) error { return json.Unmarshal(b, &n.v) }

// Scan implements sql.Scanner for reading iam_principal_roles.role_name. Like
// StoredName it validates nothing: it reads what is persisted.
func (n *Name) Scan(src any) error {
	switch v := src.(type) {
	case string:
		n.v = v
	case []byte:
		n.v = string(v)
	case nil:
		n.v = ""
	default:
		return fmt.Errorf("role.Name: cannot scan %T", src)
	}
	return nil
}

// Value implements driver.Valuer so a Name can be written as a query argument.
func (n Name) Value() (driver.Value, error) { return n.v, nil }

// ParseName shape-checks s (ValidateName) and returns it as a Name.
func ParseName(s string) (Name, error) {
	if err := ValidateName(s); err != nil {
		return Name{}, err
	}
	return Name{v: s}, nil
}

// StoredName wraps a name read back from storage. Persistence adapters only:
// it validates nothing, so a row written outside the domain stays visible
// rather than failing every read of the principal.
func StoredName(s string) Name { return Name{v: s} }

// CanonicalName is the role's own name as a Name.
func (r Role) CanonicalName() Name { return Name{v: r.Name} }

// ResolveName validates s and resolves it against the role definitions: 400
// INVALID_ROLE_NAME for a malformed value, 400 ROLE_NOT_FOUND for a well-formed
// name no role bears. The Name returned is the stored role's canonical name.
func ResolveName(ctx context.Context, repo *Repository, s string) (Name, error) {
	if err := ValidateName(s); err != nil {
		return Name{}, err
	}
	r, err := repo.FindByName(ctx, s)
	if err != nil {
		return Name{}, usecase.Internal("REPO", "validate role failed", err)
	}
	if r == nil {
		return Name{}, usecase.Validation("ROLE_NOT_FOUND", "Role not found: "+s)
	}
	return r.CanonicalName(), nil
}

// ResolveNames resolves every name (see ResolveName), in order.
func ResolveNames(ctx context.Context, repo *Repository, names []string) ([]Name, error) {
	out := make([]Name, 0, len(names))
	for _, s := range names {
		n, err := ResolveName(ctx, repo, s)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

// MustParseName is ParseName for a compile-time constant name owned by the
// platform itself (a seeded role); it panics on a malformed literal.
func MustParseName(s string) Name {
	n, err := ParseName(s)
	if err != nil {
		panic(err)
	}
	return n
}
