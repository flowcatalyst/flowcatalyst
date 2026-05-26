package provider

import (
	"context"
	"errors"
	"time"

	"github.com/ory/fosite"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/payload"
)

// ClientManager implements fosite.ClientManager (which is also fosite.Storage)
// — the minimal storage surface fosite requires.
//
// GetClient hits oauth_clients. ClientAssertionJWTValid /
// SetClientAssertionJWT use the payload table with type=client_assertion_jti
// to track seen JTIs for replay protection (RFC 7523 §3, used by the
// private_key_jwt token-endpoint auth method).
type ClientManager struct {
	clients  *auth.OAuthClientRepo
	payloads *payload.Repository
}

// NewClientManager wires the ClientManager.
func NewClientManager(clients *auth.OAuthClientRepo, payloads *payload.Repository) *ClientManager {
	return &ClientManager{clients: clients, payloads: payloads}
}

// GetClient implements fosite.ClientManager.GetClient.
func (m *ClientManager) GetClient(ctx context.Context, id string) (fosite.Client, error) {
	c, err := m.clients.FindByClientID(ctx, id)
	if err != nil {
		return nil, err
	}
	if c == nil || !c.Active {
		return nil, fosite.ErrNotFound
	}
	return AdaptClient(c), nil
}

// ClientAssertionJWTValid returns nil if the JTI has NOT been seen,
// fosite.ErrJTIKnown if it has, or another error on lookup failure.
func (m *ClientManager) ClientAssertionJWTValid(ctx context.Context, jti string) error {
	p, err := m.payloads.FindByID(ctx, payload.Type("client_assertion_jti"), jti)
	if err != nil {
		return err
	}
	if p != nil {
		// Still within validity? Reject — replay.
		if p.ExpiresAt != nil && p.ExpiresAt.After(time.Now().UTC()) {
			return fosite.ErrJTIKnown
		}
	}
	return nil
}

// SetClientAssertionJWT records the JTI as seen until exp.
func (m *ClientManager) SetClientAssertionJWT(ctx context.Context, jti string, exp time.Time) error {
	// Best-effort: garbage-collect expired JTIs piggybacked on insert via
	// the periodic purge poller. Direct insert here.
	expiresAt := exp.UTC()
	err := m.payloads.Insert(ctx, &payload.Payload{
		ID:        jti,
		Type:      payload.Type("client_assertion_jti"),
		Data:      []byte(`{}`),
		ExpiresAt: &expiresAt,
	})
	// Duplicate JTI insert means a replay — surface as ErrJTIKnown.
	if err != nil && errors.Is(err, errors.New("duplicate")) {
		return fosite.ErrJTIKnown
	}
	return err
}
