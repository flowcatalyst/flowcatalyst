// Package ids holds distinct types for the platform's entity identifiers.
//
// A ClientID and a PrincipalID are both strings on the wire and in the
// database, which is exactly why they are easy to swap: a function taking
// (principalID, clientID string) accepts its arguments in either order. Giving
// each identifier its own type turns that mistake into a compile error
// wherever the values travel as typed variables. JSON, pgx and huma treat the
// types as the strings they are, so nothing changes on the wire or in SQL.
//
// The types do not validate on conversion: ClientID("x") compiles, and rows
// already in the database are not guaranteed to carry the current prefix.
// Parse* is the checking constructor for input that arrives from outside the
// process.
package ids

import (
	"fmt"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// ClientID identifies a tenant client (prefix "clt_"). It is NOT an OAuth
// client_id; see OAuthClientID.
type ClientID string

// PrincipalID identifies a principal — a user or service account ("prn_").
type PrincipalID string

// ApplicationID identifies an application ("app_").
type ApplicationID string

// OAuthClientID is the client_id of an OAuth client. Unlike the entity IDs
// above it is not necessarily a generated TSID: an OAuth client's identifier
// is chosen by whoever registers it, so it carries no prefix guarantee and
// has no Parse.
type OAuthClientID string

func (id ClientID) String() string      { return string(id) }
func (id PrincipalID) String() string   { return string(id) }
func (id ApplicationID) String() string { return string(id) }
func (id OAuthClientID) String() string { return string(id) }

// ParseClientID checks s has the client prefix and returns it as a ClientID.
func ParseClientID(s string) (ClientID, error) {
	if err := checkPrefix(s, tsid.Client); err != nil {
		return "", err
	}
	return ClientID(s), nil
}

// ParsePrincipalID checks s has the principal prefix.
func ParsePrincipalID(s string) (PrincipalID, error) {
	if err := checkPrefix(s, tsid.Principal); err != nil {
		return "", err
	}
	return PrincipalID(s), nil
}

// ParseApplicationID checks s has the application prefix.
func ParseApplicationID(s string) (ApplicationID, error) {
	if err := checkPrefix(s, tsid.Application); err != nil {
		return "", err
	}
	return ApplicationID(s), nil
}

func checkPrefix(s string, e tsid.EntityType) error {
	prefix := e.Prefix() + "_"
	if len(s) <= len(prefix) || !strings.HasPrefix(s, prefix) {
		return fmt.Errorf("invalid id %q: want %q prefix followed by a value", s, prefix)
	}
	return nil
}

// Strings converts a typed ID slice to the plain strings the wire and SQL
// arguments use.
func Strings[T ~string](in []T) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = string(v)
	}
	return out
}
