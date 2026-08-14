// Package portalauth is the portal plane's front-channel auth surface
// (docs/portal-identity-plan.md Phase 2.5 v2): GET /portal/authorize,
// POST /portal/auth/check-domain, POST /portal/auth/login. Independent of
// the employee endpoints; never reads or writes fc_session — the portal app
// runs its own session from the id_token, so every portal login is a fresh
// authentication.
package portalauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LoginFlow is one parked /portal/authorize request: the validated OAuth
// chain waiting for the user to authenticate on the SPA portal login page.
// Short-TTL; consumed (single-use) when a code is issued.
type LoginFlow struct {
	ID                  string
	OAuthClientID       string // oauth_clients.client_id
	PortalClientID      string // owner tenant client (denormalized at stash time)
	RedirectURI         string
	Scope               *string
	State               string
	Nonce               *string
	CodeChallenge       *string
	CodeChallengeMethod *string
	CreatedAt           time.Time
	ExpiresAt           time.Time
}

// IsExpired reports whether the flow's TTL has passed.
func (f *LoginFlow) IsExpired() bool { return time.Now().UTC().After(f.ExpiresAt) }

// flowTTL bounds how long the portal login page may sit open before the
// user must restart from the portal (which just re-runs /portal/authorize).
const flowTTL = 15 * time.Minute

// NewLoginFlow builds a flow with a fresh random id and the default TTL.
func NewLoginFlow(oauthClientID, portalClientID, redirectURI, state string) (*LoginFlow, error) {
	id, err := randomToken(32)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	return &LoginFlow{
		ID:             id,
		OAuthClientID:  oauthClientID,
		PortalClientID: portalClientID,
		RedirectURI:    redirectURI,
		State:          state,
		CreatedAt:      now,
		ExpiresAt:      now.Add(flowTTL),
	}, nil
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("portal flow id: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// FlowRepo is the pgx-backed store for portal_login_flows.
type FlowRepo struct{ pool *pgxpool.Pool }

// NewFlowRepo wires the repo.
func NewFlowRepo(pool *pgxpool.Pool) *FlowRepo { return &FlowRepo{pool: pool} }

// Insert parks a fresh flow.
func (r *FlowRepo) Insert(ctx context.Context, f *LoginFlow) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO portal_login_flows
		     (id, oauth_client_id, portal_client_id, redirect_uri, scope, state,
		      nonce, code_challenge, code_challenge_method, created_at, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		f.ID, f.OAuthClientID, f.PortalClientID, f.RedirectURI, f.Scope, f.State,
		f.Nonce, f.CodeChallenge, f.CodeChallengeMethod, f.CreatedAt, f.ExpiresAt)
	if err != nil {
		return fmt.Errorf("portal_login_flows insert: %w", err)
	}
	return nil
}

const flowSelect = `SELECT id, oauth_client_id, portal_client_id, redirect_uri,
	scope, state, nonce, code_challenge, code_challenge_method, created_at, expires_at`

// Find loads a live (unexpired) flow WITHOUT consuming it — the login page
// reads it across the check-domain and login steps; a failed password
// attempt must not burn the flow. Returns (nil, nil) when absent/expired.
func (r *FlowRepo) Find(ctx context.Context, id string) (*LoginFlow, error) {
	row := r.pool.QueryRow(ctx,
		flowSelect+` FROM portal_login_flows WHERE id = $1 AND expires_at > NOW()`, id)
	return scanFlow(row)
}

// Consume atomically deletes and returns the live flow — single-use, called
// exactly when a code is about to be issued.
func (r *FlowRepo) Consume(ctx context.Context, id string) (*LoginFlow, error) {
	row := r.pool.QueryRow(ctx,
		`DELETE FROM portal_login_flows WHERE id = $1 AND expires_at > NOW()
		 RETURNING id, oauth_client_id, portal_client_id, redirect_uri,
		           scope, state, nonce, code_challenge, code_challenge_method, created_at, expires_at`, id)
	return scanFlow(row)
}

// PurgeExpired removes expired flows (janitor).
func (r *FlowRepo) PurgeExpired(ctx context.Context) (int64, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM portal_login_flows WHERE expires_at <= NOW()`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func scanFlow(row pgx.Row) (*LoginFlow, error) {
	var f LoginFlow
	if err := row.Scan(&f.ID, &f.OAuthClientID, &f.PortalClientID, &f.RedirectURI,
		&f.Scope, &f.State, &f.Nonce, &f.CodeChallenge, &f.CodeChallengeMethod,
		&f.CreatedAt, &f.ExpiresAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("portal_login_flows scan: %w", err)
	}
	return &f, nil
}
