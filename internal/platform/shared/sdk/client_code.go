package sdk

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// ClientCodes resolves an ingest item's clientCode (a client's identifier
// slug) to the client's id, memoised for the life of one request: a
// 1000-item batch costs one SELECT per distinct code.
//
// Not safe for concurrent use; build one per request.
type ClientCodes struct {
	repo  *client.Repository
	cache map[string]*string // nil value = looked up, no such client
}

// NewClientCodes builds a per-request resolver. A nil repository resolves
// every code as unknown.
func NewClientCodes(repo *client.Repository) *ClientCodes {
	return &ClientCodes{repo: repo, cache: map[string]*string{}}
}

// Resolve returns the id of the client whose identifier is code, or nil when
// there is none.
func (c *ClientCodes) Resolve(ctx context.Context, code string) (*string, error) {
	if id, ok := c.cache[code]; ok {
		return id, nil
	}
	var id *string
	if c.repo != nil {
		cl, err := c.repo.FindByIdentifier(ctx, code)
		if err != nil {
			return nil, usecase.Internal("REPO", "client find_by_identifier failed", err)
		}
		if cl != nil {
			v := string(cl.ID)
			id = &v
		}
	}
	c.cache[code] = id
	return id, nil
}

// NonBlank returns s, or nil when it is absent or only whitespace.
func NonBlank(s *string) *string {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil
	}
	return s
}

// ResolveJobClient is the dispatch-job client rule (owner ruling 2026-10-12,
// F14): an explicit clientId wins and must pass the client guard; otherwise
// a clientCode resolves to its client, which the caller must reach; an
// unknown code is refused for every caller, anchors included, so a job is
// never silently written unscoped. Refusals:
//
//   - 403 FORBIDDEN "No access to client: <clientId>" (explicit id outside
//     the caller's clients, as before);
//   - 403 FORBIDDEN "No access to client: <clientCode>" for a non-anchor
//     whose code names another client or no client (indistinguishable, and
//     never revealing the other client's id);
//   - 400 CLIENT_NOT_FOUND "Client not found: <clientCode>" for an anchor
//     whose code names no client.
//
// Neither id nor code: no client (unchanged).
func ResolveJobClient(ctx context.Context, ac *auth.AuthContext, codes *ClientCodes, clientID, clientCode *string) (*string, error) {
	if NonBlank(clientID) != nil {
		return RequireWritableClient(ac, clientID)
	}
	code := NonBlank(clientCode)
	if code == nil {
		return nil, nil
	}
	id, err := codes.Resolve(ctx, *code)
	if err != nil {
		return nil, err
	}
	switch {
	case id != nil && ac.CanAccessClient(*id):
		return id, nil
	case id == nil && ac.IsAnchor():
		return nil, httperror.BadRequest("CLIENT_NOT_FOUND", "Client not found: "+*code)
	default:
		return nil, httperror.Forbidden("No access to client: " + *code)
	}
}

// PlatformScopedRefused is the refusal of a platform-scoped write by a
// caller that is not an anchor and does not reach exactly one client.
const PlatformScopedRefused = "clientId is required: a client-scoped caller cannot write platform-scoped rows"

// RequireWritableClient is the ingest client guard (Java
// IngestApi.requireWritableClient, Rust caller_reach::require_writable_client):
// it answers the client a row is written under, or a 403 FORBIDDEN.
//
//   - A clientId (non-blank) the caller cannot reach: "No access to client: <id>".
//   - No clientId: an anchor writes a platform-scoped row (nil); a caller
//     confined to exactly one client writes under that client; any other
//     caller is refused with PlatformScopedRefused (a platform-scoped event
//     matches platform-wide subscriptions, so it acts beyond the caller's
//     reach).
func RequireWritableClient(ac *auth.AuthContext, clientID *string) (*string, error) {
	if id := NonBlank(clientID); id != nil {
		if !ac.CanAccessClient(*id) {
			return nil, httperror.Forbidden("No access to client: " + *id)
		}
		return id, nil
	}
	if ac.IsAnchor() {
		return nil, nil
	}
	if ac != nil && len(ac.Clients) == 1 && ac.Clients[0] != "*" {
		only := ac.Clients[0]
		return &only, nil
	}
	return nil, httperror.Forbidden(PlatformScopedRefused)
}

// ResolveEventClient is the event-ingest client rule (owner ruling
// 2026-10-12, F14; Java's and Rust's events batch): an explicit clientId wins
// and passes RequireWritableClient; otherwise a clientCode resolves to its
// client, which the caller must reach, and an unknown code leaves an anchor's
// event platform-scoped (the event is a fact; kept) and refuses anyone else's;
// a refused code answers 403 "No access to client: <clientCode>" whether it
// names another client or none. Neither: RequireWritableClient(nil).
func ResolveEventClient(ctx context.Context, ac *auth.AuthContext, codes *ClientCodes, clientID, clientCode *string) (*string, error) {
	if NonBlank(clientID) != nil {
		return RequireWritableClient(ac, clientID)
	}
	code := NonBlank(clientCode)
	if code == nil {
		return RequireWritableClient(ac, nil)
	}
	id, err := codes.Resolve(ctx, *code)
	if err != nil {
		return nil, err
	}
	switch {
	case id == nil && ac.IsAnchor():
		return nil, nil
	case id == nil || !ac.CanAccessClient(*id):
		return nil, httperror.Forbidden("No access to client: " + *code)
	}
	return id, nil
}
