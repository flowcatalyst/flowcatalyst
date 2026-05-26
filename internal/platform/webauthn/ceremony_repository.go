package webauthn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
)

// CeremonyRepository persists short-lived WebAuthn ceremony state
// (the challenge + session data) between the begin and finish HTTP calls.
//
// Mirrors fc-platform/src/webauthn/ceremony_repository.rs which uses the
// `oauth_oidc_payloads` table for cross-protocol short-lived state.
// Same table here. `Consume*` uses DELETE ... RETURNING so a successful
// read also marks the state as used in a single round-trip — race-free
// and replay-safe.
type CeremonyRepository struct{ q *dbq.Queries }

// NewCeremonyRepository wires a repo.
func NewCeremonyRepository(pool *pgxpool.Pool) *CeremonyRepository {
	return &CeremonyRepository{q: dbq.New(pool)}
}

const (
	registrationType   = "WebauthnRegistration"
	authenticationType = "WebauthnAuthentication"
	defaultTTL         = 10 * time.Minute
)

func makeID(kind, stateID string) string { return kind + ":" + stateID }

// StoreRegistration persists a registration challenge keyed by stateID.
func (r *CeremonyRepository) StoreRegistration(ctx context.Context, stateID, principalID string, session *webauthn.SessionData, displayName *string) error {
	payload, err := json.Marshal(map[string]any{
		"principalId": principalID,
		"session":     session,
		"displayName": displayName,
	})
	if err != nil {
		return err
	}
	expiresAt := time.Now().Add(defaultTTL).UTC()
	return r.q.WebauthnCeremonyUpsert(ctx, dbq.WebauthnCeremonyUpsertParams{
		ID:        makeID(registrationType, stateID),
		Type:      registrationType,
		Payload:   payload,
		ExpiresAt: &expiresAt,
	})
}

// ConsumedRegistration is the registration state recovered by ConsumeRegistration.
type ConsumedRegistration struct {
	PrincipalID string
	Session     webauthn.SessionData
	DisplayName *string
}

// ConsumeRegistration deletes the state with the supplied stateID and
// returns its contents. Returns nil if the state is absent or expired.
func (r *CeremonyRepository) ConsumeRegistration(ctx context.Context, stateID string) (*ConsumedRegistration, error) {
	payload, err := r.q.WebauthnCeremonyConsume(ctx, makeID(registrationType, stateID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("consume registration: %w", err)
	}
	var data struct {
		PrincipalID string                `json:"principalId"`
		Session     *webauthn.SessionData `json:"session"`
		DisplayName *string               `json:"displayName"`
	}
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, err
	}
	if data.Session == nil {
		return nil, errors.New("ceremony state missing session")
	}
	return &ConsumedRegistration{
		PrincipalID: data.PrincipalID,
		Session:     *data.Session,
		DisplayName: data.DisplayName,
	}, nil
}

// StoreAuthentication persists an authentication challenge.
func (r *CeremonyRepository) StoreAuthentication(ctx context.Context, stateID string, principalID *string, session *webauthn.SessionData) error {
	payload, err := json.Marshal(map[string]any{
		"principalId": principalID,
		"session":     session,
	})
	if err != nil {
		return err
	}
	expiresAt := time.Now().Add(defaultTTL).UTC()
	return r.q.WebauthnCeremonyUpsert(ctx, dbq.WebauthnCeremonyUpsertParams{
		ID:        makeID(authenticationType, stateID),
		Type:      authenticationType,
		Payload:   payload,
		ExpiresAt: &expiresAt,
	})
}

// ConsumedAuthentication is the authentication state recovered.
type ConsumedAuthentication struct {
	PrincipalID *string
	Session     webauthn.SessionData
}

// ConsumeAuthentication deletes + returns the authentication state.
func (r *CeremonyRepository) ConsumeAuthentication(ctx context.Context, stateID string) (*ConsumedAuthentication, error) {
	payload, err := r.q.WebauthnCeremonyConsume(ctx, makeID(authenticationType, stateID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("consume authentication: %w", err)
	}
	var data struct {
		PrincipalID *string               `json:"principalId"`
		Session     *webauthn.SessionData `json:"session"`
	}
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, err
	}
	if data.Session == nil {
		return nil, errors.New("ceremony state missing session")
	}
	return &ConsumedAuthentication{PrincipalID: data.PrincipalID, Session: *data.Session}, nil
}

// PurgeExpired removes expired ceremony rows. Mirrors Rust's purge_expired.
func (r *CeremonyRepository) PurgeExpired(ctx context.Context) (int64, error) {
	return r.q.WebauthnCeremonyPurgeExpired(ctx, dbq.WebauthnCeremonyPurgeExpiredParams{
		Type:   registrationType,
		Type_2: authenticationType,
	})
}
