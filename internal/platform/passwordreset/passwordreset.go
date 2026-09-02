// Package passwordreset stores short-lived single-use reset tokens.
// Writes are infrastructure
// processing (auth/password-reset-request directly inserts; the consume
// step uses DELETE ... RETURNING).
package passwordreset

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// Purpose distinguishes a normal password reset from a first-time account
// invite (which carries a longer TTL and a different email template).
type Purpose string

const (
	// PurposeReset is the standard "forgot password" / admin-triggered reset.
	PurposeReset Purpose = "reset"
	// PurposeInvite is the first-time "set your password" account invite.
	PurposeInvite Purpose = "invite"
)

// ParsePurpose parses a stored purpose value. Returns ok=false for anything
// other than exactly "reset" or "invite" — callers MUST reject on ok=false
// rather than coerce an unrecognised value to "reset" (X-06: a loud read
// error, never a silent default). Follows the (T, bool) shape of
// common.ParseOutboxItemType.
func ParsePurpose(s string) (Purpose, bool) {
	switch Purpose(s) {
	case PurposeReset, PurposeInvite:
		return Purpose(s), true
	default:
		return "", false
	}
}

// Token is the reset-token record. The plaintext token is sent via
// email; we store only its hash.
type Token struct {
	ID          string  `json:"id"`
	PrincipalID string  `json:"principalId"`
	TokenHash   string  `json:"tokenHash"`
	Purpose     Purpose `json:"purpose"`
	// Reset2FA, when set, clears the user's enrolled second factors on
	// confirm and forces re-enrollment (lost-device recovery path).
	Reset2FA bool `json:"reset2fa"`
	// RequiresFactor, when set, means confirming additionally requires proving
	// an authenticator (TOTP) code — email alone can't authorize the reset.
	RequiresFactor bool `json:"requiresFactor"`
	// FactorAttempts counts failed factor proofs against this token. The
	// confirm handler burns the principal's token set once it crosses the
	// wrong-guess ceiling (the token is deliberately not consumed on a wrong
	// code, so without this the retry allowance is a TOTP brute-force window).
	FactorAttempts int `json:"factorAttempts"`
	// RedirectURI, when set on an invite token, is followed by the SPA after
	// a successful confirm (portal invites chain back into the portal's OAuth
	// login). Validated against the owning client's portal OAuth clients'
	// registered redirect URIs at mint time — never caller-controlled at
	// confirm time.
	RedirectURI *string   `json:"redirectUri,omitempty"`
	ExpiresAt   time.Time `json:"expiresAt"`
	CreatedAt   time.Time `json:"createdAt"`
}

// New constructs a standard reset Token.
func New(principalID, tokenHash string, expiresAt time.Time) *Token {
	return &Token{
		ID:          tsid.Generate(tsid.PasswordResetToken),
		PrincipalID: principalID,
		TokenHash:   tokenHash,
		Purpose:     PurposeReset,
		ExpiresAt:   expiresAt,
		CreatedAt:   time.Now().UTC(),
	}
}

// IsExpired reports whether the token's expiry has passed.
func (t *Token) IsExpired() bool { return time.Now().After(t.ExpiresAt) }

// IsValid reports whether the token is not yet expired (single-use is
// enforced at the storage layer by deleting on consume).
func (t *Token) IsValid() bool { return !t.IsExpired() }

// Repository persists tokens. No UoW: this is short-lived session state.
type Repository struct{ pool *pgxpool.Pool }

// NewRepository wires a repo.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// Insert persists a new token.
func (r *Repository) Insert(ctx context.Context, t *Token) error {
	purpose := t.Purpose
	if purpose == "" {
		purpose = PurposeReset
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO iam_password_reset_tokens
		     (id, principal_id, token_hash, purpose, reset_2fa, requires_factor, redirect_uri, expires_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		t.ID, t.PrincipalID, t.TokenHash, string(purpose), t.Reset2FA, t.RequiresFactor, t.RedirectURI, t.ExpiresAt, t.CreatedAt)
	return err
}

// Consume deletes the token (single-use) and returns it if present and
// not expired. The DELETE ... RETURNING is race-free.
func (r *Repository) Consume(ctx context.Context, tokenHash string) (*Token, error) {
	row := r.pool.QueryRow(ctx,
		`DELETE FROM iam_password_reset_tokens
		   WHERE token_hash = $1 AND expires_at > NOW()
		 RETURNING id, principal_id, token_hash, purpose, reset_2fa, requires_factor, factor_attempts, redirect_uri, expires_at, created_at`,
		tokenHash)
	return scanToken(row)
}

// FindByTokenHash returns the token for the given hash WITHOUT consuming it
// (used by the validate endpoint, which must not delete). Returns (nil, nil)
// when absent. Expiry is reported by the caller via Token.IsExpired.
func (r *Repository) FindByTokenHash(ctx context.Context, tokenHash string) (*Token, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, principal_id, token_hash, purpose, reset_2fa, requires_factor, factor_attempts, redirect_uri, expires_at, created_at
		   FROM iam_password_reset_tokens WHERE token_hash = $1`,
		tokenHash)
	return scanToken(row)
}

// IncrementFactorAttempts records a failed factor proof against the token and
// returns the new count. Atomic (UPDATE ... RETURNING) so concurrent wrong
// guesses can't share a slot under the ceiling.
func (r *Repository) IncrementFactorAttempts(ctx context.Context, id string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`UPDATE iam_password_reset_tokens
		    SET factor_attempts = factor_attempts + 1
		  WHERE id = $1
		 RETURNING factor_attempts`, id).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("increment factor attempts: %w", err)
	}
	return n, nil
}

// scanToken reads a token row, mapping pgx.ErrNoRows to (nil, nil).
//
// A purpose value that isn't one of the known Purpose constants (junk
// written before write-boundary validation existed, or a hand-edited row)
// is a loud read error — never round-tripped as-is and never coerced to
// "reset", per the X-06 ruling. The row id is logged so the bad row can be
// found and fixed without a debugger.
func scanToken(row pgx.Row) (*Token, error) {
	var t Token
	var purpose string
	if err := row.Scan(&t.ID, &t.PrincipalID, &t.TokenHash, &purpose, &t.Reset2FA, &t.RequiresFactor, &t.FactorAttempts, &t.RedirectURI, &t.ExpiresAt, &t.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("scan token: %w", err)
	}
	p, ok := ParsePurpose(purpose)
	if !ok {
		slog.Error("password reset token row has unrecognised purpose",
			"id", t.ID, "purpose", purpose)
		return nil, usecase.Internal("CORRUPT_PASSWORD_RESET_PURPOSE",
			fmt.Sprintf("password reset token %s has an unrecognised purpose", t.ID), nil)
	}
	t.Purpose = p
	return &t, nil
}

// DeleteByPrincipalID removes every reset token for a principal. Used to
// invalidate outstanding tokens before issuing a new one and after a
// successful reset (single-use across the whole set).
func (r *Repository) DeleteByPrincipalID(ctx context.Context, principalID string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM iam_password_reset_tokens WHERE principal_id = $1`, principalID)
	return err
}

// PurgeExpired removes all expired tokens. Run periodically by a janitor.
func (r *Repository) PurgeExpired(ctx context.Context) (int64, error) {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM iam_password_reset_tokens WHERE expires_at <= NOW()`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
