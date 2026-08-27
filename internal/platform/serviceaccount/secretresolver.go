package serviceaccount

import (
	"context"
	"sync"
	"time"
)

// OutboundCreds is what a platform-originated delivery (scheduled-job firing,
// dispatch webhook) stamps on the request: a static bearer token plus the
// HMAC signing secret. Either may be empty when the service account has no
// webhook credentials; the signature is the security boundary, the bearer is
// convenience/defense-in-depth for apps fronted by bearer-auth middleware.
type OutboundCreds struct {
	BearerToken   string
	SigningSecret string
}

// NewCachedOutboundCredsResolver returns an application-id → OutboundCreds
// lookup (zero-value creds when the application has no active SA or no
// credentials), memoised with the given TTL. One resolver backs every
// outbound-delivery call site: the cache keeps a delivery batch from
// re-querying per item, while the short TTL means a credential rotation takes
// effect within a minute — no restart, no re-sync.
func NewCachedOutboundCredsResolver(repo *Repository, ttl time.Duration) func(ctx context.Context, applicationID string) (OutboundCreds, error) {
	type entry struct {
		creds   OutboundCreds
		expires time.Time
	}
	var mu sync.Mutex
	cache := make(map[string]entry)
	return func(ctx context.Context, applicationID string) (OutboundCreds, error) {
		mu.Lock()
		if e, ok := cache[applicationID]; ok && time.Now().Before(e.expires) {
			mu.Unlock()
			return e.creds, nil
		}
		mu.Unlock()

		sa, err := repo.FindFirstByApplicationID(ctx, applicationID)
		if err != nil {
			return OutboundCreds{}, err
		}
		var creds OutboundCreds
		if sa != nil {
			if sa.WebhookCredentials.Token != nil {
				creds.BearerToken = *sa.WebhookCredentials.Token
			}
			if sa.WebhookCredentials.SigningSecret != nil {
				creds.SigningSecret = *sa.WebhookCredentials.SigningSecret
			}
		}
		if sa != nil && (creds.BearerToken != "" || creds.SigningSecret != "") {
			// The account's outbound secrets were handed to a delivery — a use
			// of its credentials. Stamped on the CACHE MISS only: the field
			// means "last known use", and a write per delivery on this hot path
			// would cost more than the precision is worth. Best-effort, and
			// outside any delivery transaction — a failed stamp must never fail
			// a delivery.
			_ = repo.TouchLastUsed(ctx, sa.ID)
		}
		mu.Lock()
		cache[applicationID] = entry{creds: creds, expires: time.Now().Add(ttl)}
		mu.Unlock()
		return creds, nil
	}
}
