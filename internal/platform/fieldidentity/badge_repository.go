package fieldidentity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// ── badges ───────────────────────────────────────────────────────────────

// BadgeRepo stores field_badges. Implements usecasepgx.Persist[Badge].
type BadgeRepo struct{ pool *pgxpool.Pool }

const badgeColumns = `id, client_id, identity_id, token_hash, label, status, created_by, created_at,
	revoked_at, last_used_at`

func scanBadge(row pgx.Row) (*Badge, error) {
	var b Badge
	var label, createdBy *string
	var status string
	err := row.Scan(&b.ID, &b.ClientID, &b.IdentityID, &b.TokenHash, &label, &status, &createdBy,
		&b.CreatedAt, &b.RevokedAt, &b.LastUsedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("field_badge scan: %w", err)
	}
	b.Label, b.CreatedBy, b.Status = deref(label), deref(createdBy), BadgeStatus(status)
	return &b, nil
}

// FindByID returns the badge, nil if absent.
func (r *BadgeRepo) FindByID(ctx context.Context, id string) (*Badge, error) {
	return scanBadge(r.pool.QueryRow(ctx, `SELECT `+badgeColumns+` FROM field_badges WHERE id = $1`, id))
}

// FindByHash returns the badge (any status) for a token hash, nil if absent.
func (r *BadgeRepo) FindByHash(ctx context.Context, hash string) (*Badge, error) {
	return scanBadge(r.pool.QueryRow(ctx, `SELECT `+badgeColumns+` FROM field_badges WHERE token_hash = $1`, hash))
}

// ListByIdentity returns every badge of a worker, newest first.
func (r *BadgeRepo) ListByIdentity(ctx context.Context, identityID string) ([]Badge, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+badgeColumns+` FROM field_badges WHERE identity_id = $1 ORDER BY created_at DESC, id DESC`, identityID)
	if err != nil {
		return nil, fmt.Errorf("field_badge list: %w", err)
	}
	defer rows.Close()
	out := []Badge{}
	for rows.Next() {
		b, err := scanBadge(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
	}
	return out, rows.Err()
}

// ActiveByIdentity returns the worker's ACTIVE badge, nil if none.
func (r *BadgeRepo) ActiveByIdentity(ctx context.Context, identityID string) (*Badge, error) {
	return scanBadge(r.pool.QueryRow(ctx,
		`SELECT `+badgeColumns+` FROM field_badges WHERE identity_id = $1 AND status = 'ACTIVE'
		 ORDER BY created_at DESC LIMIT 1`, identityID))
}

// ActiveByIdentities returns the ACTIVE badge of each listed worker, by worker id.
func (r *BadgeRepo) ActiveByIdentities(ctx context.Context, identityIDs []string) (map[string]*Badge, error) {
	out := make(map[string]*Badge, len(identityIDs))
	if len(identityIDs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx,
		`SELECT `+badgeColumns+` FROM field_badges WHERE identity_id = ANY($1) AND status = 'ACTIVE'`, identityIDs)
	if err != nil {
		return nil, fmt.Errorf("field_badge active: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		b, err := scanBadge(rows)
		if err != nil {
			return nil, err
		}
		out[b.IdentityID] = b
	}
	return out, rows.Err()
}

// RevokeActive revokes the worker's ACTIVE badges inside the caller's
// transaction and returns the ids it revoked (issuing a new badge supersedes
// the earlier ones).
func (r *BadgeRepo) RevokeActive(ctx context.Context, tx pgx.Tx, identityID string, at time.Time) ([]string, error) {
	rows, err := tx.Query(ctx,
		`UPDATE field_badges SET status = 'REVOKED', revoked_at = $2
		 WHERE identity_id = $1 AND status = 'ACTIVE' RETURNING id`, identityID, at)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// Persist implements usecasepgx.Persist[Badge]: insert, or update the state of
// an existing badge (revocation).
func (r *BadgeRepo) Persist(ctx context.Context, b *Badge, tx *usecasepgx.DbTx) error {
	_, err := tx.Inner().Exec(ctx,
		`INSERT INTO field_badges (id, client_id, identity_id, token_hash, label, status, created_by, created_at,
		                           revoked_at, last_used_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		 ON CONFLICT (id) DO UPDATE SET status = EXCLUDED.status, revoked_at = EXCLUDED.revoked_at,
		     label = EXCLUDED.label`,
		b.ID, b.ClientID, b.IdentityID, b.TokenHash, emptyToNil(b.Label), string(b.Status), emptyToNil(b.CreatedBy),
		b.CreatedAt, b.RevokedAt, b.LastUsedAt)
	return err
}

// Delete removes the badge row.
func (r *BadgeRepo) Delete(ctx context.Context, b *Badge, tx *usecasepgx.DbTx) error {
	_, err := tx.Inner().Exec(ctx, `DELETE FROM field_badges WHERE id = $1`, b.ID)
	return err
}

// ── sign-in codes ────────────────────────────────────────────────────────

// SignInCodeRepo stores field_sign_in_codes. Implements
// usecasepgx.Persist[SignInCode].
type SignInCodeRepo struct{ pool *pgxpool.Pool }

const signInCodeColumns = `id, client_id, identity_id, application_id, code_hash, created_by, created_by_name,
	expires_at, used_at, created_at`

// Consume atomically marks an unused, unexpired code used and returns it; nil
// when the code is unknown, used or expired. Runs inside the caller's tx so a
// redemption that fails afterwards does not burn the code.
func (r *SignInCodeRepo) Consume(ctx context.Context, tx pgx.Tx, hash string) (*SignInCode, error) {
	var c SignInCode
	var name *string
	err := tx.QueryRow(ctx,
		`UPDATE field_sign_in_codes SET used_at = NOW()
		 WHERE code_hash = $1 AND used_at IS NULL AND expires_at > NOW()
		 RETURNING `+signInCodeColumns, hash).
		Scan(&c.ID, &c.ClientID, &c.IdentityID, &c.ApplicationID, &c.CodeHash, &c.CreatedBy, &name,
			&c.ExpiresAt, &c.UsedAt, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.CreatedByName = deref(name)
	return &c, nil
}

// Persist implements usecasepgx.Persist[SignInCode].
func (r *SignInCodeRepo) Persist(ctx context.Context, c *SignInCode, tx *usecasepgx.DbTx) error {
	_, err := tx.Inner().Exec(ctx,
		`INSERT INTO field_sign_in_codes (id, client_id, identity_id, application_id, code_hash, created_by,
		                                  created_by_name, expires_at, used_at, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		 ON CONFLICT (id) DO UPDATE SET used_at = EXCLUDED.used_at`,
		c.ID, c.ClientID, c.IdentityID, c.ApplicationID, c.CodeHash, c.CreatedBy, emptyToNil(c.CreatedByName),
		c.ExpiresAt, c.UsedAt, c.CreatedAt)
	return err
}

// Delete removes the code row.
func (r *SignInCodeRepo) Delete(ctx context.Context, c *SignInCode, tx *usecasepgx.DbTx) error {
	_, err := tx.Inner().Exec(ctx, `DELETE FROM field_sign_in_codes WHERE id = $1`, c.ID)
	return err
}
