package server

import (
	"context"
	"net/url"
	"sync"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/oauthapi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/pages"
)

// clientFinder is the OAuth client lookup /oauth/authorize uses.
type clientFinder interface {
	FindByClientID(ctx context.Context, clientID string) (*auth.OAuthClient, error)
}

const (
	formDestinationTTL  = 30 * time.Second
	formDestinationSize = 1024
)

// formDestination answers pages.FormDestination from the OAuth client
// repository: the origin of redirectURI when it is registered for the active,
// non-portal client clientID by oauthapi.MatchRedirectURI, the rule
// /oauth/authorize applies. Registrations are cached briefly (unknown clients
// too, so a crafted client_id cannot turn each page view into a query); the
// cache only decides which origin a sign-in page's CSP names, and
// /oauth/authorize still checks every request itself.
func formDestination(clients clientFinder) pages.FormDestination {
	type entry struct {
		uris    []string // nil: unknown, inactive or portal client
		expires time.Time
	}
	var (
		mu    sync.Mutex
		cache = map[string]entry{}
	)
	lookup := func(ctx context.Context, clientID string) []string {
		now := time.Now()
		mu.Lock()
		e, ok := cache[clientID]
		mu.Unlock()
		if ok && now.Before(e.expires) {
			return e.uris
		}
		c, err := clients.FindByClientID(ctx, clientID)
		if err != nil {
			return nil // not cached: the next view asks again
		}
		e = entry{expires: now.Add(formDestinationTTL)}
		if c != nil && c.Active && (c.PortalClientID == nil || *c.PortalClientID == "") {
			e.uris = c.RedirectURIs
		}
		mu.Lock()
		if len(cache) >= formDestinationSize {
			clear(cache)
		}
		cache[clientID] = e
		mu.Unlock()
		return e.uris
	}
	return func(ctx context.Context, clientID, redirectURI string) (string, bool) {
		uris := lookup(ctx, clientID)
		if len(uris) == 0 || !oauthapi.MatchRedirectURI(redirectURI, uris) {
			return "", false
		}
		u, err := url.Parse(redirectURI)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return "", false
		}
		return u.Scheme + "://" + u.Host, true
	}
}
