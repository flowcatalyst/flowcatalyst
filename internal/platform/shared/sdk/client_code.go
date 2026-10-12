package sdk

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
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
	if id := NonBlank(clientID); id != nil {
		if !ac.CanAccessClient(*id) {
			return nil, httperror.Forbidden("No access to client: " + *id)
		}
		return id, nil
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
