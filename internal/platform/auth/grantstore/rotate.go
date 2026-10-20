package grantstore

import (
	"context"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/envutil"
)

// ReplayLeewayEnv names the setting for ReplayLeeway, in seconds; 0 turns the
// leeway off (every replay of a rotated-out token revokes its family).
const ReplayLeewayEnv = "FC_REFRESH_REPLAY_LEEWAY_SECS"

// defaultReplayLeeway is how long after a token is rotated out a second
// presentation of it is treated as its own client racing or retrying, not a
// theft. The Laravel and TypeScript SDKs refresh per request without a lock,
// and a lost response is retried with the old token.
const defaultReplayLeeway = 10 * time.Second

// ReplayLeeway is the window in effect (see ReplayLeewayEnv).
func ReplayLeeway() time.Duration {
	n := envutil.Int(ReplayLeewayEnv, int(defaultReplayLeeway/time.Second))
	if n <= 0 {
		return 0
	}
	return time.Duration(n) * time.Second
}

// RotationResult is Rotate's outcome. Stored is the consumed token (nil when
// the presented token was invalid/expired/revoked); NewRaw/New are the
// replacement (set only on a completed rotation).
type RotationResult struct {
	Stored *RefreshToken
	NewRaw string
	New    *RefreshToken
}

// Rotate consumes a presented refresh token and issues its replacement,
// implementing the full rotation contract in ONE place so the two refresh
// surfaces (/oauth/token refresh_token grant and /auth/refresh) can't drift:
//
//   - Reuse detection (OAuth 2.0 Security BCP §4.14.2): a token we don't
//     accept as valid might be one we already rotated out. If the hash
//     matches a previously-replaced token, the family is presumed
//     compromised and every token in it is revoked, so a stolen,
//     already-rotated token cannot keep the chain alive.
//   - Replay leeway (ReplayLeeway, default 10 s): a token rotated out moments
//     ago, whose replacement is still valid, is almost always its own client
//     racing or retrying. It rotates again into a sibling in the same family
//     instead of revoking it. Later, or once the family is revoked, it is a
//     replay as above.
//   - Rotation: the presented token is revoked before the replacement is
//     inserted.
//   - Lineage: the replacement preserves Scopes, AccessibleClients, AuthTime and the
//     OAuth client binding, stays in the presented token's rotation family
//     (rooting a fresh family for legacy tokens that predate tracking), and
//     the rotated-out token is linked to its replacement so a later replay
//     trips the reuse detection above.
//
// authorize, when non-nil, runs between lookup and revocation — it's the
// caller's binding check (e.g. "issued to the authenticated OAuth client").
// On authorize failure nothing is rotated and (RotationResult{Stored}, err)
// is returned so the caller can shape its own response.
//
// A nil-Stored result with nil error means "invalid or expired token".
func Rotate(ctx context.Context, repo *RefreshTokenRepository, rawToken string, authorize func(*RefreshToken) error) (RotationResult, error) {
	tokenHash := HashToken(rawToken)
	stored, err := repo.FindValidByHash(ctx, tokenHash)
	if err != nil {
		return RotationResult{}, err
	}
	if stored == nil {
		return repo.replayOrRevoke(ctx, tokenHash, authorize)
	}

	if authorize != nil {
		if aerr := authorize(stored); aerr != nil {
			return RotationResult{Stored: stored}, aerr
		}
	}

	raw, entity, err := successorOf(stored)
	if err != nil {
		return RotationResult{Stored: stored}, err
	}

	// Revoke-and-issue is ONE transaction around ONE guarded claim: of N
	// concurrent presentations of the same token exactly one wins; losers
	// are treated as replays (the winner's tx has already linked the token
	// to its replacement, so reuse detection fires). A failed insert rolls
	// the revocation back instead of stranding the client without a token.
	tx, err := repo.pool.Begin(ctx)
	if err != nil {
		return RotationResult{Stored: stored}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	won, err := repo.claimForRotation(ctx, tx, tokenHash, entity.TokenHash)
	if err != nil {
		return RotationResult{Stored: stored}, err
	}
	if !won {
		_ = tx.Rollback(ctx)
		return repo.replayOrRevoke(ctx, tokenHash, authorize)
	}
	if err := repo.insertWith(ctx, tx, entity); err != nil {
		return RotationResult{Stored: stored}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RotationResult{Stored: stored}, err
	}

	return RotationResult{Stored: stored, NewRaw: raw, New: entity}, nil
}

// successorOf builds the replacement for stored: same principal, scopes,
// accessible clients, client binding and sign-in time, in the same family
// (rooting a fresh family for a legacy token), with the family's absolute
// expiry carried forward, never a fresh one.
func successorOf(stored *RefreshToken) (string, *RefreshToken, error) {
	raw, entity, err := GenerateTokenPair(stored.PrincipalID)
	if err != nil {
		return "", nil, err
	}
	entity.Scopes = stored.Scopes
	entity.AccessibleClients = stored.AccessibleClients
	entity.OAuthClientID = stored.OAuthClientID
	// The sign-in time belongs to the family, not the individual token: a
	// rotation is not a re-authentication, so auth_time must not advance.
	entity.AuthTime = stored.AuthTime
	// The original deadline is the family's absolute cap (see
	// RefreshTokenTTL); carrying it forward unchanged means rotation renews
	// the access token without ever extending the refresh window.
	entity.ExpiresAt = stored.ExpiresAt
	if stored.TokenFamily != nil {
		entity.TokenFamily = stored.TokenFamily
	} else {
		fam := entity.ID
		entity.TokenFamily = &fam
	}
	return raw, entity, nil
}

// replayOrRevoke handles a presented token that is not valid (or that lost a
// rotation race). One rotated out within ReplayLeeway with its replacement
// still valid is its own client: it gets a sibling in the same family. Any
// other rotated-out token is the BCP §4.14.2 reuse case: the family is
// presumed compromised and revoked. A token that was never rotated out is
// simply invalid (nil Stored, nil error).
func (r *RefreshTokenRepository) replayOrRevoke(ctx context.Context, tokenHash string, authorize func(*RefreshToken) error) (RotationResult, error) {
	prior, err := r.FindByHash(ctx, tokenHash)
	if err != nil || prior == nil || prior.ReplacedBy == nil || prior.TokenFamily == nil {
		return RotationResult{}, nil
	}
	if leeway := ReplayLeeway(); leeway > 0 && prior.RevokedAt != nil && time.Since(*prior.RevokedAt) <= leeway {
		if next, ferr := r.FindValidByHash(ctx, *prior.ReplacedBy); ferr == nil && next != nil {
			if authorize != nil {
				if aerr := authorize(prior); aerr != nil {
					return RotationResult{Stored: prior}, aerr
				}
			}
			raw, sibling, serr := successorOf(prior)
			if serr != nil {
				return RotationResult{Stored: prior}, serr
			}
			if ierr := r.Insert(ctx, sibling); ierr != nil {
				return RotationResult{Stored: prior}, ierr
			}
			return RotationResult{Stored: prior, NewRaw: raw, New: sibling}, nil
		}
	}
	_, _ = r.RevokeAllInFamily(ctx, *prior.TokenFamily)
	return RotationResult{}, nil
}
