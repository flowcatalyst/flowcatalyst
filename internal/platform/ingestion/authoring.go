package ingestion

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// Change is a definition about to be saved (created, saved as a version,
// applied by a set, or tried by a test), with what it replaces.
type Change struct {
	// Current is the ingestion's stored definition, nil for a create. A kind
	// that declares a permission needs it to change an ingestion of that kind
	// as well as to make one.
	Current *Definition
	// Live is the live version's definition, nil while nothing is live (and
	// for a create). What the definition points secrets at, and whether it
	// newly uses the runtime's own identity, is judged against it: that is
	// what runtimes run.
	Live *Definition
	// Next is the definition being saved.
	Next Definition
	// Supplied are the names of the secrets whose values the author gives in
	// this same write (the settings form, a config write with secrets). An
	// author who provides a secret's value is not reading one they cannot
	// see, so a name in Supplied never counts as steering a secret
	// (SteersSecretsExcept).
	Supplied []string
}

// CheckAuthor refuses a caller who may not save the change, beyond the coarse
// create or update permission (docs/ingestion-design.md section 4):
//
//   - a kind's declared permission (camel-route's platform:ingestion:ingestion:route)
//     to make or change an ingestion of the kind; route counts only at anchor
//     tier (owner ruling R2), since a route runs with the runtime's network,
//     identity and environment;
//   - platform:ingestion:ingestion:runtime-identity for an SQS or S3 source
//     without a credential (the runtime fleet reads it with its own AWS
//     identity) that is new or differs from the live one;
//   - platform:ingestion:ingestion:bind-secrets for a definition that names
//     secrets and is new, or differs from the live one in where it sends the
//     runtime (a host, a URL, a JDBC URL) or in which secrets it names: the
//     runtime presents a named secret to whatever host the definition names,
//     so naming one is reading it. For an extension kind every text setting
//     counts as a secret name, so any change of its settings counts.
//
// A secret whose value the same write supplies (Change.Supplied) is the
// author's own: it does not need bind-secrets, whether its name is new or its
// address changed. Everything else the definition names still does.
func (r *Repository) CheckAuthor(ctx context.Context, ac *auth.AuthContext, c Change) error {
	sources := []Source{c.Next.Source}
	if c.Current != nil {
		sources = append([]Source{c.Current.Source}, sources...)
	}
	for _, src := range sources {
		kind := RunKind(src)
		if kind == src.Kind() {
			continue // a built-in kind declares no permission
		}
		perm, err := r.RequiredPermission(ctx, kind)
		if err != nil {
			return usecase.Internal("REPO", "required permission lookup failed", err)
		}
		if err := RequireKindPermission(ac, kind, perm); err != nil {
			return err
		}
	}
	if UsesRuntimeIdentity(c.Next.Source) && (c.Live == nil || !sameSource(c.Live.Source, c.Next.Source)) {
		if !ac.HasPermission(auth.PermIngestionRuntimeIdentity) {
			return usecase.Authorization("INGESTION_RUNTIME_IDENTITY_PERMISSION_REQUIRED",
				"an "+c.Next.Source.Kind()+" source without a credential is read with the runtime fleet's own AWS identity, which needs "+
					auth.PermIngestionRuntimeIdentity+"; name an access key in source.credential instead")
		}
	}
	// Accepting any sftp host key sends the credential to whoever answers at
	// the address: as much a widening of where a secret goes as a new host.
	if acceptsAnyHostKey(c.Next.Source) && (c.Live == nil || !acceptsAnyHostKey(c.Live.Source)) {
		if !ac.HasPermission(auth.PermIngestionBindSecrets) {
			return usecase.Authorization("INGESTION_BIND_SECRETS_PERMISSION_REQUIRED",
				"accepting any sftp host key sends the credential to whoever answers at the address, which needs "+
					auth.PermIngestionBindSecrets+"; give the server's known_hosts line instead")
		}
	}
	if unsupplied := withoutNames(SecretNames(c.Next.Source), c.Supplied); len(unsupplied) > 0 && (c.Live == nil || SteersSecretsExcept(*c.Live, c.Next, c.Supplied)) {
		if !ac.HasPermission(auth.PermIngestionBindSecrets) {
			return usecase.Authorization("INGESTION_BIND_SECRETS_PERMISSION_REQUIRED",
				"the definition names secrets and points them at a new address or names other secrets than the live version, which needs "+
					auth.PermIngestionBindSecrets)
		}
	}
	return nil
}

// RequireKindPermission refuses a caller who lacks perm, the permission the
// source kind kind declares ("" for none).
//
// The route permission (camel-route's) counts only for an anchor-tier caller
// (ruling R2), and only for a pipeline or an ingestion administrator (owner
// ruling 2026-10-25): a route is written by a pipeline's service account
// holding the route-author role, or by an ingestion administrator (through
// fcdev ingest apply, say), never by a person who holds only the route
// permission. The role assignment has no hook to keep the route-author role
// off users, so the rule is enforced here, at use: writing a route needs
// anchor tier and the route permission and either a SERVICE principal or
// platform:ingestion:ingestion:delete, which only the administrator role
// holds among the ingestion roles.
func RequireKindPermission(ac *auth.AuthContext, kind, perm string) error {
	if perm == "" {
		return nil
	}
	if !ac.HasPermission(perm) {
		return usecase.Authorization("INGESTION_KIND_PERMISSION_REQUIRED",
			"permission required for source kind "+kind+": "+perm)
	}
	if perm != auth.PermIngestionRoute {
		return nil
	}
	if !ac.IsAnchor() {
		return usecase.Authorization("INGESTION_ROUTE_ANCHOR_REQUIRED",
			"a "+kind+" ingestion runs a route with the runtime's network and identity, so only an anchor-tier caller may write one, "+perm+" or not")
	}
	if ac.PrincipalType != auth.PrincipalTypeService && auth.CanDeleteIngestions(ac) != nil {
		return usecase.Authorization("INGESTION_ROUTE_PIPELINE_REQUIRED",
			"a "+kind+" ingestion is written by a pipeline's service account that holds "+perm+
				", or by an ingestion administrator; a person who holds only "+perm+" may not write one")
	}
	return nil
}

// HoldsKindPermission reports whether the caller may write an ingestion of a
// kind that declares perm (RequireKindPermission).
func HoldsKindPermission(ac *auth.AuthContext, perm string) bool {
	return RequireKindPermission(ac, "", perm) == nil
}

func acceptsAnyHostKey(src Source) bool {
	s, ok := src.(Sftp)
	return ok && s.AcceptAnyHostKey
}

// UsesRuntimeIdentity reports whether src is read with the runtime's own
// cloud identity: an SQS or S3 source without a credential.
func UsesRuntimeIdentity(src Source) bool {
	switch s := src.(type) {
	case Sqs:
		return s.Credential == nil
	case S3:
		return s.Credential == nil
	}
	return false
}

// SteersSecrets reports whether next, which names secrets, sends them
// somewhere other than live does: another kind, another outbound address,
// another set of secret names, or (an extension kind, whose settings all
// count as secret names) other settings.
func SteersSecrets(live, next Definition) bool { return SteersSecretsExcept(live, next, nil) }

// SteersSecretsExcept is SteersSecrets for a write that supplies the values
// of the secrets in supplied: those are the author's own, so they are left out
// of the comparison. What remains named and unsupplied is steered when it is
// sent to another address or kind, or the unsupplied names differ from live's.
// A write that supplies every secret it names steers none.
func SteersSecretsExcept(live, next Definition, supplied []string) bool {
	nextNames := withoutNames(SecretNames(next.Source), supplied)
	if len(nextNames) == 0 {
		return false
	}
	if RunKind(live.Source) != RunKind(next.Source) {
		return true
	}
	if lc, ok := live.Source.(Custom); ok {
		nc, _ := next.Source.(Custom)
		// A setting whose new value is a supplied secret is the author's own, so
		// it is left out of both sides, whatever it held before.
		own := func(k string) bool {
			text, ok := nc.Settings[k].(string)
			return ok && slices.Contains(supplied, text)
		}
		a, errA := json.Marshal(settingsWithout(lc.Settings, supplied, own))
		b, errB := json.Marshal(settingsWithout(nc.Settings, supplied, own))
		return errA != nil || errB != nil || string(a) != string(b)
	}
	if !slices.Equal(addressValues(live.Source), addressValues(next.Source)) {
		return true
	}
	a, b := withoutNames(SecretNames(live.Source), supplied), nextNames
	slices.Sort(a)
	slices.Sort(b)
	return !slices.Equal(slices.Compact(a), slices.Compact(b))
}

// withoutNames is names without those in except.
func withoutNames(names, except []string) []string {
	return slices.DeleteFunc(slices.Clone(names), func(n string) bool { return slices.Contains(except, n) })
}

// settingsWithout is an extension's settings without the secrets it names that
// the write supplies, so that giving a secret does not read as changing the
// other settings.
func settingsWithout(settings map[string]any, supplied []string, own func(key string) bool) map[string]any {
	out := make(map[string]any, len(settings))
	for k, v := range settings {
		if own(k) {
			continue
		}
		switch x := v.(type) {
		case string:
			if slices.Contains(supplied, x) {
				continue
			}
		case []any:
			x = slices.DeleteFunc(slices.Clone(x), func(item any) bool {
				text, ok := item.(string)
				return ok && slices.Contains(supplied, text)
			})
			v = x
		}
		out[k] = v
	}
	return out
}

// addressValues are src's outbound addresses as written, field by field.
func addressValues(src Source) []string {
	var out []string
	for _, a := range sourceAddresses(src) {
		out = append(out, a.field+"="+a.value)
	}
	return out
}

func sameSource(a, b Source) bool {
	x, errX := json.Marshal(a)
	y, errY := json.Marshal(b)
	return errX == nil && errY == nil && string(x) == string(y)
}
