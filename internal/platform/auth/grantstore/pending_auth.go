package grantstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// 1:1 port of pending_auth_repository.rs. Pending authorization states
// (between /oauth/authorize and the post-login callback) live in
// oauth_oidc_payloads (type = "PendingAuth", id "PendingAuth:{state}"),
// keyed by the OAuth `state` parameter, and expire after 10 minutes.

const (
	pendingAuthPayloadType = "PendingAuth"
	pendingAuthExpiry      = 10 * time.Minute
)

// PendingAuth is the authorization request stashed while the user logs in.
type PendingAuth struct {
	ClientID            string
	RedirectURI         string
	Scope               *string
	CodeChallenge       *string
	CodeChallengeMethod *string
	Nonce               *string
	CreatedAt           time.Time
}

type pendingAuthPayload struct {
	ClientID            string  `json:"clientId"`
	RedirectURI         string  `json:"redirectUri"`
	Scope               *string `json:"scope"`
	CodeChallenge       *string `json:"codeChallenge"`
	CodeChallengeMethod *string `json:"codeChallengeMethod"`
	Nonce               *string `json:"nonce"`
	CreatedAt           string  `json:"createdAt"`
}

// PendingAuthRepository persists pending auth states in oauth_oidc_payloads.
type PendingAuthRepository struct{ pool *pgxpool.Pool }

// NewPendingAuthRepository wires the repo against pool.
func NewPendingAuthRepository(pool *pgxpool.Pool) *PendingAuthRepository {
	return &PendingAuthRepository{pool: pool}
}

func pendingAuthID(state string) string { return pendingAuthPayloadType + ":" + state }

// Insert stores a pending auth state keyed by the state parameter, with a
// 10-minute expiry (upserting on id conflict).
func (r *PendingAuthRepository) Insert(ctx context.Context, stateParam string, p *PendingAuth) error {
	now := time.Now().UTC()
	payload, err := json.Marshal(pendingAuthPayload{
		ClientID:            p.ClientID,
		RedirectURI:         p.RedirectURI,
		Scope:               p.Scope,
		CodeChallenge:       p.CodeChallenge,
		CodeChallengeMethod: p.CodeChallengeMethod,
		Nonce:               p.Nonce,
		CreatedAt:           p.CreatedAt.Format(time.RFC3339Nano),
	})
	if err != nil {
		return fmt.Errorf("marshal pending-auth payload: %w", err)
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO oauth_oidc_payloads (id, type, payload, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO UPDATE SET payload = $3, expires_at = $4`,
		pendingAuthID(stateParam), pendingAuthPayloadType, payload, now.Add(pendingAuthExpiry), now)
	if err != nil {
		return fmt.Errorf("insert pending auth: %w", err)
	}
	return nil
}

// FindAndConsume atomically deletes and returns a still-valid pending auth
// state (single-use). Returns (nil, nil) when missing, expired, or
// already consumed.
func (r *PendingAuthRepository) FindAndConsume(ctx context.Context, stateParam string) (*PendingAuth, error) {
	row := r.pool.QueryRow(ctx,
		`DELETE FROM oauth_oidc_payloads
		WHERE id = $1 AND consumed_at IS NULL AND expires_at > NOW()
		RETURNING payload`,
		pendingAuthID(stateParam))
	var payload []byte
	if err := row.Scan(&payload); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("consume pending auth: %w", err)
	}
	var p pendingAuthPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("unmarshal pending-auth payload: %w", err)
	}
	createdAt := time.Now().UTC()
	if t, err := time.Parse(time.RFC3339, p.CreatedAt); err == nil {
		createdAt = t.UTC()
	}
	return &PendingAuth{
		ClientID:            p.ClientID,
		RedirectURI:         p.RedirectURI,
		Scope:               p.Scope,
		CodeChallenge:       p.CodeChallenge,
		CodeChallengeMethod: p.CodeChallengeMethod,
		Nonce:               p.Nonce,
		CreatedAt:           createdAt,
	}, nil
}

// DeleteExpired removes expired pending-auth rows. Returns the number
// deleted.
func (r *PendingAuthRepository) DeleteExpired(ctx context.Context) (int64, error) {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM oauth_oidc_payloads WHERE type = $1 AND expires_at < NOW()`,
		pendingAuthPayloadType)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
