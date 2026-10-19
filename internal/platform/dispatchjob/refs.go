package dispatchjob

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/repocommon"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// Limits on the ids a name resolves to. Past them the filter is refused (400)
// rather than truncated: a silently truncated id list would list, count or
// requeue a subset of what the caller named.
const (
	// MaxResolvedSubscriptions caps the subscription ids one name, code or
	// connection resolves to.
	MaxResolvedSubscriptions = 200
	// MaxResolvedConnections caps the connection ids one connection code
	// resolves to (a code is unique per client, so this is "one per client").
	MaxResolvedConnections = 50

	// TooManyMatchesCode is the 400 code for a name that resolves to more than
	// the cap.
	TooManyMatchesCode = "TOO_MANY_MATCHES"
)

// resolvedRefs is the outcome of ResolveRefs: the ids the by-name and by-connection
// references stand for.
type resolvedRefs struct {
	// subs constrains subscription_id when a subscription code/name was given
	// (nil: no such reference; empty: matched nothing).
	subs []string
	hasS bool
	// connection constraint: connection_id = ANY(connIDs) OR subscription_id =
	// ANY(connSubs).
	connIDs  []string
	connSubs []string
	hasC     bool
}

// pending reports whether p carries a by-name reference that has not been resolved.
func (p FilterParams) pending() bool {
	return p.refs == nil && (p.SubscriptionCode != nil || p.SubscriptionName != nil ||
		p.ConnectionCode != nil || p.ConnectionID != nil)
}

// refsWhere renders the resolved references into f.
func (p FilterParams) refsWhere(f *repocommon.Filter) {
	if p.pending() {
		f.Cond("FALSE") // never widen: an unresolved reference selects nothing
		return
	}
	if p.refs == nil {
		return
	}
	if p.refs.hasS {
		f.Clause("subscription_id = ANY($%d)", nonNil(p.refs.subs))
	}
	if p.refs.hasC {
		a, b := f.Arg(nonNil(p.refs.connIDs)), f.Arg(nonNil(p.refs.connSubs))
		f.Cond(fmt.Sprintf("(connection_id = ANY($%d) OR subscription_id = ANY($%d))", a, b))
	}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// nameMatch renders a column match for a user-typed value: a trailing * makes it a
// prefix (LIKE, with %, _ and \ escaped), anything else is an exact match.
// fold compares case-insensitively.
func nameMatch(col string, v string, fold bool, args *[]any) string {
	prefix := strings.HasSuffix(v, "*")
	v = strings.TrimSuffix(v, "*")
	if !prefix {
		*args = append(*args, v)
		if fold {
			return fmt.Sprintf("lower(%s) = lower($%d)", col, len(*args))
		}
		return fmt.Sprintf("%s = $%d", col, len(*args))
	}
	esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(v) + "%"
	*args = append(*args, esc)
	if fold {
		return fmt.Sprintf(`lower(%s) LIKE lower($%d) ESCAPE '\'`, col, len(*args))
	}
	return fmt.Sprintf(`%s LIKE $%d ESCAPE '\'`, col, len(*args))
}

// ResolveRefs resolves p's subscription code/name and connection references to ids,
// so the list query stays a single-table read. The lookups hit msg_subscriptions and
// msg_connections (control-plane tables: hundreds of rows, indexed on code), scoped to
// the platform's own rows plus the caller's tenants (p.AccessibleClientIDs), so a
// client user cannot probe another tenant's names.
//
// A name that matches nothing is not an error: the returned params select no jobs.
// A name that matches more than the cap is a 400 TOO_MANY_MATCHES.
func (r *Repository) ResolveRefs(ctx context.Context, p FilterParams) (FilterParams, error) {
	if p.SubscriptionCode == nil && p.SubscriptionName == nil && p.ConnectionCode == nil && p.ConnectionID == nil {
		return p, nil
	}
	out := &resolvedRefs{}
	if p.SubscriptionCode != nil || p.SubscriptionName != nil {
		var conds []string
		var args []any
		if p.SubscriptionCode != nil {
			conds = append(conds, nameMatch("code", *p.SubscriptionCode, false, &args))
		}
		if p.SubscriptionName != nil {
			conds = append(conds, nameMatch("name", *p.SubscriptionName, true, &args))
		}
		ids, err := r.lookupIDs(ctx, "msg_subscriptions", conds, args, p.AccessibleClientIDs, MaxResolvedSubscriptions, "subscription")
		if err != nil {
			return p, err
		}
		out.hasS, out.subs = true, ids
	}
	if p.ConnectionCode != nil || p.ConnectionID != nil {
		out.hasC = true
		switch {
		case p.ConnectionCode != nil:
			var args []any
			conds := []string{nameMatch("code", *p.ConnectionCode, false, &args)}
			ids, err := r.lookupIDs(ctx, "msg_connections", conds, args, p.AccessibleClientIDs, MaxResolvedConnections, "connection")
			if err != nil {
				return p, err
			}
			if p.ConnectionID != nil { // both given: the id must be one the code names
				kept := ids[:0]
				for _, id := range ids {
					if id == *p.ConnectionID {
						kept = append(kept, id)
					}
				}
				ids = kept
			}
			out.connIDs = ids
		default:
			out.connIDs = []string{*p.ConnectionID}
		}
		if len(out.connIDs) > 0 {
			args := []any{out.connIDs}
			subs, err := r.lookupIDs(ctx, "msg_subscriptions", []string{"connection_id = ANY($1)"}, args, p.AccessibleClientIDs, MaxResolvedSubscriptions, "subscription")
			if err != nil {
				return p, err
			}
			out.connSubs = subs
		}
	}
	p.refs = out
	return p, nil
}

// lookupIDs selects the ids of table's rows matching conds, tenant-scoped, at most
// limit of them (more is TooManyMatchesCode).
func (r *Repository) lookupIDs(ctx context.Context, table string, conds []string, args []any, scope *[]string, limit int, what string) ([]string, error) {
	if scope != nil {
		args = append(args, *scope)
		conds = append(conds, fmt.Sprintf("(client_id IS NULL OR client_id = ANY($%d))", len(args)))
	}
	args = append(args, limit+1)
	q := fmt.Sprintf("SELECT id FROM %s WHERE %s ORDER BY id LIMIT $%d", table, strings.Join(conds, " AND "), len(args))
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	if len(ids) > limit {
		return nil, usecase.Validation(TooManyMatchesCode,
			fmt.Sprintf("the %s filter matches more than %d records; narrow it", what, limit))
	}
	return ids, nil
}
