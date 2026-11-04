package fn

import (
	"context"
	"encoding/json"
	"errors"
)

// PlatformTokenOption configures a PlatformToken call.
type PlatformTokenOption func(*platformTokenMeta)

// Refresh skips the host's cached token and fetches a new one. Use it when
// the target answered 401 with the token you were given (for example the
// application's service account had its roles changed).
func Refresh() PlatformTokenOption {
	return func(m *platformTokenMeta) { m.Refresh = true }
}

type platformTokenMeta struct {
	Refresh bool `json:"refresh,omitempty"`
}

type platformTokenResult struct {
	AccessToken string `json:"accessToken"`
	ExpiresAt   int64  `json:"expiresAt"`
}

// UsesPlatformToken declares that this function calls PlatformToken (describe
// key `platformToken: true`). Without it the host refuses the call with
// NOT_DECLARED.
func UsesPlatformToken() {
	reg.mu.Lock()
	reg.platformToken = true
	reg.mu.Unlock()
}

// PlatformToken returns a FlowCatalyst access token (op 15, platform.token)
// to send as a bearer to another first-party application that accepts
// FlowCatalyst tokens. The token is the service account linked to the
// function's application; the function holds no client id or secret. The host
// caches it and renews it shortly before it expires, so call it per request
// rather than keeping it. Never log it.
//
// Errors wrap *Error; errors.Is(err, ErrRetryable) is true for UNAVAILABLE
// (platform trouble) and false for a permanent refusal: NOT_DECLARED
// (UsesPlatformToken not called) or NOT_ALLOWED (the platform refused: not
// declared there, the application has no linked service account, or the
// account is inactive; the message carries the platform code). DEADLINE means
// the invocation's deadline passed. A runner that predates the capability
// answers UNKNOWN_OP, returned as an *Error with that code and a message
// saying the runner does not support platform tokens.
func PlatformToken(ctx context.Context, opts ...PlatformTokenOption) (string, error) {
	var meta platformTokenMeta
	for _, o := range opts {
		o(&meta)
	}
	mb, merr := json.Marshal(meta)
	if merr != nil {
		return "", merr
	}
	rm, _, hostErr, err := currentHost.call(opPlatformToken, mb, nil)
	if err != nil {
		return "", err
	}
	if hostErr != nil {
		if hostErr.Code == ErrCodeUnknownOp {
			return "", &Error{Code: ErrCodeUnknownOp, Message: "this runner does not support platform tokens (it predates host op 15, platform.token); upgrade the function runner"}
		}
		return "", hostErr
	}
	var out platformTokenResult
	if len(rm) > 0 {
		if uerr := json.Unmarshal(rm, &out); uerr != nil {
			return "", uerr
		}
	}
	if out.AccessToken == "" {
		return "", errors.New("fn: platform.token: host returned no access token")
	}
	return out.AccessToken, nil
}
