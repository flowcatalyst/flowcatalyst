// Package grantstore is a 1:1 port of the Rust authorization-code and
// refresh-token repositories (auth/authorization_code*.rs,
// auth/refresh_token*.rs). Both artifact kinds are persisted in the
// shared oauth_oidc_payloads table — keyed by a composite id
// ("AuthorizationCode:{code}" / "RefreshToken:{id}"), discriminated by
// the `type` column, with a camelCase JSONB payload — for byte-level
// storage compatibility with the TypeScript/Rust providers during
// cutover.
//
// These are infrastructure rows, not domain aggregates: they bypass the
// UoW and are accessed via raw pgx (per docs/conventions.md §3 and the
// PLAN's "raw pgx for JSONB-heavy queries" rule).
package grantstore

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

const (
	authCodePayloadType     = "AuthorizationCode"
	refreshTokenPayloadType = "RefreshToken"

	authCodeDefaultExpiry     = 10 * time.Minute
	refreshTokenDefaultExpiry = 30 * 24 * time.Hour
)

// ─── Authorization Code ─────────────────────────────────────────────────

// AuthorizationCode is an OAuth2 authorization code for the
// authorization-code flow. Codes are short-lived, single-use, and bound
// to PKCE. Mirrors the Rust AuthorizationCode struct.
type AuthorizationCode struct {
	Code                string
	ClientID            string
	PrincipalID         string
	RedirectURI         string
	Scope               *string
	CodeChallenge       *string
	CodeChallengeMethod *string
	Nonce               *string
	State               *string
	ContextClientID     *string
	CreatedAt           time.Time
	ExpiresAt           time.Time
	Used                bool
}

// NewAuthorizationCode builds a code with the default 10-minute expiry.
func NewAuthorizationCode(code, clientID, principalID, redirectURI string) *AuthorizationCode {
	now := time.Now().UTC()
	return &AuthorizationCode{
		Code:        code,
		ClientID:    clientID,
		PrincipalID: principalID,
		RedirectURI: redirectURI,
		CreatedAt:   now,
		ExpiresAt:   now.Add(authCodeDefaultExpiry),
	}
}

// IsExpired reports whether the code's expiry has passed.
func (c *AuthorizationCode) IsExpired() bool { return time.Now().UTC().After(c.ExpiresAt) }

// IsValid reports whether the code is neither used nor expired.
func (c *AuthorizationCode) IsValid() bool { return !c.Used && !c.IsExpired() }

// authCodePayload is the camelCase JSONB stored under the row's payload
// column, matching Rust's AuthorizationCodeRepository::to_payload.
type authCodePayload struct {
	AccountID           string  `json:"accountId"`
	ClientID            string  `json:"clientId"`
	RedirectURI         string  `json:"redirectUri"`
	Scope               *string `json:"scope"`
	CodeChallenge       *string `json:"codeChallenge"`
	CodeChallengeMethod *string `json:"codeChallengeMethod"`
	Nonce               *string `json:"nonce"`
	State               *string `json:"state"`
	ContextClientID     *string `json:"contextClientId"`
	Kind                string  `json:"kind"`
	IAT                 int64   `json:"iat"`
	EXP                 int64   `json:"exp"`
}

// AuthorizationCodeRepository persists authorization codes in
// oauth_oidc_payloads (type = "AuthorizationCode").
type AuthorizationCodeRepository struct{ pool *pgxpool.Pool }

// NewAuthorizationCodeRepository wires the repo against pool.
func NewAuthorizationCodeRepository(pool *pgxpool.Pool) *AuthorizationCodeRepository {
	return &AuthorizationCodeRepository{pool: pool}
}

func authCodeID(code string) string { return authCodePayloadType + ":" + code }

// Insert writes a new authorization code (upserting payload + expiry on
// id conflict).
func (r *AuthorizationCodeRepository) Insert(ctx context.Context, c *AuthorizationCode) error {
	payload, err := json.Marshal(authCodePayload{
		AccountID:           c.PrincipalID,
		ClientID:            c.ClientID,
		RedirectURI:         c.RedirectURI,
		Scope:               c.Scope,
		CodeChallenge:       c.CodeChallenge,
		CodeChallengeMethod: c.CodeChallengeMethod,
		Nonce:               c.Nonce,
		State:               c.State,
		ContextClientID:     c.ContextClientID,
		Kind:                authCodePayloadType,
		IAT:                 c.CreatedAt.Unix(),
		EXP:                 c.ExpiresAt.Unix(),
	})
	if err != nil {
		return fmt.Errorf("marshal auth-code payload: %w", err)
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO oauth_oidc_payloads
			(id, type, payload, grant_id, user_code, uid, expires_at, consumed_at, created_at)
		VALUES ($1, $2, $3, NULL, NULL, NULL, $4, NULL, $5)
		ON CONFLICT (id) DO UPDATE SET payload = $3, expires_at = $4`,
		authCodeID(c.Code), authCodePayloadType, payload, c.ExpiresAt, c.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert auth code: %w", err)
	}
	return nil
}

// FindAndConsume atomically marks a still-valid code consumed and returns
// it, preventing two concurrent token requests from both spending the
// same code. Returns (nil, nil) when the code is missing, expired, or
// already consumed.
func (r *AuthorizationCodeRepository) FindAndConsume(ctx context.Context, code string) (*AuthorizationCode, error) {
	row := r.pool.QueryRow(ctx,
		`UPDATE oauth_oidc_payloads
		SET consumed_at = NOW()
		WHERE id = $1 AND consumed_at IS NULL AND expires_at > NOW()
		RETURNING id, payload, expires_at, consumed_at, created_at`,
		authCodeID(code))
	return scanAuthCode(row)
}

// FindByCode loads a code regardless of consumed/expired state.
func (r *AuthorizationCodeRepository) FindByCode(ctx context.Context, code string) (*AuthorizationCode, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, payload, expires_at, consumed_at, created_at
		FROM oauth_oidc_payloads WHERE id = $1`,
		authCodeID(code))
	return scanAuthCode(row)
}

// DeleteExpired removes expired authorization-code rows. Returns the
// number deleted.
func (r *AuthorizationCodeRepository) DeleteExpired(ctx context.Context) (int64, error) {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM oauth_oidc_payloads WHERE type = $1 AND expires_at < NOW()`,
		authCodePayloadType)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func scanAuthCode(row pgx.Row) (*AuthorizationCode, error) {
	var (
		id         string
		payload    []byte
		expiresAt  *time.Time
		consumedAt *time.Time
		createdAt  time.Time
	)
	if err := row.Scan(&id, &payload, &expiresAt, &consumedAt, &createdAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("scan auth code: %w", err)
	}
	var p authCodePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("unmarshal auth-code payload: %w", err)
	}
	exp := createdAt.Add(authCodeDefaultExpiry)
	if expiresAt != nil {
		exp = *expiresAt
	}
	return &AuthorizationCode{
		Code:                strings.TrimPrefix(id, authCodePayloadType+":"),
		ClientID:            p.ClientID,
		PrincipalID:         p.AccountID,
		RedirectURI:         p.RedirectURI,
		Scope:               p.Scope,
		CodeChallenge:       p.CodeChallenge,
		CodeChallengeMethod: p.CodeChallengeMethod,
		Nonce:               p.Nonce,
		State:               p.State,
		ContextClientID:     p.ContextClientID,
		CreatedAt:           createdAt,
		ExpiresAt:           exp,
		Used:                consumedAt != nil,
	}, nil
}

// ─── Refresh Token ──────────────────────────────────────────────────────

// RefreshToken is a long-lived token used to renew access tokens. Only
// the hash is stored; the raw token is returned to the client once.
// Mirrors the Rust RefreshToken struct.
type RefreshToken struct {
	ID                string
	TokenHash         string
	PrincipalID       string
	OAuthClientID     *string
	Scopes            []string
	AccessibleClients []string
	Revoked           bool
	RevokedAt         *time.Time
	TokenFamily       *string
	ReplacedBy        *string
	CreatedAt         time.Time
	ExpiresAt         time.Time
	LastUsedAt        *time.Time
	CreatedFromIP     *string
	UserAgent         *string
}

// NewRefreshToken builds a token entity from a precomputed hash.
func NewRefreshToken(tokenHash, principalID string) *RefreshToken {
	now := time.Now().UTC()
	return &RefreshToken{
		ID:          tsid.GenerateUntyped(),
		TokenHash:   tokenHash,
		PrincipalID: principalID,
		CreatedAt:   now,
		ExpiresAt:   now.Add(refreshTokenDefaultExpiry),
	}
}

// GenerateRawToken returns a base64url (no-pad) 32-byte random token.
func GenerateRawToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// HashToken returns base64url (no-pad) SHA-256 of the raw token, matching
// Rust's RefreshToken::hash_token.
func HashToken(rawToken string) string {
	h := sha256.Sum256([]byte(rawToken))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// GenerateTokenPair returns (rawToken, entity) for a fresh refresh token.
func GenerateTokenPair(principalID string) (string, *RefreshToken, error) {
	raw, err := GenerateRawToken()
	if err != nil {
		return "", nil, err
	}
	return raw, NewRefreshToken(HashToken(raw), principalID), nil
}

// IsValid reports whether the token is neither revoked nor expired.
func (t *RefreshToken) IsValid() bool {
	return !t.Revoked && time.Now().UTC().Before(t.ExpiresAt)
}

// WasReplaced reports whether the token was rotated out (replaced_by set).
func (t *RefreshToken) WasReplaced() bool { return t.ReplacedBy != nil }

// refreshTokenPayload is the camelCase JSONB stored under payload,
// matching Rust's RefreshTokenRepository::to_payload.
type refreshTokenPayload struct {
	AccountID         string   `json:"accountId"`
	ClientID          *string  `json:"clientId"`
	TokenHash         string   `json:"tokenHash"`
	Scope             string   `json:"scope"`
	AccessibleClients []string `json:"accessibleClients"`
	Revoked           bool     `json:"revoked"`
	RevokedAt         *string  `json:"revokedAt"`
	TokenFamily       *string  `json:"tokenFamily"`
	ReplacedBy        *string  `json:"replacedBy"`
	LastUsedAt        *string  `json:"lastUsedAt"`
	CreatedFromIP     *string  `json:"createdFromIp"`
	UserAgent         *string  `json:"userAgent"`
	IAT               int64    `json:"iat"`
	EXP               int64    `json:"exp"`
	Kind              string   `json:"kind"`
}

// RefreshTokenRepository persists refresh tokens in oauth_oidc_payloads
// (type = "RefreshToken").
type RefreshTokenRepository struct{ pool *pgxpool.Pool }

// NewRefreshTokenRepository wires the repo against pool.
func NewRefreshTokenRepository(pool *pgxpool.Pool) *RefreshTokenRepository {
	return &RefreshTokenRepository{pool: pool}
}

func refreshTokenID(id string) string { return refreshTokenPayloadType + ":" + id }

// Insert writes a new refresh token. The token_family is mirrored into
// the grant_id column so a whole rotation family can be revoked at once.
func (r *RefreshTokenRepository) Insert(ctx context.Context, t *RefreshToken) error {
	payload, err := json.Marshal(refreshTokenPayload{
		AccountID:         t.PrincipalID,
		ClientID:          t.OAuthClientID,
		TokenHash:         t.TokenHash,
		Scope:             strings.Join(t.Scopes, " "),
		AccessibleClients: t.AccessibleClients,
		Revoked:           t.Revoked,
		RevokedAt:         rfc3339Ptr(t.RevokedAt),
		TokenFamily:       t.TokenFamily,
		ReplacedBy:        t.ReplacedBy,
		LastUsedAt:        rfc3339Ptr(t.LastUsedAt),
		CreatedFromIP:     t.CreatedFromIP,
		UserAgent:         t.UserAgent,
		IAT:               t.CreatedAt.Unix(),
		EXP:               t.ExpiresAt.Unix(),
		Kind:              refreshTokenPayloadType,
	})
	if err != nil {
		return fmt.Errorf("marshal refresh-token payload: %w", err)
	}
	if t.AccessibleClients == nil {
		// Match Rust: accessibleClients always an array, never null.
		payload, _ = injectEmptyArray(payload, "accessibleClients")
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO oauth_oidc_payloads
			(id, type, payload, grant_id, user_code, uid, expires_at, consumed_at, created_at)
		VALUES ($1, $2, $3, $4, NULL, NULL, $5, NULL, $6)
		ON CONFLICT (id) DO UPDATE SET payload = $3, grant_id = $4, expires_at = $5`,
		refreshTokenID(t.ID), refreshTokenPayloadType, payload, t.TokenFamily, t.ExpiresAt, t.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert refresh token: %w", err)
	}
	return nil
}

// FindByHash loads a refresh token by its hash regardless of state.
func (r *RefreshTokenRepository) FindByHash(ctx context.Context, tokenHash string) (*RefreshToken, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, payload, expires_at, created_at FROM oauth_oidc_payloads
		WHERE type = $1 AND payload->>'tokenHash' = $2`,
		refreshTokenPayloadType, tokenHash)
	return scanRefreshToken(row)
}

// FindValidByHash loads a non-expired, non-consumed, non-revoked refresh
// token by its hash. Returns (nil, nil) when none matches.
func (r *RefreshTokenRepository) FindValidByHash(ctx context.Context, tokenHash string) (*RefreshToken, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, payload, expires_at, created_at FROM oauth_oidc_payloads
		WHERE type = $1 AND payload->>'tokenHash' = $2
		  AND expires_at > NOW() AND consumed_at IS NULL`,
		refreshTokenPayloadType, tokenHash)
	t, err := scanRefreshToken(row)
	if err != nil || t == nil {
		return nil, err
	}
	if t.Revoked {
		return nil, nil
	}
	return t, nil
}

// MarkAsReplaced records the hash of the token that replaced this one
// (rotation; a later reuse of the replaced token signals an attack).
func (r *RefreshTokenRepository) MarkAsReplaced(ctx context.Context, tokenHash, newTokenHash string) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE oauth_oidc_payloads
		SET payload = jsonb_set(payload, '{replacedBy}', to_jsonb($3::text))
		WHERE type = $1 AND payload->>'tokenHash' = $2`,
		refreshTokenPayloadType, tokenHash, newTokenHash)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// RevokeByHash revokes a single refresh token by its hash.
func (r *RefreshTokenRepository) RevokeByHash(ctx context.Context, tokenHash string) (bool, error) {
	now := time.Now().UTC()
	tag, err := r.pool.Exec(ctx,
		`UPDATE oauth_oidc_payloads
		SET payload = jsonb_set(
			jsonb_set(payload, '{revoked}', 'true'::jsonb),
			'{revokedAt}', to_jsonb($3::text)
		), consumed_at = $3
		WHERE type = $1 AND payload->>'tokenHash' = $2`,
		refreshTokenPayloadType, tokenHash, now)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// RevokeAllInFamily revokes every still-active token in a rotation family
// (by grant_id). Returns the number revoked.
func (r *RefreshTokenRepository) RevokeAllInFamily(ctx context.Context, familyID string) (int64, error) {
	now := time.Now().UTC()
	tag, err := r.pool.Exec(ctx,
		`UPDATE oauth_oidc_payloads
		SET payload = jsonb_set(
			jsonb_set(payload, '{revoked}', 'true'::jsonb),
			'{revokedAt}', to_jsonb($3::text)
		), consumed_at = $3
		WHERE type = $1 AND grant_id = $2
		  AND (payload->>'revoked' IS NULL OR payload->>'revoked' = 'false')`,
		refreshTokenPayloadType, familyID, now)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// RevokeAllForPrincipal revokes every active refresh token for a
// principal (logout-all). Returns the number revoked.
func (r *RefreshTokenRepository) RevokeAllForPrincipal(ctx context.Context, principalID string) (int64, error) {
	now := time.Now().UTC()
	tag, err := r.pool.Exec(ctx,
		`UPDATE oauth_oidc_payloads
		SET payload = jsonb_set(
			jsonb_set(payload, '{revoked}', 'true'::jsonb),
			'{revokedAt}', to_jsonb($3::text)
		), consumed_at = $3
		WHERE type = $1 AND payload->>'accountId' = $2
		  AND consumed_at IS NULL AND expires_at > NOW()
		  AND (payload->>'revoked' IS NULL OR payload->>'revoked' = 'false')`,
		refreshTokenPayloadType, principalID, now)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// UpdateLastUsed stamps the lastUsedAt field for a token by id.
func (r *RefreshTokenRepository) UpdateLastUsed(ctx context.Context, id string) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE oauth_oidc_payloads
		SET payload = jsonb_set(payload, '{lastUsedAt}', to_jsonb($2::text))
		WHERE id = $1`,
		refreshTokenID(id), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// DeleteExpired removes expired refresh-token rows. Returns the number
// deleted.
func (r *RefreshTokenRepository) DeleteExpired(ctx context.Context) (int64, error) {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM oauth_oidc_payloads WHERE type = $1 AND expires_at < NOW()`,
		refreshTokenPayloadType)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func scanRefreshToken(row pgx.Row) (*RefreshToken, error) {
	var (
		id        string
		payload   []byte
		expiresAt *time.Time
		createdAt time.Time
	)
	if err := row.Scan(&id, &payload, &expiresAt, &createdAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("scan refresh token: %w", err)
	}
	var p refreshTokenPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("unmarshal refresh-token payload: %w", err)
	}
	exp := createdAt.Add(refreshTokenDefaultExpiry)
	if expiresAt != nil {
		exp = *expiresAt
	}
	scopes := []string{}
	if p.Scope != "" {
		scopes = strings.Fields(p.Scope)
	}
	return &RefreshToken{
		ID:                strings.TrimPrefix(id, refreshTokenPayloadType+":"),
		TokenHash:         p.TokenHash,
		PrincipalID:       p.AccountID,
		OAuthClientID:     p.ClientID,
		Scopes:            scopes,
		AccessibleClients: p.AccessibleClients,
		Revoked:           p.Revoked,
		RevokedAt:         parseRFC3339Ptr(p.RevokedAt),
		TokenFamily:       p.TokenFamily,
		ReplacedBy:        p.ReplacedBy,
		CreatedAt:         createdAt,
		ExpiresAt:         exp,
		LastUsedAt:        parseRFC3339Ptr(p.LastUsedAt),
		CreatedFromIP:     p.CreatedFromIP,
		UserAgent:         p.UserAgent,
	}, nil
}

// ─── helpers ────────────────────────────────────────────────────────────

func rfc3339Ptr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(time.RFC3339Nano)
	return &s
}

func parseRFC3339Ptr(s *string) *time.Time {
	if s == nil || *s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, *s)
	if err != nil {
		return nil
	}
	t = t.UTC()
	return &t
}

// injectEmptyArray ensures key holds [] rather than null in the payload
// blob (matches Rust serializing an empty Vec as []).
func injectEmptyArray(payload []byte, key string) ([]byte, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(payload, &m); err != nil {
		return payload, err
	}
	m[key] = json.RawMessage("[]")
	return json.Marshal(m)
}
