package portalidentity

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// Repository is the pgx-backed store. Implements usecasepgx.Persist[Identity]
// so identity changes flow through the standard UoW (event + audit
// atomically).
type Repository struct{ pool *pgxpool.Pool }

// NewRepository wires the repo.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

const identityColumns = `pi.id, pi.client_id, pi.email, pi.name, pi.password_hash,
	pi.status, pi.source, pi.last_login_at, pi.invited_at, pi.invite_expires_at,
	pi.created_at, pi.updated_at`

const identitySelect = `SELECT ` + identityColumns + ` FROM portal_identities pi`

// FindByID returns the identity (with its app grants), nil if absent.
func (r *Repository) FindByID(ctx context.Context, id string) (*Identity, error) {
	return r.one(ctx, identitySelect+` WHERE pi.id = $1`, id)
}

// FindByClientAndEmail returns the identity for (client, email), nil if
// absent. Email is matched lower-cased.
func (r *Repository) FindByClientAndEmail(ctx context.Context, clientID, email string) (*Identity, error) {
	return r.one(ctx, identitySelect+` WHERE pi.client_id = $1 AND pi.email = $2`,
		clientID, strings.ToLower(strings.TrimSpace(email)))
}

// SearchFilter narrows a client's identities for the admin list.
type SearchFilter struct {
	ClientID string
	// Query is a prefix (TERM%) matched case-insensitively against email
	// and name. Empty matches everything.
	Query string
	// AppID restricts to identities granted that portal app. Empty = all.
	AppID  string
	Offset int
	Limit  int
}

// Search lists a client's identities, newest first, with the total match
// count for pagination.
func (r *Repository) Search(ctx context.Context, f SearchFilter) ([]Identity, int64, error) {
	where := []string{"pi.client_id = $1"}
	args := []any{f.ClientID}
	if q := strings.ToLower(strings.TrimSpace(f.Query)); q != "" {
		args = append(args, escapeLike(q)+"%")
		n := strconv.Itoa(len(args))
		where = append(where, `(pi.email LIKE $`+n+` OR lower(pi.name) LIKE $`+n+`)`)
	}
	if f.AppID != "" {
		args = append(args, f.AppID)
		where = append(where, `EXISTS (SELECT 1 FROM portal_identity_apps g
			WHERE g.identity_id = pi.id AND g.portal_app_id = $`+strconv.Itoa(len(args))+`)`)
	}
	cond := strings.Join(where, " AND ")

	var total int64
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM portal_identities pi WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("portal_identity search count: %w", err)
	}
	args = append(args, f.Limit, f.Offset)
	rows, err := r.pool.Query(ctx,
		identitySelect+` WHERE `+cond+` ORDER BY pi.created_at DESC, pi.id DESC LIMIT $`+
			strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("portal_identity search: %w", err)
	}
	out, err := scanAll(rows)
	if err != nil {
		return nil, 0, err
	}
	if err := r.attachApps(ctx, out); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// escapeLike neutralises LIKE metacharacters in user input (backslash is
// Postgres' default LIKE escape).
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// TouchLastLogin best-effort stamps a successful login.
func (r *Repository) TouchLastLogin(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE portal_identities SET last_login_at = NOW(), updated_at = NOW() WHERE id = $1`, id)
	return err
}

// MarkInvited records the latest invite (infrastructure bookkeeping on the
// invite path, like SetPasswordHash). expiresAt nil = an SSO invite, which
// never expires.
func (r *Repository) MarkInvited(ctx context.Context, id string, at time.Time, expiresAt *time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE portal_identities SET invited_at = $2, invite_expires_at = $3, updated_at = NOW() WHERE id = $1`,
		id, at, expiresAt)
	return err
}

// SetPasswordHash writes a freshly-set password (the invite/reset confirm
// path — infrastructure processing on the shared reset-token flow, mirroring
// how principal password resets write outside an operation envelope).
func (r *Repository) SetPasswordHash(ctx context.Context, id, hash string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE portal_identities SET password_hash = $2, updated_at = NOW() WHERE id = $1`, id, hash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("portal identity not found")
	}
	return nil
}

// Persist implements usecasepgx.Persist[Identity]. Conflict on the
// (client, email) unique key updates the mutable fields — re-ensuring an
// existing identity keeps its id/source/created_at. The app grants are then
// synced to i.Apps against the id that actually holds the row (a racing
// ensure may have won the insert).
func (r *Repository) Persist(ctx context.Context, i *Identity, tx *usecasepgx.DbTx) error {
	now := time.Now().UTC()
	var rowID string
	err := tx.Inner().QueryRow(ctx,
		`INSERT INTO portal_identities
		     (id, client_id, email, name, password_hash, status, source, last_login_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 ON CONFLICT (client_id, email) DO UPDATE SET
		     name = EXCLUDED.name,
		     status = EXCLUDED.status,
		     updated_at = EXCLUDED.updated_at
		 RETURNING id`,
		i.ID, i.ClientID, i.Email, emptyToNil(i.Name), i.PasswordHash,
		string(i.Status), string(i.Source), i.LastLoginAt, i.CreatedAt, now).Scan(&rowID)
	if err != nil {
		return err
	}
	appIDs := make([]string, 0, len(i.Apps))
	for _, g := range i.Apps {
		appIDs = append(appIDs, g.AppID)
	}
	if _, err := tx.Inner().Exec(ctx,
		`DELETE FROM portal_identity_apps WHERE identity_id = $1 AND NOT (portal_app_id = ANY($2))`,
		rowID, appIDs); err != nil {
		return err
	}
	for _, g := range i.Apps {
		if _, err := tx.Inner().Exec(ctx,
			`INSERT INTO portal_identity_apps (identity_id, portal_app_id, source, granted_at)
			 VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`,
			rowID, g.AppID, string(g.Source), g.GrantedAt); err != nil {
			return err
		}
	}
	return nil
}

// Delete removes the identity (offboarding — just the row; grants cascade).
func (r *Repository) Delete(ctx context.Context, i *Identity, tx *usecasepgx.DbTx) error {
	_, err := tx.Inner().Exec(ctx, `DELETE FROM portal_identities WHERE id = $1`, i.ID)
	return err
}

func (r *Repository) one(ctx context.Context, sql string, args ...any) (*Identity, error) {
	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("portal_identity repo: %w", err)
	}
	all, err := scanAll(rows)
	if err != nil || len(all) == 0 {
		return nil, err
	}
	if err := r.attachApps(ctx, all[:1]); err != nil {
		return nil, err
	}
	return &all[0], nil
}

// attachApps loads the app grants for a page of identities in one query.
func (r *Repository) attachApps(ctx context.Context, idents []Identity) error {
	if len(idents) == 0 {
		return nil
	}
	ids := make([]string, len(idents))
	index := make(map[string]int, len(idents))
	for n := range idents {
		ids[n] = idents[n].ID
		index[idents[n].ID] = n
		idents[n].Apps = []AppGrant{}
	}
	rows, err := r.pool.Query(ctx,
		`SELECT identity_id, portal_app_id, source, granted_at FROM portal_identity_apps
		 WHERE identity_id = ANY($1) ORDER BY granted_at`, ids)
	if err != nil {
		return fmt.Errorf("portal_identity apps: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var identityID, source string
		var g AppGrant
		if err := rows.Scan(&identityID, &g.AppID, &source, &g.GrantedAt); err != nil {
			return err
		}
		g.Source = Source(source)
		if n, ok := index[identityID]; ok {
			idents[n].Apps = append(idents[n].Apps, g)
		}
	}
	return rows.Err()
}

func emptyToNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func scanAll(rows pgx.Rows) ([]Identity, error) {
	defer rows.Close()
	out := []Identity{}
	for rows.Next() {
		var i Identity
		var name *string
		var status, source string
		if err := rows.Scan(&i.ID, &i.ClientID, &i.Email, &name, &i.PasswordHash,
			&status, &source, &i.LastLoginAt, &i.InvitedAt, &i.InviteExpiresAt,
			&i.CreatedAt, &i.UpdatedAt); err != nil {
			return nil, err
		}
		if name != nil {
			i.Name = *name
		}
		i.Status = Status(status)
		i.Source = Source(source)
		out = append(out, i)
	}
	return out, rows.Err()
}
