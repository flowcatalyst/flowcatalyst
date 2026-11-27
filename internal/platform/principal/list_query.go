package principal

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
)

const principalSelectCols = `
	p.id, p.type, p.scope, p.client_id, p.application_id, p.name, p.active,
	p.email, p.email_domain, p.idp_type, p.external_idp_id, p.password_hash,
	p.last_login_at, p.service_account_id, p.created_at, p.updated_at, p.all_applications,
	p.dev_client_secret_ref, p.dev_client_secret_updated_at, p.sessions_valid_after`

// VisibilityScope encodes the caller's row-level visibility over iam_principals
// (matching api.PrincipalVisible):
//   - Anchor == true: every principal is visible.
//   - Anchor == false: principals homed at one of AccessibleClients, plus
//     PARTNER principals (with no home client) granted at least one of
//     AccessibleClients in iam_client_access_grants.
type VisibilityScope struct {
	Anchor            bool
	AccessibleClients []string
}

// ListFilter describes the SQL filter, sort, and pagination parameters for
// GET /api/principals.
type ListFilter struct {
	Visibility VisibilityScope
	Type       string   // "" (all) or upper-cased type ("USER", "SERVICE", ...)
	Active     *bool    // nil = both true and false
	ClientID   string   // matches p.client_id OR iam_client_access_grants.client_id
	Query      string   // lower-cased literal substring matched on name or USER email
	Roles      []string // non-empty: principal holds at least one role in iam_principal_roles
	SortField  string   // "name" | "email" | "createdAt" (default)
	SortDesc   bool
	Page       int // 0-based; negative clamps to 0
	PageSize   int // <= 0 returns all matching rows
}

// FindFiltered queries iam_principals with SQL-pushed visibility, filters,
// sorting, and pagination, then batch-hydrates role assignments and client
// access grants ONLY for the returned page slice. Returns (pageRows, totalMatches, err).
func (r *Repository) FindFiltered(ctx context.Context, f ListFilter) ([]Principal, int, error) {
	whereSQL, args := buildListWhere(f)
	orderSQL := OrderByClause(f.SortField, f.SortDesc)

	if f.PageSize <= 0 {
		q := `SELECT ` + principalSelectCols + ` FROM iam_principals p` + whereSQL + ` ORDER BY ` + orderSQL
		out, err := r.queryAndHydratePrincipals(ctx, q, args...)
		if err != nil {
			return nil, 0, err
		}
		return out, len(out), nil
	}

	var total int
	countQuery := `SELECT COUNT(*) FROM iam_principals p` + whereSQL
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("principal repo: count filtered: %w", err)
	}
	if total == 0 {
		return []Principal{}, 0, nil
	}

	page := max(f.Page, 0)
	offset := int64(page) * int64(f.PageSize)
	if offset >= int64(total) {
		return []Principal{}, total, nil
	}

	limitParam := "$" + strconv.Itoa(len(args)+1)
	offsetParam := "$" + strconv.Itoa(len(args)+2)
	pageArgs := make([]any, 0, len(args)+2)
	pageArgs = append(pageArgs, args...)
	pageArgs = append(pageArgs, f.PageSize, offset)

	q := `SELECT ` + principalSelectCols +
		` FROM iam_principals p` + whereSQL +
		` ORDER BY ` + orderSQL +
		` LIMIT ` + limitParam + ` OFFSET ` + offsetParam
	out, err := r.queryAndHydratePrincipals(ctx, q, pageArgs...)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// FindUsersByEmails loads visible USER principals matching any of lowerEmails
// (case-insensitively), with roles and client access grants batch-hydrated only
// for the matched rows. Backs POST /api/principals/match-emails without scanning
// the entire iam_principals table.
func (r *Repository) FindUsersByEmails(ctx context.Context, lowerEmails []string, vis VisibilityScope) ([]Principal, error) {
	if len(lowerEmails) == 0 {
		return []Principal{}, nil
	}
	var conds []string
	var args []any

	args = append(args, nonNilStrings(lowerEmails))
	conds = append(conds, `p.type = 'USER'`, `p.email IS NOT NULL`, `LOWER(p.email) = ANY($1::text[])`)
	appendVisibilityCond(&conds, &args, vis)

	q := `SELECT ` + principalSelectCols +
		` FROM iam_principals p WHERE ` + strings.Join(conds, " AND ")
	return r.queryAndHydratePrincipals(ctx, q, args...)
}

func (r *Repository) queryAndHydratePrincipals(ctx context.Context, query string, args ...any) ([]Principal, error) {
	pgRows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	rows, err := pgx.CollectRows(pgRows, pgx.RowToStructByName[dbq.IamPrincipal])
	if err != nil {
		return nil, err
	}

	// Preserve X-06: a corrupted type or scope on any returned row fails the
	// whole read rather than silently skipping or coercing that row.
	out := make([]Principal, 0, len(rows))
	for _, row := range rows {
		p, err := rowToPrincipal(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	if err := r.hydrateRolesAll(ctx, out); err != nil {
		return nil, err
	}
	if err := r.hydrateClientGrantsAll(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

func buildListWhere(f ListFilter) (string, []any) {
	var conds []string
	var args []any

	appendVisibilityCond(&conds, &args, f.Visibility)

	if f.Type != "" {
		p := nextParam(&args, f.Type)
		conds = append(conds, `p.type = `+p)
	}
	if f.Active != nil {
		p := nextParam(&args, *f.Active)
		conds = append(conds, `p.active = `+p)
	}
	if f.ClientID != "" {
		p := nextParam(&args, f.ClientID)
		conds = append(conds, `(p.client_id = `+p+` OR EXISTS (
			SELECT 1 FROM iam_client_access_grants g
			WHERE g.principal_id = p.id AND g.client_id = `+p+`
		))`)
	}
	if f.Query != "" {
		// strpos performs literal substring matching (no LIKE % or _ wildcards),
		// and email only belongs to USER principals (rowToPrincipal leaves
		// UserIdentity nil on SERVICE rows).
		p := nextParam(&args, f.Query)
		conds = append(conds, `(strpos(LOWER(p.name), `+p+`) > 0 OR (p.type = 'USER' AND p.email IS NOT NULL AND strpos(LOWER(p.email), `+p+`) > 0))`)
	}
	if len(f.Roles) > 0 {
		p := nextParam(&args, nonNilStrings(f.Roles))
		conds = append(conds, `EXISTS (
			SELECT 1 FROM iam_principal_roles pr
			WHERE pr.principal_id = p.id AND pr.role_name = ANY(`+p+`::text[])
		)`)
	}

	if len(conds) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

func appendVisibilityCond(conds *[]string, args *[]any, vis VisibilityScope) {
	if vis.Anchor {
		return
	}
	p := nextParam(args, nonNilStrings(vis.AccessibleClients))
	*conds = append(*conds, `(
		(p.client_id IS NOT NULL AND p.client_id = ANY(`+p+`::text[]))
		OR (
			p.client_id IS NULL
			AND p.scope = 'PARTNER'
			AND EXISTS (
				SELECT 1 FROM iam_client_access_grants g
				WHERE g.principal_id = p.id AND g.client_id = ANY(`+p+`::text[])
			)
		)
	)`)
}

// OrderByClause returns a strictly whitelisted SQL ORDER BY expression (never
// interpolating user input). COLLATE "C" matches Go's bytewise strings.Compare
// on lower-cased strings, and p.id provides a unique tie-breaker so OFFSET/LIMIT
// pages never overlap or skip rows.
func OrderByClause(sortField string, desc bool) string {
	switch sortField {
	case "name":
		if desc {
			return `LOWER(p.name) COLLATE "C" DESC, p.created_at ASC, p.id DESC`
		}
		return `LOWER(p.name) COLLATE "C" ASC, p.created_at DESC, p.id ASC`
	case "email":
		const expr = `(CASE WHEN p.type = 'USER' THEN LOWER(COALESCE(p.email, '')) ELSE '' END) COLLATE "C"`
		if desc {
			return expr + ` DESC, p.created_at ASC, p.id DESC`
		}
		return expr + ` ASC, p.created_at DESC, p.id ASC`
	default: // "createdAt" and any unknown/empty key
		if desc {
			return `p.created_at DESC, p.id DESC`
		}
		return `p.created_at ASC, p.id ASC`
	}
}

func nextParam(args *[]any, val any) string {
	*args = append(*args, val)
	return "$" + strconv.Itoa(len(*args))
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
