// Package appusers is the user and invitation administration an application
// performs for its own users, as its own service account (F20). It backs the
// application admin pages of the framework (users and invitations): list and
// read the application's users, invite one, and list, resend and revoke
// pending invitations.
//
// An application's users are, by definition, the CLIENT-scope users who are
//
//   - homed at a client the application is enabled for (an enabled
//     app_client_configs row), and
//   - hold at least one of the application's roles (a role named
//     `{application code}:...`, the canonical form).
//
// That is the narrowest definition that is still meaningful: a user who holds
// none of the application's roles is not the application's, an anchor or
// partner user is a platform-level principal the application does not
// administer, and a client the application is not enabled for is not its
// tenant. Every read and every write goes through the same predicate
// (Scope.where), so a list, a get-by-id and a resend cannot disagree about who
// is visible.
package appusers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/repocommon"
)

// Repository reads the application's users and its pending invitations.
type Repository struct{ pool *pgxpool.Pool }

// NewRepository wires the repository.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// Scope is the set of users one caller may see for one application.
type Scope struct {
	// ApplicationID and ApplicationCode identify the application.
	ApplicationID   string
	ApplicationCode string
	// ClientIDs, when non-nil, narrows the visible users to those homed at one
	// of these clients: the caller's own client reach when it is not an
	// anchor. nil means every client the application is enabled for.
	ClientIDs *[]string
}

// where renders the visibility predicate onto f. Table alias p is
// iam_principals.
func (s Scope) where(f *repocommon.Filter) {
	f.Cond("p.type = 'USER'")
	f.Cond("p.scope = 'CLIENT'")
	f.Clause(`p.client_id IN (SELECT c.client_id FROM app_client_configs c
	                           WHERE c.application_id = $%d AND c.enabled)`, s.ApplicationID)
	f.Clause(`EXISTS (SELECT 1 FROM iam_principal_roles pr
	                   WHERE pr.principal_id = p.id AND starts_with(pr.role_name, $%d))`, s.ApplicationCode+":")
	if s.ClientIDs != nil {
		f.Clause("p.client_id = ANY($%d)", *s.ClientIDs)
	}
}

// User is one application user. It carries nothing sensitive: no password
// hash, no token, no second-factor state.
type User struct {
	ID          string
	Email       string
	Name        string
	Active      bool
	ClientID    string
	IdpType     string
	LastLoginAt *time.Time
	CreatedAt   time.Time
	// Roles are the application's own roles only; the user's roles in other
	// applications are not the application's to see.
	Roles []string
	// Invitation is "PENDING" while an unexpired invite token is outstanding,
	// "EXPIRED" for an invitee who has not set a password and has no live
	// token, and "" for a user who is not awaiting an invitation (a password
	// is set, a federated login, or inactive).
	Invitation string
}

// Invitation is one outstanding invitation: an active, internal-password user
// who has not yet set a password (the platform's own "password setup
// eligible" rule, passwordreset.passwordSetupEligible). It is PENDING while an
// unexpired invite token exists and EXPIRED otherwise (the token lapsed and
// was purged, or never existed), in which case a resend is what the invitee
// needs.
type Invitation struct {
	PrincipalID string
	Email       string
	Name        string
	ClientID    string
	Roles       []string
	Status      string // PENDING | EXPIRED
	// CreatedAt is when the latest invite was minted, or the user was created
	// when no token remains.
	CreatedAt time.Time
	// ExpiresAt is when the live token expires; nil when none remains.
	ExpiresAt *time.Time
}

// invitable is the SQL form of "awaiting an invitation": the platform's
// password-setup-eligibility rule on iam_principals.
const invitable = `(p.active AND p.password_hash IS NULL AND p.external_idp_id IS NULL
	AND COALESCE(p.idp_type, 'INTERNAL') <> 'OIDC')`

// ListQuery is the user list's filter and page.
type ListQuery struct {
	Q        string
	Active   *bool
	ClientID string
	Roles    []string
	Sort     string // name | email | createdAt
	Desc     bool
	Page     int
	PageSize int
}

const latestInvite = `
	LEFT JOIN (SELECT DISTINCT ON (principal_id) principal_id, created_at, expires_at
	             FROM iam_password_reset_tokens
	            WHERE purpose = 'invite'
	            ORDER BY principal_id, created_at DESC) t ON t.principal_id = p.id`

const userCols = `p.id, COALESCE(p.email, ''), p.name, p.active, COALESCE(p.client_id, ''),
	COALESCE(p.idp_type, 'INTERNAL'), p.last_login_at, p.created_at,
	ARRAY(SELECT pr2.role_name FROM iam_principal_roles pr2
	       WHERE pr2.principal_id = p.id AND starts_with(pr2.role_name, $1)
	       ORDER BY pr2.role_name),
	t.expires_at, ` + invitable

func scanUser(row pgx.Row, total *int) (User, error) {
	var (
		u        User
		expires  *time.Time
		awaiting bool
		args     = []any{&u.ID, &u.Email, &u.Name, &u.Active, &u.ClientID, &u.IdpType, &u.LastLoginAt, &u.CreatedAt, &u.Roles, &expires, &awaiting}
	)
	if total != nil {
		args = append(args, total)
	}
	if err := row.Scan(args...); err != nil {
		return User{}, err
	}
	if awaiting {
		u.Invitation = inviteStatus(expires)
	}
	return u, nil
}

// List returns one page of the scope's users and the total matching.
func (r *Repository) List(ctx context.Context, s Scope, q ListQuery) ([]User, int, error) {
	var f repocommon.Filter
	// $1 is the role prefix for the roles projection; claim it first.
	f.Arg(s.ApplicationCode + ":")
	s.where(&f)
	if q.Active != nil {
		f.Cond(fmt.Sprintf("p.active = $%d", f.Arg(*q.Active)))
	}
	if q.ClientID != "" {
		f.Cond(fmt.Sprintf("p.client_id = $%d", f.Arg(q.ClientID)))
	}
	if q.Q != "" {
		f.Cond(fmt.Sprintf("(p.name ILIKE $%d OR p.email ILIKE $%d)", f.Arg("%"+escapeLike(q.Q)+"%"), f.Arg("%"+escapeLike(q.Q)+"%")))
	}
	if len(q.Roles) > 0 {
		f.Cond(fmt.Sprintf(`EXISTS (SELECT 1 FROM iam_principal_roles pr3
		                             WHERE pr3.principal_id = p.id AND pr3.role_name = ANY($%d))`, f.Arg(q.Roles)))
	}
	col := "p.created_at"
	switch q.Sort {
	case "name":
		col = "lower(p.name)"
	case "email":
		col = "lower(p.email)"
	}
	dir := "ASC"
	if q.Desc {
		dir = "DESC"
	}
	size := q.PageSize
	page := max(q.Page, 0)
	sql := `SELECT ` + userCols + `, count(*) OVER() FROM iam_principals p` + latestInvite +
		f.Where() + fmt.Sprintf(" ORDER BY %s %s, p.id ASC LIMIT $%d OFFSET $%d", col, dir, f.Arg(size), f.Arg(page*size))
	rows, err := r.pool.Query(ctx, sql, f.Args()...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []User{}
	total := 0
	for rows.Next() {
		var n int
		u, err := scanUser(rows, &n)
		if err != nil {
			return nil, 0, err
		}
		total = n
		out = append(out, u)
	}
	return out, total, rows.Err()
}

// Get returns one user of the scope, or nil when the id is not in it: an id
// that does not exist and an id of someone outside the scope are the same
// answer.
func (r *Repository) Get(ctx context.Context, s Scope, id string) (*User, error) {
	var f repocommon.Filter
	f.Arg(s.ApplicationCode + ":")
	s.where(&f)
	f.Cond(fmt.Sprintf("p.id = $%d", f.Arg(id)))
	row := r.pool.QueryRow(ctx, `SELECT `+userCols+` FROM iam_principals p`+latestInvite+f.Where(), f.Args()...)
	u, err := scanUser(row, nil)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func inviteStatus(expires *time.Time) string {
	if expires != nil && expires.After(time.Now()) {
		return "PENDING"
	}
	return "EXPIRED"
}

const invitationCols = `p.id, COALESCE(p.email, ''), p.name, COALESCE(p.client_id, ''),
	ARRAY(SELECT pr2.role_name FROM iam_principal_roles pr2
	       WHERE pr2.principal_id = p.id AND starts_with(pr2.role_name, $1)
	       ORDER BY pr2.role_name),
	COALESCE(t.created_at, p.created_at), t.expires_at`

func scanInvitation(row pgx.Row, total *int) (Invitation, error) {
	var i Invitation
	args := []any{&i.PrincipalID, &i.Email, &i.Name, &i.ClientID, &i.Roles, &i.CreatedAt, &i.ExpiresAt}
	if total != nil {
		args = append(args, total)
	}
	if err := row.Scan(args...); err != nil {
		return Invitation{}, err
	}
	i.Status = inviteStatus(i.ExpiresAt)
	return i, nil
}

// Invitations returns one page of the scope's outstanding invitations (see
// Invitation), newest first, and the total. status filters to PENDING or
// EXPIRED ("" both). The token itself and its hash never leave the database.
func (r *Repository) Invitations(ctx context.Context, s Scope, status string, page, size int) ([]Invitation, int, error) {
	var f repocommon.Filter
	f.Arg(s.ApplicationCode + ":")
	s.where(&f)
	f.Cond(invitable)
	switch status {
	case "PENDING":
		f.Cond("t.expires_at > now()")
	case "EXPIRED":
		f.Cond("(t.expires_at IS NULL OR t.expires_at <= now())")
	}
	sql := `SELECT ` + invitationCols + `, count(*) OVER() FROM iam_principals p` + latestInvite + f.Where() +
		fmt.Sprintf(" ORDER BY COALESCE(t.created_at, p.created_at) DESC, p.id ASC LIMIT $%d OFFSET $%d", f.Arg(size), f.Arg(max(page, 0)*size))
	rows, err := r.pool.Query(ctx, sql, f.Args()...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Invitation{}
	total := 0
	for rows.Next() {
		var n int
		i, err := scanInvitation(rows, &n)
		if err != nil {
			return nil, 0, err
		}
		total = n
		out = append(out, i)
	}
	return out, total, rows.Err()
}

// GetInvitation returns the scope's outstanding invitation for a user id, or
// nil when the id is not an invitee in the scope (not visible, not awaiting an
// invitation, or not a user at all: all the same answer).
func (r *Repository) GetInvitation(ctx context.Context, s Scope, id string) (*Invitation, error) {
	var f repocommon.Filter
	f.Arg(s.ApplicationCode + ":")
	s.where(&f)
	f.Cond(invitable)
	f.Cond(fmt.Sprintf("p.id = $%d", f.Arg(id)))
	row := r.pool.QueryRow(ctx, `SELECT `+invitationCols+` FROM iam_principals p`+latestInvite+f.Where(), f.Args()...)
	i, err := scanInvitation(row, nil)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &i, nil
}

// RevokeInvites deletes the principal's outstanding invite tokens. It
// returns how many it deleted.
func (r *Repository) RevokeInvites(ctx context.Context, principalID string) (int64, error) {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM iam_password_reset_tokens WHERE principal_id = $1 AND purpose = 'invite'`, principalID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// escapeLike escapes the LIKE metacharacters of a user-supplied search.
func escapeLike(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\', '%', '_':
			out = append(out, '\\')
		}
		out = append(out, s[i])
	}
	return string(out)
}
