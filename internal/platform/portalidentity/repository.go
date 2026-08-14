package portalidentity

import (
	"context"
	"errors"
	"fmt"
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

const identitySelect = `SELECT id, client_id, email, name, password_hash,
	status, source, last_login_at, created_at, updated_at FROM portal_identities`

// FindByID returns the identity, nil if absent.
func (r *Repository) FindByID(ctx context.Context, id string) (*Identity, error) {
	rows, err := r.pool.Query(ctx, identitySelect+` WHERE id = $1`, id)
	if err != nil {
		return nil, fmt.Errorf("portal_identity repo: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	return scanIdentity(rows)
}

// FindByClientAndEmail returns the identity for (client, email), nil if
// absent. Email is matched lower-cased.
func (r *Repository) FindByClientAndEmail(ctx context.Context, clientID, email string) (*Identity, error) {
	rows, err := r.pool.Query(ctx,
		identitySelect+` WHERE client_id = $1 AND email = $2`,
		clientID, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return nil, fmt.Errorf("portal_identity repo: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	return scanIdentity(rows)
}

// FindByClient lists a client's portal identities, newest first.
func (r *Repository) FindByClient(ctx context.Context, clientID string) ([]Identity, error) {
	rows, err := r.pool.Query(ctx,
		identitySelect+` WHERE client_id = $1 ORDER BY created_at DESC`, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Identity
	for rows.Next() {
		i, err := scanIdentity(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *i)
	}
	return out, rows.Err()
}

// TouchLastLogin best-effort stamps a successful login.
func (r *Repository) TouchLastLogin(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE portal_identities SET last_login_at = NOW(), updated_at = NOW() WHERE id = $1`, id)
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
// existing identity keeps its id/source/created_at.
func (r *Repository) Persist(ctx context.Context, i *Identity, tx *usecasepgx.DbTx) error {
	now := time.Now().UTC()
	_, err := tx.Inner().Exec(ctx,
		`INSERT INTO portal_identities
		     (id, client_id, email, name, password_hash, status, source, last_login_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 ON CONFLICT (client_id, email) DO UPDATE SET
		     name = EXCLUDED.name,
		     status = EXCLUDED.status,
		     updated_at = EXCLUDED.updated_at`,
		i.ID, i.ClientID, i.Email, emptyToNil(i.Name), i.PasswordHash,
		string(i.Status), string(i.Source), i.LastLoginAt, i.CreatedAt, now)
	return err
}

// Delete removes the identity (offboarding — just the row).
func (r *Repository) Delete(ctx context.Context, i *Identity, tx *usecasepgx.DbTx) error {
	_, err := tx.Inner().Exec(ctx, `DELETE FROM portal_identities WHERE id = $1`, i.ID)
	return err
}

func emptyToNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func scanIdentity(rows pgx.Rows) (*Identity, error) {
	var i Identity
	var name *string
	var status, source string
	if err := rows.Scan(&i.ID, &i.ClientID, &i.Email, &name, &i.PasswordHash,
		&status, &source, &i.LastLoginAt, &i.CreatedAt, &i.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if name != nil {
		i.Name = *name
	}
	i.Status = Status(status)
	i.Source = Source(source)
	return &i, nil
}
