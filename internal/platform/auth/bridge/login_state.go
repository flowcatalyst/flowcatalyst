package bridge

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OIDCLoginState backs the oauth_oidc_login_states table — one row per
// in-flight OIDC authorization-code login. The state column is the
// primary key, used both as the CSRF token and the lookup key on the
// callback.
//
// 10-minute TTL (matches the Rust constant); a periodic purge poller
// drops expired rows.
type OIDCLoginState struct {
	State                    string
	EmailDomain              string
	IdentityProviderID       string
	EmailDomainMappingID     string
	Nonce                    string
	CodeVerifier             string
	ReturnURL                *string
	OAuthClientID            *string
	OAuthRedirectURI         *string
	OAuthScope               *string
	OAuthState               *string
	OAuthCodeChallenge       *string
	OAuthCodeChallengeMethod *string
	OAuthNonce               *string
	InteractionUID           *string
	CreatedAt                time.Time
	ExpiresAt                time.Time
}

// IsExpired reports whether ExpiresAt is in the past.
func (s *OIDCLoginState) IsExpired() bool { return time.Now().UTC().After(s.ExpiresAt) }

// NewLoginState builds an OIDCLoginState with the 10-minute default TTL.
func NewLoginState(state, emailDomain, identityProviderID, mappingID, nonce, codeVerifier string) *OIDCLoginState {
	now := time.Now().UTC()
	return &OIDCLoginState{
		State:                state,
		EmailDomain:          strings.ToLower(emailDomain),
		IdentityProviderID:   identityProviderID,
		EmailDomainMappingID: mappingID,
		Nonce:                nonce,
		CodeVerifier:         codeVerifier,
		CreatedAt:            now,
		ExpiresAt:            now.Add(10 * time.Minute),
	}
}

// LoginStateRepo is the pgx-backed repo.
type LoginStateRepo struct{ pool *pgxpool.Pool }

// NewLoginStateRepo wires the repo.
func NewLoginStateRepo(pool *pgxpool.Pool) *LoginStateRepo {
	return &LoginStateRepo{pool: pool}
}

// Insert persists a fresh login state.
func (r *LoginStateRepo) Insert(ctx context.Context, s *OIDCLoginState) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO oauth_oidc_login_states
		     (state, email_domain, identity_provider_id, email_domain_mapping_id,
		      nonce, code_verifier, return_url,
		      oauth_client_id, oauth_redirect_uri, oauth_scope, oauth_state,
		      oauth_code_challenge, oauth_code_challenge_method, oauth_nonce,
		      interaction_uid, created_at, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`,
		s.State, s.EmailDomain, s.IdentityProviderID, s.EmailDomainMappingID,
		s.Nonce, s.CodeVerifier, s.ReturnURL,
		s.OAuthClientID, s.OAuthRedirectURI, s.OAuthScope, s.OAuthState,
		s.OAuthCodeChallenge, s.OAuthCodeChallengeMethod, s.OAuthNonce,
		s.InteractionUID, s.CreatedAt, s.ExpiresAt)
	if err != nil {
		return fmt.Errorf("oauth_oidc_login_states insert: %w", err)
	}
	return nil
}

// FindByState loads + locks the state row by its primary key. Returns
// (nil, nil) when not found.
func (r *LoginStateRepo) FindByState(ctx context.Context, state string) (*OIDCLoginState, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT state, email_domain, identity_provider_id, email_domain_mapping_id,
		        nonce, code_verifier, return_url,
		        oauth_client_id, oauth_redirect_uri, oauth_scope, oauth_state,
		        oauth_code_challenge, oauth_code_challenge_method, oauth_nonce,
		        interaction_uid, created_at, expires_at
		   FROM oauth_oidc_login_states
		  WHERE state = $1`, state)
	var s OIDCLoginState
	if err := row.Scan(&s.State, &s.EmailDomain, &s.IdentityProviderID, &s.EmailDomainMappingID,
		&s.Nonce, &s.CodeVerifier, &s.ReturnURL,
		&s.OAuthClientID, &s.OAuthRedirectURI, &s.OAuthScope, &s.OAuthState,
		&s.OAuthCodeChallenge, &s.OAuthCodeChallengeMethod, &s.OAuthNonce,
		&s.InteractionUID, &s.CreatedAt, &s.ExpiresAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("oauth_oidc_login_states lookup: %w", err)
	}
	return &s, nil
}

// Consume atomically deletes and returns the state row, but only if it exists
// and has not expired. This is single-use / replay-proof: a second concurrent
// (or later) callback for the same state gets (nil, nil) because the row is
// gone. Mirrors Rust's find_and_consume_state (DELETE … WHERE expires_at > NOW()
// RETURNING …) and replaces the non-atomic FindByState + IsExpired + Delete
// dance, which left the row replayable on every error path.
func (r *LoginStateRepo) Consume(ctx context.Context, state string) (*OIDCLoginState, error) {
	row := r.pool.QueryRow(ctx,
		`DELETE FROM oauth_oidc_login_states
		  WHERE state = $1 AND expires_at > NOW()
		 RETURNING state, email_domain, identity_provider_id, email_domain_mapping_id,
		           nonce, code_verifier, return_url,
		           oauth_client_id, oauth_redirect_uri, oauth_scope, oauth_state,
		           oauth_code_challenge, oauth_code_challenge_method, oauth_nonce,
		           interaction_uid, created_at, expires_at`, state)
	var s OIDCLoginState
	if err := row.Scan(&s.State, &s.EmailDomain, &s.IdentityProviderID, &s.EmailDomainMappingID,
		&s.Nonce, &s.CodeVerifier, &s.ReturnURL,
		&s.OAuthClientID, &s.OAuthRedirectURI, &s.OAuthScope, &s.OAuthState,
		&s.OAuthCodeChallenge, &s.OAuthCodeChallengeMethod, &s.OAuthNonce,
		&s.InteractionUID, &s.CreatedAt, &s.ExpiresAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("oauth_oidc_login_states consume: %w", err)
	}
	return &s, nil
}

// Delete removes a state row. Called after the callback exchange so the
// state isn't replayable.
func (r *LoginStateRepo) Delete(ctx context.Context, state string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM oauth_oidc_login_states WHERE state = $1`, state)
	return err
}

// PurgeExpired drops every row past its expires_at. Run on the periodic
// payload-purge poller alongside oauth_oidc_payloads.
func (r *LoginStateRepo) PurgeExpired(ctx context.Context) (int64, error) {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM oauth_oidc_login_states WHERE expires_at < NOW()`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
