// Package payload backs the oauth_oidc_payloads table that holds all
// OIDC artifacts the provider issues: access tokens, refresh tokens,
// authorization codes, client_credentials sessions, etc.
//
// The schema (migration 007) is type+id keyed with a JSONB payload —
// fosite's notion of "one Storage method per artifact type" maps onto
// this as `(type=access_token|refresh_token|authorization_code, id=key)`.
// That keeps the provider library swap-free as long as fosite stays
// agreement-shaped (or we switch off it). See provider/provider.go for
// the rest of the wiring.
//
// All access goes through pgxpool directly — these are infrastructure
// rows (no UoW, per docs/conventions.md §3): they're not domain
// aggregates, they're per-request token-lifecycle state.
package payload

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
)

// Type discriminates payload kinds. The string values match the
// TypeScript reference + Rust port so existing rows are readable as-is.
type Type string

const (
	TypeAccessToken       Type = "access_token"
	TypeRefreshToken      Type = "refresh_token"
	TypeAuthorizationCode Type = "authorization_code"
	TypeClientCredentials Type = "client_credentials"
	TypePKCESession       Type = "pkce_session"
)

// Payload is one artifact row in oauth_oidc_payloads.
type Payload struct {
	ID         string          // primary key — token jti, code, or session id
	Type       Type            // discriminator
	Data       json.RawMessage // typed sub-payload, see AccessTokenData/RefreshTokenData/AuthCodeData
	GrantID    *string         // links access+refresh tokens issued from the same grant
	UserCode   *string         // device-code flow (not used by client_credentials)
	UID        *string         // login-state uid for OIDC bridge correlation
	ExpiresAt  *time.Time
	ConsumedAt *time.Time
	CreatedAt  time.Time
}

// AccessTokenData is the JSON payload for an access_token row.
// Keep this in sync with what the JWT signer emits — we store the same
// claim bag the JWT carries so introspection answers cheaply without
// re-validating signatures.
type AccessTokenData struct {
	Subject      string   `json:"sub"`
	ClientID     string   `json:"client_id"`
	Scope        string   `json:"scope"`
	Clients      []string `json:"clients"`
	Roles        []string `json:"roles"`
	Applications []string `json:"applications"`
	Email        string   `json:"email,omitempty"`
}

// RefreshTokenData is the JSON payload for a refresh_token row.
type RefreshTokenData struct {
	Subject     string   `json:"sub"`
	ClientID    string   `json:"client_id"`
	Scope       string   `json:"scope"`
	Clients     []string `json:"clients"`
	Roles       []string `json:"roles"`
	TokenFamily string   `json:"token_family"`
	ReplacedBy  string   `json:"replaced_by,omitempty"`
	Revoked     bool     `json:"revoked"`
}

// AuthorizationCodeData is the JSON payload for an authorization_code row.
type AuthorizationCodeData struct {
	ClientID            string `json:"client_id"`
	PrincipalID         string `json:"principal_id"`
	RedirectURI         string `json:"redirect_uri"`
	Scope               string `json:"scope"`
	CodeChallenge       string `json:"code_challenge,omitempty"`
	CodeChallengeMethod string `json:"code_challenge_method,omitempty"`
	Nonce               string `json:"nonce,omitempty"`
	State               string `json:"state,omitempty"`
}

// Repository is the pgx-backed payload store.
type Repository struct{ q *dbq.Queries }

// NewRepository wires a Repository against the supplied pool.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{q: dbq.New(pool)}
}

// Insert writes a new payload row. Returns an error on duplicate id —
// callers handle id collisions by minting a fresh id and retrying.
func (r *Repository) Insert(ctx context.Context, p *Payload) error {
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	if err := r.q.OAuthPayloadInsert(ctx, dbq.OAuthPayloadInsertParams{
		ID:         p.ID,
		Type:       string(p.Type),
		Payload:    p.Data,
		GrantID:    p.GrantID,
		UserCode:   p.UserCode,
		Uid:        p.UID,
		ExpiresAt:  p.ExpiresAt,
		ConsumedAt: p.ConsumedAt,
		CreatedAt:  p.CreatedAt,
	}); err != nil {
		return fmt.Errorf("payload insert: %w", err)
	}
	return nil
}

// FindByID loads a payload of the given type. Returns (nil, nil) if not
// found or if the row is the wrong type — callers cannot distinguish
// type-mismatch from missing on purpose to avoid leaking artifact-shape
// metadata across grant types.
func (r *Repository) FindByID(ctx context.Context, t Type, id string) (*Payload, error) {
	row, err := r.q.OAuthPayloadFindByID(ctx, dbq.OAuthPayloadFindByIDParams{
		ID:   id,
		Type: string(t),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("payload find: %w", err)
	}
	return &Payload{
		ID:         row.ID,
		Type:       Type(row.Type),
		Data:       row.Payload,
		GrantID:    row.GrantID,
		UserCode:   row.UserCode,
		UID:        row.Uid,
		ExpiresAt:  row.ExpiresAt,
		ConsumedAt: row.ConsumedAt,
		CreatedAt:  row.CreatedAt,
	}, nil
}

// MarkConsumed sets consumed_at = NOW(). Used by authorization_code single-use.
func (r *Repository) MarkConsumed(ctx context.Context, id string) error {
	return r.q.OAuthPayloadMarkConsumed(ctx, id)
}

// Delete removes a row outright. Used by revocation.
func (r *Repository) Delete(ctx context.Context, id string) error {
	return r.q.OAuthPayloadDelete(ctx, id)
}

// DeleteByGrant removes every row in a grant family. Used when a
// refresh token is revoked — every access token issued under it is
// invalidated, plus any auth-code-tied artifacts.
func (r *Repository) DeleteByGrant(ctx context.Context, grantID string) error {
	return r.q.OAuthPayloadDeleteByGrant(ctx, &grantID)
}

// PurgeExpired removes payloads past their expires_at. Run on a poller
// alongside the dispatch-job stale-recovery loop.
func (r *Repository) PurgeExpired(ctx context.Context) (int64, error) {
	return r.q.OAuthPayloadPurgeExpired(ctx)
}
