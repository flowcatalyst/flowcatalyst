package runner

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"

	fcauth "github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/auth"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/webhook"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
)

// Webhook signature headers (the platform's delivery signing).
const (
	headerSignature = webhook.ValidatorSignatureHeader
	headerTimestamp = webhook.ValidatorTimestampHeader
)

// PermVersionInvoke lets a caller invoke an explicit version (/fn/{a}@vN/…).
const PermVersionInvoke = "platform:function:version:invoke"

// TokenVerifier verifies a platform bearer token and returns the caller.
type TokenVerifier interface {
	Verify(ctx context.Context, bearer string) (*abi.Caller, error)
}

var errUnauthenticated = errors.New("unauthenticated")

// jwksVerifier verifies platform access tokens against the platform's JWKS
// (signature, issuer, audience, expiry via the SDK validator) and reads the
// current claim shape itself.
type jwksVerifier struct {
	mu       sync.Mutex
	cfg      control.TokenAuth
	v        *fcauth.TokenValidator
	fallback string // issuer when Desired carries none (dev)
}

func (j *jwksVerifier) configure(a control.TokenAuth) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if a == j.cfg && j.v != nil {
		return
	}
	j.cfg = a
	iss := a.Issuer
	if iss == "" {
		iss = j.fallback
	}
	if iss == "" {
		j.v = nil
		return
	}
	j.v = fcauth.NewTokenValidator(fcauth.TokenValidatorConfig{IssuerURL: iss, Audience: a.Audience})
}

// platformClaims is the access-token payload the platform mints today.
type platformClaims struct {
	Sub             string   `json:"sub"`
	Type            string   `json:"type"`
	Tier            string   `json:"tier"`
	Scope           string   `json:"scope"`
	Clients         []string `json:"clients"`
	Roles           []string `json:"roles"`
	Applications    []string `json:"applications"`
	AllApplications bool     `json:"all_applications"`
	TokenUse        string   `json:"token_use"`
}

func (j *jwksVerifier) Verify(ctx context.Context, bearer string) (*abi.Caller, error) {
	j.mu.Lock()
	v := j.v
	j.mu.Unlock()
	if v == nil {
		return nil, errors.New("token verification is not configured")
	}
	ac, err := v.ValidateBearer(ctx, bearer)
	if err != nil {
		return nil, errUnauthenticated
	}
	// The signature is verified; read the payload's current claim shape.
	parts := strings.Split(ac.Token, ".")
	if len(parts) != 3 {
		return nil, errUnauthenticated
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errUnauthenticated
	}
	var c platformClaims
	if err := json.Unmarshal(payload, &c); err != nil {
		return nil, errUnauthenticated
	}
	if c.TokenUse == "identity" {
		// An identity-only token carries no authority (see authservice.TokenUseIdentity).
		return nil, errUnauthenticated
	}
	return &abi.Caller{
		Kind:            abi.CallerPrincipal,
		ID:              c.Sub,
		Type:            c.Type,
		Tier:            c.Tier,
		Clients:         clientIDs(c.Clients),
		Roles:           c.Roles,
		Applications:    c.Applications,
		AllApplications: c.AllApplications,
		Permissions:     strings.Fields(c.Scope),
	}, nil
}

// clientIDs reduces the token's "id:identifier" client entries to ids ("*" stays).
func clientIDs(in []string) []string {
	out := make([]string, 0, len(in))
	for _, c := range in {
		id, _, _ := strings.Cut(c, ":")
		out = append(out, id)
	}
	return out
}

// verifyWebhook checks the platform's delivery signature over the raw body.
func verifyWebhook(secret string, r *http.Request, body []byte) bool {
	if secret == "" {
		return false
	}
	return webhook.NewValidator(secret).Validate(r.Header.Get(headerSignature), r.Header.Get(headerTimestamp), body) == nil
}

// hasPermission is the platform's rule: exact, or segment-wise with `*`
// matching any one segment, same segment count.
func hasPermission(held []string, required string) bool {
	req := strings.Split(required, ":")
	for _, h := range held {
		if h == required {
			return true
		}
		hs := strings.Split(h, ":")
		if len(hs) != len(req) {
			continue
		}
		match := true
		for i := range hs {
			if hs[i] != "*" && hs[i] != req[i] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// canReach is the platform's reach rule for a function: anchor callers reach
// everything; platform-owned functions (no client) are anchor-only; otherwise
// the caller must hold the function's client.
func canReach(c *abi.Caller, clientID *string) bool {
	if c.Tier == "ANCHOR" {
		return true
	}
	if clientID == nil {
		return false
	}
	for _, id := range c.Clients {
		if id == "*" || id == *clientID {
			return true
		}
	}
	return false
}
