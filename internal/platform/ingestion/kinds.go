package ingestion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
)

// RuntimeLiveWindow is how long after its last poll a runtime counts as live:
// runtimes poll every minute, so three missed polls.
const RuntimeLiveWindow = 3 * time.Minute

// RuntimeKind is a source kind a runtime reports (ing_runtimes.kinds) in its
// poll: one built on an extension, or camel-route. The platform stores and
// lists it and enforces RequiredPermission; it knows nothing else about the
// kind. A runtime may also name built-in kinds (Carries explains what that
// means); those are never listed as extensions.
type RuntimeKind struct {
	// Kind is the kind's name, written as the type of a custom source.
	Kind        string `json:"kind" pattern:"^[a-z0-9][a-z0-9-]*$" maxLength:"50"`
	Description string `json:"description,omitempty" maxLength:"500"`
	// RequiredPermission, when set, is held by whoever creates or changes an
	// ingestion of this kind.
	RequiredPermission string `json:"requiredPermission,omitempty" maxLength:"200"`
	// Settings describes the kind's settings for the form; the platform passes
	// it through untouched.
	Settings json.RawMessage `json:"settings,omitempty"`
}

// RunKind is the kind a runtime must carry to run src: the built-in kind, or
// the type of a custom source.
func RunKind(src Source) string {
	switch c := src.(type) {
	case Custom:
		return c.Type
	case Configured:
		if c.Base == KindCustom {
			return c.Type
		}
	}
	return src.Kind()
}

// BaselineBuiltInKinds are the built-in kinds a runtime carries when it does
// not declare its own (a v2 runtime that predates builtinKinds): every
// built-in kind but custom, which a source is never run as.
func BaselineBuiltInKinds() []string {
	return []string{KindHTTP, KindSFTP, KindS3, KindSQS, KindRabbitMQ, KindDatabasePoll, KindAPIPoll}
}

// CarriedKinds lists the kinds a runtime runs, from what it reported:
// builtin is the built-in kinds it declared (the poll's builtinKinds, nil
// when it sent none) and reported the kinds its extensions add (and
// camel-route when it has it). A runtime that declares builtinKinds carries
// exactly those built-ins (an empty list: none). One that does not carries the
// baseline, unless it names built-in kinds among reported, which narrows it to
// those (the v2 way for a limited runtime to say what it lacks).
func CarriedKinds(reported []RuntimeKind, builtin []string) []string {
	names := make([]string, 0, len(reported)+len(BaselineBuiltInKinds()))
	var named []string
	for _, k := range reported {
		if IsBuiltInKind(k.Kind) {
			named = append(named, k.Kind)
			continue
		}
		if k.Kind != "" && !slices.Contains(names, k.Kind) {
			names = append(names, k.Kind)
		}
	}
	switch {
	case builtin != nil:
		named = builtin
	case len(named) == 0:
		named = BaselineBuiltInKinds()
	}
	for _, k := range named {
		// A source is run by its custom type, never as "custom".
		if k != KindCustom && IsBuiltInKind(k) && !slices.Contains(names, k) {
			names = append(names, k)
		}
	}
	return names
}

// Carries reports whether a runtime that reported these kinds, and declared no
// builtinKinds, can run an ingestion of the kind (RunKind).
func Carries(reported []RuntimeKind, kind string) bool {
	return slices.Contains(CarriedKinds(reported, nil), kind)
}

// decodeCarried reads what one ing_runtimes row says its runtime carries.
func decodeCarried(kinds, builtin json.RawMessage) ([]string, error) {
	var reported []RuntimeKind
	if err := json.Unmarshal(kinds, &reported); err != nil {
		return nil, fmt.Errorf("kinds: %w", err)
	}
	declared, err := decodeBuiltin(builtin)
	if err != nil {
		return nil, err
	}
	return CarriedKinds(reported, declared), nil
}

// decodeBuiltin reads ing_runtimes.builtin_kinds: nil for NULL (the runtime
// declared none), a non-nil list otherwise.
func decodeBuiltin(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	declared := []string{}
	if err := json.Unmarshal(raw, &declared); err != nil {
		return nil, fmt.Errorf("builtin kinds: %w", err)
	}
	return declared, nil
}

// AnyLiveRuntimeCarries reports, for each kind, whether some live runtime
// carries it: false is "no runtime carries this kind", which is not a failure
// of the ingestion, only of there being nobody to run it yet.
func (r *Repository) AnyLiveRuntimeCarries(ctx context.Context) (func(kind string) bool, error) {
	rows, err := r.q.IngestionRuntimeKindsSeenSince(ctx, time.Now().UTC().Add(-RuntimeLiveWindow))
	if err != nil {
		return nil, fmt.Errorf("ingestion repo: runtime kinds: %w", err)
	}
	var carried []string
	for _, row := range rows {
		names, err := decodeCarried(row.Kinds, row.BuiltinKinds)
		if err != nil {
			slog.Warn("ingestion runtime reported kinds that do not parse", "err", err)
			continue
		}
		carried = append(carried, names...)
	}
	return func(kind string) bool { return slices.Contains(carried, kind) }, nil
}

// builtInRequirements are the permissions a kind needs whatever a runtime
// reports, so a runtime that is down, or that reports nothing, cannot make a
// dangerous kind open. camel-route runs a route with the runtime's network,
// identity and environment.
var builtInRequirements = map[string]string{"camel-route": auth.PermIngestionRoute}

// LiveKinds returns the kinds the live runtimes report, one entry per kind
// (the first runtime's description wins; a required permission from any
// runtime is kept). A kind that does not parse is skipped and logged: one
// runtime's bad report must not hide the others.
func (r *Repository) LiveKinds(ctx context.Context) ([]RuntimeKind, error) {
	rows, err := r.q.IngestionRuntimeKindsSeenSince(ctx, time.Now().UTC().Add(-RuntimeLiveWindow))
	if err != nil {
		return nil, fmt.Errorf("ingestion repo: runtime kinds: %w", err)
	}
	var out []RuntimeKind
	index := map[string]int{}
	for _, row := range rows {
		var reported []RuntimeKind
		if err := json.Unmarshal(row.Kinds, &reported); err != nil {
			slog.Warn("ingestion runtime reported kinds that do not parse", "err", err)
			continue
		}
		for _, k := range reported {
			if k.Kind == "" {
				continue
			}
			i, seen := index[k.Kind]
			if !seen {
				index[k.Kind] = len(out)
				out = append(out, k)
				continue
			}
			if out[i].RequiredPermission == "" {
				out[i].RequiredPermission = k.RequiredPermission
			}
		}
	}
	for i := range out {
		if perm, ok := builtInRequirements[out[i].Kind]; ok {
			out[i].RequiredPermission = perm
		}
	}
	return out, nil
}

// RequiredPermission is the permission an ingestion of the custom kind kind
// needs, "" for none: the built-in requirement for the kind, else what a live
// runtime reports, else what the last runtime to report the kind declared
// (ing_kind_requirements), so a kind does not open up while every runtime
// that reports it is down.
func (r *Repository) RequiredPermission(ctx context.Context, kind string) (string, error) {
	if perm, ok := builtInRequirements[kind]; ok {
		return perm, nil
	}
	reported, err := r.LiveKinds(ctx)
	if err != nil {
		return "", err
	}
	for _, k := range reported {
		if k.Kind == kind && k.RequiredPermission != "" {
			return k.RequiredPermission, nil
		}
	}
	stored, err := r.q.IngestionKindRequirementFind(ctx, kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("ingestion repo: kind requirement: %w", err)
	}
	return stored, nil
}

// rememberRequirements records, for each extension kind the runtime reported,
// the permission it declared ("" for none): RequiredPermission falls back to
// it while no runtime that reports the kind is live.
func rememberRequirements(ctx context.Context, q *dbq.Queries, runtimeID string, reported []RuntimeKind, now time.Time) error {
	seen := map[string]bool{}
	for _, k := range reported {
		if k.Kind == "" || IsBuiltInKind(k.Kind) || seen[k.Kind] {
			continue
		}
		seen[k.Kind] = true
		if err := q.IngestionKindRequirementUpsert(ctx, dbq.IngestionKindRequirementUpsertParams{
			Kind: k.Kind, RequiredPermission: k.RequiredPermission, RuntimeID: runtimeID, UpdatedAt: now,
		}); err != nil {
			return fmt.Errorf("kind requirement: %w", err)
		}
	}
	return nil
}
