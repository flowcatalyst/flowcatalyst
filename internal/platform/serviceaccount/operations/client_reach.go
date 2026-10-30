package operations

import (
	"context"
	"strconv"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/go-sdk/usecase"
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
		p.AssignedClients = []ids.ClientID{}
		return nil
	case 1:
		id := clientIDs[0]
		p.Scope = principal.ScopeClient
		p.ClientID = ids.PtrOf[ids.ClientID](&id)
		p.AssignedClients = []ids.ClientID{}
		return nil
	default:
		p.Scope = principal.ScopePartner
		p.ClientID = nil
		p.AssignedClients = ids.Typed[ids.ClientID](append([]string(nil), clientIDs...))
		return ids.Strings(p.AssignedClients)
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

// derivedTier is the tier a token for an account with these client links gets
// (applyClientReach): none = ANCHOR, one = CLIENT, several = PARTNER.
func derivedTier(clientIDs []string) string {
	switch len(clientIDs) {
	case 0:
		return string(principal.ScopeAnchor)
	case 1:
		return string(principal.ScopeClient)
	default:
		return string(principal.ScopePartner)
	}
}

// requireScopeMatchesReach refuses a stored `scope` that disagrees with the
// tier the account's client links give its token (F28). The scope field is
// descriptive: tokens follow the links, so ANCHOR stored beside client links
// was echoed by the API while the token stayed PARTNER or CLIENT, and a scope
// of CLIENT or PARTNER with too few links yields a wider (ANCHOR) token than
// the field claims. Refusing keeps the stored value and the token one fact,
// and never widens reach. A scope that is not one of the three tiers, or is
// unset, is left alone.
func requireScopeMatchesReach(scope *string, clientIDs []string) error {
	if scope == nil {
		return nil
	}
	want := strings.ToUpper(strings.TrimSpace(*scope))
	switch want {
	case string(principal.ScopeAnchor), string(principal.ScopeClient), string(principal.ScopePartner):
	default:
		return nil
	}
	if got := derivedTier(clientIDs); got != want {
		return usecase.Validation("SCOPE_REACH_MISMATCH",
			"scope "+want+" does not match the account's client links ("+itoa(len(clientIDs))+
				" linked: tokens are ANCHOR with no clients, CLIENT with one, PARTNER with several, so these links give "+
				got+"). Change clientIds and scope together, or omit scope")
	}
	return nil
}

func itoa(n int) string { return strconv.Itoa(n) }
