package serviceaccount

import (
	"context"
	"sync"
	"time"
)

// NewCachedSigningSecretResolver returns an application-id → webhook signing
// secret lookup ("" when the application has no active SA or no secret),
// memoised with the given TTL. One resolver backs every outbound-signing
// call site (scheduled-job firings, dispatch webhook deliveries): the cache
// keeps a delivery batch from re-querying per item, while the short TTL means
// a secret rotation takes effect within a minute — no restart, no re-sync.
func NewCachedSigningSecretResolver(repo *Repository, ttl time.Duration) func(ctx context.Context, applicationID string) (string, error) {
	type entry struct {
		secret  string
		expires time.Time
	}
	var mu sync.Mutex
	cache := make(map[string]entry)
	return func(ctx context.Context, applicationID string) (string, error) {
		mu.Lock()
		if e, ok := cache[applicationID]; ok && time.Now().Before(e.expires) {
			mu.Unlock()
			return e.secret, nil
		}
		mu.Unlock()

		sa, err := repo.FindFirstByApplicationID(ctx, applicationID)
		if err != nil {
			return "", err
		}
		secret := ""
		if sa != nil && sa.WebhookCredentials.SigningSecret != nil {
			secret = *sa.WebhookCredentials.SigningSecret
		}
		mu.Lock()
		cache[applicationID] = entry{secret: secret, expires: time.Now().Add(ttl)}
		mu.Unlock()
		return secret, nil
	}
}
