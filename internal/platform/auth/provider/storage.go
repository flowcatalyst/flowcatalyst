package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"time"

	"github.com/ory/fosite"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/payload"
)

// Storage implements fosite.Storage + oauth2.CoreStorage +
// oauth2.TokenRevocationStorage + oauth2.ClientCredentialsGrantStorage,
// backed by oauth_oidc_payloads.
//
// We serialize Requester instances to JSON via storedRequest below. On
// read, the client field is re-resolved through ClientManager.GetClient
// so we never persist hashed secrets twice (and so client mutations
// take effect on the next token refresh).
type Storage struct {
	*ClientManager
	payloads *payload.Repository
	clients  *auth.OAuthClientRepo
}

// NewStorage wires the adapter against the supplied repos.
func NewStorage(clients *auth.OAuthClientRepo, payloads *payload.Repository) *Storage {
	return &Storage{
		ClientManager: NewClientManager(clients, payloads),
		payloads:      payloads,
		clients:       clients,
	}
}

// storedRequest is the JSON shape we persist under payload.Payload.Data.
// We deliberately keep Client out of the serialized form — it's hydrated
// from the OAuth-client repo at read time.
type storedRequest struct {
	ID                string          `json:"id"`
	RequestedAt       time.Time       `json:"requested_at"`
	ClientID          string          `json:"client_id"`
	RequestedScope    []string        `json:"requested_scope"`
	GrantedScope      []string        `json:"granted_scope"`
	RequestedAudience []string        `json:"requested_audience"`
	GrantedAudience   []string        `json:"granted_audience"`
	Form              url.Values      `json:"form"`
	Session           json.RawMessage `json:"session"`
}

func toStored(req fosite.Requester) ([]byte, error) {
	sess, err := json.Marshal(req.GetSession())
	if err != nil {
		return nil, err
	}
	sr := storedRequest{
		ID:                req.GetID(),
		RequestedAt:       req.GetRequestedAt(),
		ClientID:          req.GetClient().GetID(),
		RequestedScope:    []string(req.GetRequestedScopes()),
		GrantedScope:      []string(req.GetGrantedScopes()),
		RequestedAudience: []string(req.GetRequestedAudience()),
		GrantedAudience:   []string(req.GetGrantedAudience()),
		Form:              req.GetRequestForm(),
		Session:           sess,
	}
	return json.Marshal(sr)
}

func (s *Storage) fromStored(ctx context.Context, data []byte, session fosite.Session) (fosite.Requester, error) {
	var sr storedRequest
	if err := json.Unmarshal(data, &sr); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(sr.Session, session); err != nil {
		return nil, err
	}
	c, err := s.GetClient(ctx, sr.ClientID)
	if err != nil {
		return nil, err
	}
	r := &fosite.Request{
		ID:                sr.ID,
		RequestedAt:       sr.RequestedAt,
		Client:            c,
		RequestedScope:    fosite.Arguments(sr.RequestedScope),
		GrantedScope:      fosite.Arguments(sr.GrantedScope),
		RequestedAudience: fosite.Arguments(sr.RequestedAudience),
		GrantedAudience:   fosite.Arguments(sr.GrantedAudience),
		Form:              sr.Form,
		Session:           session,
	}
	return r, nil
}

// ── AccessTokenStorage ───────────────────────────────────────────────────

func (s *Storage) CreateAccessTokenSession(ctx context.Context, signature string, request fosite.Requester) error {
	data, err := toStored(request)
	if err != nil {
		return err
	}
	exp := request.GetSession().GetExpiresAt(fosite.AccessToken)
	return s.payloads.Insert(ctx, &payload.Payload{
		ID:        signature,
		Type:      payload.TypeAccessToken,
		Data:      data,
		GrantID:   strPtr(request.GetID()),
		ExpiresAt: nonZero(exp),
	})
}

func (s *Storage) GetAccessTokenSession(ctx context.Context, signature string, session fosite.Session) (fosite.Requester, error) {
	return s.findRequester(ctx, payload.TypeAccessToken, signature, session, fosite.ErrNotFound)
}

func (s *Storage) DeleteAccessTokenSession(ctx context.Context, signature string) error {
	return s.payloads.Delete(ctx, signature)
}

// ── RefreshTokenStorage ──────────────────────────────────────────────────

func (s *Storage) CreateRefreshTokenSession(ctx context.Context, signature string, _ string, request fosite.Requester) error {
	data, err := toStored(request)
	if err != nil {
		return err
	}
	exp := request.GetSession().GetExpiresAt(fosite.RefreshToken)
	return s.payloads.Insert(ctx, &payload.Payload{
		ID:        signature,
		Type:      payload.TypeRefreshToken,
		Data:      data,
		GrantID:   strPtr(request.GetID()),
		ExpiresAt: nonZero(exp),
	})
}

func (s *Storage) GetRefreshTokenSession(ctx context.Context, signature string, session fosite.Session) (fosite.Requester, error) {
	return s.findRequester(ctx, payload.TypeRefreshToken, signature, session, fosite.ErrNotFound)
}

func (s *Storage) DeleteRefreshTokenSession(ctx context.Context, signature string) error {
	return s.payloads.Delete(ctx, signature)
}

// RotateRefreshToken is the hook fosite calls when a refresh-token
// rotation happens (single-use refresh tokens, OAuth 2.1 default). We
// mark the old token consumed; the new one is created via
// CreateRefreshTokenSession. We don't proactively delete the old row —
// keeping consumed_at lets us detect reuse on the next refresh.
func (s *Storage) RotateRefreshToken(ctx context.Context, _ string, refreshTokenSignature string) error {
	return s.payloads.MarkConsumed(ctx, refreshTokenSignature)
}

// ── AuthorizeCodeStorage ─────────────────────────────────────────────────

func (s *Storage) CreateAuthorizeCodeSession(ctx context.Context, code string, request fosite.Requester) error {
	data, err := toStored(request)
	if err != nil {
		return err
	}
	exp := request.GetSession().GetExpiresAt(fosite.AuthorizeCode)
	return s.payloads.Insert(ctx, &payload.Payload{
		ID:        code,
		Type:      payload.TypeAuthorizationCode,
		Data:      data,
		GrantID:   strPtr(request.GetID()),
		ExpiresAt: nonZero(exp),
	})
}

func (s *Storage) GetAuthorizeCodeSession(ctx context.Context, code string, session fosite.Session) (fosite.Requester, error) {
	p, err := s.payloads.FindByID(ctx, payload.TypeAuthorizationCode, code)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, fosite.ErrNotFound
	}
	req, err := s.fromStored(ctx, p.Data, session)
	if err != nil {
		return nil, err
	}
	if p.ConsumedAt != nil {
		// Spec says: return the request AND ErrInvalidatedAuthorizeCode so
		// fosite can short-circuit the token-revocation cascade.
		return req, fosite.ErrInvalidatedAuthorizeCode
	}
	return req, nil
}

func (s *Storage) InvalidateAuthorizeCodeSession(ctx context.Context, code string) error {
	return s.payloads.MarkConsumed(ctx, code)
}

// ── PKCERequestStorage ───────────────────────────────────────────────────

func (s *Storage) CreatePKCERequestSession(ctx context.Context, signature string, request fosite.Requester) error {
	data, err := toStored(request)
	if err != nil {
		return err
	}
	return s.payloads.Insert(ctx, &payload.Payload{
		ID:      signature,
		Type:    payload.TypePKCESession,
		Data:    data,
		GrantID: strPtr(request.GetID()),
	})
}

func (s *Storage) GetPKCERequestSession(ctx context.Context, signature string, session fosite.Session) (fosite.Requester, error) {
	return s.findRequester(ctx, payload.TypePKCESession, signature, session, fosite.ErrNotFound)
}

func (s *Storage) DeletePKCERequestSession(ctx context.Context, signature string) error {
	return s.payloads.Delete(ctx, signature)
}

// ── TokenRevocationStorage ───────────────────────────────────────────────

// RevokeAccessToken / RevokeRefreshToken receive a request_id, which we
// stored as grant_id. Drop every payload row tied to the grant.
func (s *Storage) RevokeAccessToken(ctx context.Context, requestID string) error {
	return s.payloads.DeleteByGrant(ctx, requestID)
}

func (s *Storage) RevokeRefreshToken(ctx context.Context, requestID string) error {
	return s.payloads.DeleteByGrant(ctx, requestID)
}

// ── helpers ──────────────────────────────────────────────────────────────

func (s *Storage) findRequester(ctx context.Context, t payload.Type, signature string, session fosite.Session, notFound error) (fosite.Requester, error) {
	p, err := s.payloads.FindByID(ctx, t, signature)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, notFound
	}
	return s.fromStored(ctx, p.Data, session)
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nonZero(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// Compile-time interface assertions so any drift in the fosite
// interfaces surfaces immediately at compile time.
var (
	_ fosite.Storage = (*Storage)(nil)
	_                = errors.Is // keep errors import — fosite Errs are sentinels we'll wrap
)
