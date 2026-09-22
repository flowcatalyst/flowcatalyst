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
//
// Reason says why BOTH are empty, when they are — "connection cnn_x's
// service account sa_y is inactive", "application billing has no active
// service account" — so an unsigned delivery can say so on its attempt
// instead of surfacing as a bare "HTTP 401" three retries later
// (2026-09-22). Empty whenever any credential is present.
type OutboundCreds struct {
	BearerToken   string
	SigningSecret string
	Reason        string
}

// Empty reports whether the delivery would go out bare.
func (c OutboundCreds) Empty() bool { return c.BearerToken == "" && c.SigningSecret == "" }

// NewCachedOutboundCredsResolver returns an application-id → OutboundCreds
// lookup (zero-value creds, with a Reason, when the application has no
// active SA or no credentials), memoised with the given TTL. One resolver
// backs every outbound-delivery call site: the cache keeps a delivery batch
// from re-querying per item, while the short TTL means a credential
// rotation takes effect within a minute — no restart, no re-sync.
func NewCachedOutboundCredsResolver(repo *Repository, ttl time.Duration) func(ctx context.Context, applicationID string) (OutboundCreds, error) {
	return cachedCreds(ttl, func(ctx context.Context, applicationID string) (OutboundCreds, error) {
		sa, err := repo.FindFirstByApplicationID(ctx, applicationID)
		if err != nil {
			return OutboundCreds{}, err
		}
		if sa == nil {
			return OutboundCreds{Reason: "application " + applicationID + " has no active service account"}, nil
		}
		return credsOf(ctx, repo, sa, "application "+applicationID+"'s service account "+sa.Code), nil
	})
}

// NewCachedOutboundCredsByIDResolver is the same lookup keyed by service
// account id — for a delivery whose subscription or connection names its
// signing account explicitly (2026-09-22 ruling: the connection's service
// account signs; see server.newDeliveryCredsResolver). An inactive account
// yields no credentials, never a fall-through: deactivating an account must
// stop it signing, and a caller that wants a fallback asks for one itself.
func NewCachedOutboundCredsByIDResolver(repo *Repository, ttl time.Duration) func(ctx context.Context, serviceAccountID string) (OutboundCreds, error) {
	return cachedCreds(ttl, func(ctx context.Context, id string) (OutboundCreds, error) {
		sa, err := repo.FindByID(ctx, id)
		if err != nil {
			return OutboundCreds{}, err
		}
		if sa == nil {
			return OutboundCreds{Reason: "service account " + id + " does not exist"}, nil
		}
		if !sa.Active {
			return OutboundCreds{Reason: "service account " + sa.Code + " is inactive"}, nil
		}
		return credsOf(ctx, repo, sa, "service account "+sa.Code), nil
	})
}

// credsOf lifts an account's webhook credentials, stamping last-used when
// anything is handed out. The stamp is best-effort and outside any delivery
// transaction — a failed stamp must never fail a delivery — and lands on the
// CACHE MISS only: the field means "last known use", and a write per
// delivery on this hot path would cost more than the precision is worth.
func credsOf(ctx context.Context, repo *Repository, sa *ServiceAccount, who string) OutboundCreds {
	var creds OutboundCreds
	if sa.WebhookCredentials.Token != nil {
		creds.BearerToken = *sa.WebhookCredentials.Token
	}
	if sa.WebhookCredentials.SigningSecret != nil {
		creds.SigningSecret = *sa.WebhookCredentials.SigningSecret
	}
	if creds.Empty() {
		creds.Reason = who + " has no webhook credentials"
		return creds
	}
	_ = repo.TouchLastUsed(ctx, sa.ID)
	return creds
}

// cachedCreds memoises a key → OutboundCreds lookup for ttl. Errors are not
// cached: the next call retries.
func cachedCreds(ttl time.Duration, lookup func(ctx context.Context, key string) (OutboundCreds, error)) func(ctx context.Context, key string) (OutboundCreds, error) {
	type entry struct {
		creds   OutboundCreds
		expires time.Time
	}
	var mu sync.Mutex
	cache := make(map[string]entry)
	return func(ctx context.Context, key string) (OutboundCreds, error) {
		mu.Lock()
		if e, ok := cache[key]; ok && time.Now().Before(e.expires) {
			mu.Unlock()
			return e.creds, nil
		}
		mu.Unlock()

		creds, err := lookup(ctx, key)
		if err != nil {
			return OutboundCreds{}, err
		}
		mu.Lock()
		cache[key] = entry{creds: creds, expires: time.Now().Add(ttl)}
		mu.Unlock()
		return creds, nil
	}
}
