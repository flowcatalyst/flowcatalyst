package client

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// identifierLookup narrows Repository to the one read NewCachedIdentifierResolver
// needs, so a test can substitute a counting fake without mocking pgx (the
// cache is what's under test, not the repository's SQL). *Repository
// satisfies this trivially.
type identifierLookup interface {
	FindByID(ctx context.Context, id string) (*Client, error)
}

// NewCachedIdentifierResolver returns a clientID → identifier lookup for
// delivery paths that must not pay a database round trip per job — see
// docs/spec/webhook-client-code.md R3 (the dispatch-processing webhook
// envelope's clientCode and the X-FlowCatalyst-Client header).
//
// Caching policy:
//   - A client's identifier is immutable after create (Repository.Persist
//     never changes it once set), so a HIT is cached for the process's
//     life — no TTL, no invalidation.
//   - A MISS (not found, or a repository error) is cached for only
//     negativeTTL: a client created after the first miss resolves on a
//     later call once the window elapses, rather than staying unresolvable
//     forever.
//   - A repository error resolves to ok=false — "unknown" — same as a
//     genuine not-found, rather than propagating: the caller's delivery
//     must proceed without the code. Logged at most once per client id
//     (not once per call), so a sustained outage doesn't spam the log once
//     per job.
func NewCachedIdentifierResolver(repo identifierLookup, negativeTTL time.Duration) func(ctx context.Context, clientID string) (string, bool) {
	type entry struct {
		identifier string
		ok         bool
		expires    time.Time // unused when ok: a hit never expires
	}

	var mu sync.Mutex
	cache := make(map[string]entry)
	warned := make(map[string]bool)

	return func(ctx context.Context, clientID string) (string, bool) {
		if clientID == "" {
			return "", false
		}

		mu.Lock()
		if e, found := cache[clientID]; found && (e.ok || time.Now().Before(e.expires)) {
			mu.Unlock()
			return e.identifier, e.ok
		}
		mu.Unlock()

		c, err := repo.FindByID(ctx, clientID)
		if err != nil {
			mu.Lock()
			if !warned[clientID] {
				warned[clientID] = true
				slog.Warn("dispatch delivery: client identifier lookup failed; delivering without clientCode",
					"client_id", clientID, "err", err)
			}
			cache[clientID] = entry{expires: time.Now().Add(negativeTTL)}
			mu.Unlock()
			return "", false
		}
		if c == nil {
			mu.Lock()
			cache[clientID] = entry{expires: time.Now().Add(negativeTTL)}
			mu.Unlock()
			return "", false
		}

		mu.Lock()
		cache[clientID] = entry{identifier: c.Identifier, ok: true}
		mu.Unlock()
		return c.Identifier, true
	}
}
