package fieldidentity

import (
	"context"
	"time"
)

// TokenClaims are the verified claims of a field access token, as the
// introspection endpoint needs them.
type TokenClaims struct {
	Subject    string
	ClientID   string
	WorkerCode string
	DeviceID   string
	AppCode    string
	// SessionID is the login family (the token's sid claim).
	SessionID string
	AMR       []string
	AuthTime  time.Time
	Expires   time.Time
}

// TokenVerifier verifies a field access token's signature, typ and expiry and
// returns its claims. The implementation arrives with the device protocol
// (slice B, authservice.GenerateFieldAccessToken's counterpart).
type TokenVerifier interface {
	VerifyFieldAccessToken(ctx context.Context, token string) (*TokenClaims, error)
}

// TokenActive reports whether the claims still describe a usable credential:
// not expired, the login family not revoked or expired, and both the worker
// and the device ACTIVE. It reads current state, which is the point of an
// online introspection check.
func (o *Ops) TokenActive(ctx context.Context, c *TokenClaims, now time.Time) (bool, error) {
	if c == nil || !c.Expires.After(now) {
		return false, nil
	}
	live, err := o.R.Sessions.IsFamilyLive(ctx, c.SessionID, now)
	if err != nil || !live {
		return false, err
	}
	ident, err := o.R.Identities.FindByID(ctx, c.Subject)
	if err != nil || ident == nil || ident.Status != StatusActive {
		return false, err
	}
	dev, err := o.R.Devices.FindByID(ctx, c.DeviceID)
	if err != nil || dev == nil || dev.Status != DeviceActive {
		return false, err
	}
	return true, nil
}
