package authservice

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/envutil"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// Field-worker access tokens (docs/field-worker-identity-p1-spec.md §4.3).
const (
	// TokenUseField is the token_use of a field access token. The platform API
	// middleware's allowlist refuses it as an API bearer.
	TokenUseField = "field"
	// FieldTokenType is the JOSE `typ` header of a field access token.
	FieldTokenType = "fc-field+jwt"
	// DefaultFieldAccessTokenTTLSecs is the default field access-token life.
	DefaultFieldAccessTokenTTLSecs = 600
)

// FieldAccessTokenTTLSecs is the field access-token lifetime
// (FC_FIELD_ACCESS_TOKEN_TTL_SECS, default 600).
func FieldAccessTokenTTLSecs() int64 {
	if v := envutil.Int("FC_FIELD_ACCESS_TOKEN_TTL_SECS", DefaultFieldAccessTokenTTLSecs); v > 0 {
		return int64(v)
	}
	return DefaultFieldAccessTokenTTLSecs
}

// FieldSubject is a field worker signed in on a device, as a token subject. It
// is not a principal: it carries no roles, scope, applications or clients.
type FieldSubject struct {
	WorkerID         string
	ClientID         string
	ClientIdentifier string
	WorkerCode       string
	Name             string
	// AppCode is the application the token is for (the aud claim).
	AppCode string
	// DeviceID is empty on an unenrolled key, where DeviceMode is "UNENROLLED"
	// and the device_id claim is omitted.
	DeviceID   string
	DeviceMode string
	// SessionID is the login family (the sid claim).
	SessionID string
	AMR       []string
	AuthTime  time.Time
	// JKT is the RFC 7638 thumbprint of the device key the token is bound to.
	JKT string
	// PINChangeRequired marks the token pcr: usable only on /field/auth/pin.
	PINChangeRequired bool
	// ActorID, ActorType ("field_worker" or "principal") and ActorName make the
	// act claim of an impersonated token; ActorID empty = no act claim.
	ActorID   string
	ActorType string
	ActorName string
	// Test marks a test identity (the test claim).
	Test bool
}

// FieldAccessTokenClaims is the wire shape of a field access token.
type FieldAccessTokenClaims struct {
	jwt.RegisteredClaims

	// Aud is the application code (a bare string).
	Aud string `json:"aud"`

	TokenUse         string   `json:"token_use"`
	ClientID         string   `json:"client_id"`
	ClientIdentifier string   `json:"client_identifier"`
	WorkerCode       string   `json:"worker_code"`
	Name             string   `json:"name"`
	DeviceID         string   `json:"device_id,omitempty"`
	DeviceMode       string   `json:"device_mode"`
	SessionID        string   `json:"sid"`
	AMR              []string `json:"amr"`
	AuthTime         int64    `json:"auth_time"`
	Cnf              FieldCnf `json:"cnf"`
	PCR              bool     `json:"pcr,omitempty"`
	// Act is the RFC 8693 actor of an impersonated token: someone other than
	// the subject is signed in as them. Test marks a test identity.
	Act  *FieldAct `json:"act,omitempty"`
	Test bool      `json:"test,omitempty"`
}

// FieldAct is the act claim: who is acting as the worker. Typ is
// "field_worker" (a supervisor) or "principal" (a platform user).
type FieldAct struct {
	Sub  string `json:"sub"`
	Typ  string `json:"typ"`
	Name string `json:"name,omitempty"`
}

// FieldCnf is the RFC 7800 confirmation claim binding the token to a device key.
type FieldCnf struct {
	JKT string `json:"jkt"`
}

// GenerateFieldAccessToken mints the access token for a field sign-in or
// refresh: JOSE typ fc-field+jwt, signed with the platform key (same kid).
func (s *AuthService) GenerateFieldAccessToken(sub FieldSubject) (string, error) {
	now := time.Now().UTC()
	amr := sub.AMR
	if amr == nil {
		amr = []string{"pin"}
	}
	claims := FieldAccessTokenClaims{
		Issuer:           s.config.Issuer,
		Subject:          sub.WorkerID,
		ExpiresAt:        jwt.NewNumericDate(now.Add(time.Duration(FieldAccessTokenTTLSecs()) * time.Second)),
		IssuedAt:         jwt.NewNumericDate(now),
		NotBefore:        jwt.NewNumericDate(now),
		ID:               tsid.GenerateUntyped(),
		Aud:              sub.AppCode,
		TokenUse:         TokenUseField,
		ClientID:         sub.ClientID,
		ClientIdentifier: sub.ClientIdentifier,
		WorkerCode:       sub.WorkerCode,
		Name:             sub.Name,
		DeviceID:         sub.DeviceID,
		DeviceMode:       sub.DeviceMode,
		SessionID:        sub.SessionID,
		AMR:              amr,
		AuthTime:         sub.AuthTime.Unix(),
		Cnf:              FieldCnf{JKT: sub.JKT},
		PCR:              sub.PINChangeRequired,
		Test:             sub.Test,
	}
	if sub.ActorID != "" {
		claims.Act = &FieldAct{Sub: sub.ActorID, Typ: sub.ActorType, Name: sub.ActorName}
	}
	tok := jwt.NewWithClaims(s.signingMethod, claims)
	tok.Header["typ"] = FieldTokenType
	if s.keyID != "" {
		tok.Header["kid"] = s.keyID
	}
	signed, err := tok.SignedString(s.signKey)
	if err != nil {
		return "", fmt.Errorf("encode JWT: %w", err)
	}
	return signed, nil
}

// ValidateFieldToken verifies a field access token: signature (current key,
// then previous keys), issuer, expiry, JOSE typ fc-field+jwt and token_use
// "field". The audience is the application code and is the caller's to check.
func (s *AuthService) ValidateFieldToken(token string) (*FieldAccessTokenClaims, error) {
	keys := make([]any, 0, 1+len(s.previousKeys))
	keys = append(keys, s.currentVerify)
	for _, k := range s.previousKeys {
		keys = append(keys, k.verifyKey)
	}
	var lastErr error
	for _, vk := range keys {
		claims := &FieldAccessTokenClaims{}
		parsed, err := jwt.ParseWithClaims(token, claims,
			func(*jwt.Token) (any, error) { return vk, nil },
			jwt.WithValidMethods([]string{s.algorithm}),
			jwt.WithIssuer(s.config.Issuer),
		)
		if err == nil {
			if typ, _ := parsed.Header["typ"].(string); typ != FieldTokenType {
				return nil, fmt.Errorf("%w: not a field token", ErrInvalidToken)
			}
			if claims.TokenUse != TokenUseField {
				return nil, fmt.Errorf("%w: not a field token", ErrInvalidToken)
			}
			return claims, nil
		}
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		lastErr = err
	}
	return nil, fmt.Errorf("%w: %w", ErrInvalidToken, lastErr)
}
