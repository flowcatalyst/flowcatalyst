package operations

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// applyClientReach derives the linked SERVICE principal's association from the
// account's client links, and returns the client ids that need partner grants.
//
// A service account's reach is its client links: a token is built from the
// principal, so links that never reach the principal never reach a token —
// which is how every service account came to reach every tenant regardless of
// what it was linked to.
//
//	no links  → ANCHOR   (unchanged; also every application-provisioned account)
//	one link  → CLIENT   confined to that client
//	several   → PARTNER  with one assigned-client grant each
func applyClientReach(p *principal.Principal, clientIDs []string) []string {
	switch len(clientIDs) {
	case 0:
		p.Scope = principal.ScopeAnchor
		p.ClientID = nil
		p.AssignedClients = []string{}
		return nil
	case 1:
		id := clientIDs[0]
		p.Scope = principal.ScopeClient
		p.ClientID = &id
		p.AssignedClients = []string{}
		return nil
	default:
		p.Scope = principal.ScopePartner
		p.ClientID = nil
		p.AssignedClients = append([]string(nil), clientIDs...)
		return p.AssignedClients
	}
}

// requireClientsExist refuses an unknown client id the way the user
// client-association operation does, so a typo is a 404 naming the client
// rather than a principal silently confined to a client that is not there.
func requireClientsExist(ctx context.Context, clients *client.Repository, clientIDs []string) error {
	if clients == nil {
		return nil
	}
	for _, id := range clientIDs {
		c, err := clients.FindByID(ctx, id)
		if err != nil {
			return usecase.Internal("REPO", "find_client failed", err)
		}
		if c == nil {
			return httperror.NotFound("Client", id)
		}
	}
	return nil
}
