package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/functiondomain"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// CreateCommand is the input DTO for POST /api/function-domains.
type CreateCommand struct {
	Zone     string  `json:"zone"`
	ClientID *string `json:"clientId,omitempty"`
}

// CreateDomain validates the zone shape, enforces per-resource client
// scope (a client-owned claim needs access to that client; a platform claim
// needs anchor), refuses an overlapping claim held by a DIFFERENT owner
// (plan: "may not be claimed if an existing claim by a DIFFERENT owner
// covers it or is covered by it" — checked both directions), and persists
// the claim.
func CreateDomain(repo *functiondomain.Repository) usecaseop.Operation[CreateCommand, DomainClaimed] {
	return usecaseop.Operation[CreateCommand, DomainClaimed]{
		Name: "CreateFunctionDomain",
		Validate: func(_ context.Context, cmd CreateCommand) error {
			zone := strings.ToLower(strings.TrimSpace(cmd.Zone))
			if zone == "" {
				return usecase.Validation("ZONE_REQUIRED", "zone is required")
			}
			if !functiondomain.ValidZone(zone) {
				return usecase.Validation("INVALID_ZONE_FORMAT",
					"zone must be a lowercase DNS hostname with no port and no wildcard")
			}
			return nil
		},
		// Resource-level authorization: a client-scoped claim requires access
		// to that client; a platform claim (nil ClientID) requires anchor. The
		// coarse "may manage function domains" permission is enforced at the
		// controller.
		Authorize: func(ctx context.Context, cmd CreateCommand) error {
			return auth.CheckScopeAccess(auth.FromContext(ctx), cmd.ClientID)
		},
		Execute: func(ctx context.Context, cmd CreateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[DomainClaimed], error) {
			zone := strings.ToLower(strings.TrimSpace(cmd.Zone))

			existing, err := repo.FindByZone(ctx, zone)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_zone failed", err)
			}
			if existing != nil {
				return nil, usecase.Conflict("ZONE_EXISTS", "zone '"+zone+"' is already claimed")
			}

			all, err := repo.FindAll(ctx)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_all failed", err)
			}
			for _, d := range all {
				if functiondomain.SameOwner(d.ClientID, cmd.ClientID) {
					continue
				}
				if functiondomain.Covers(d.Zone, zone) || functiondomain.Covers(zone, d.Zone) {
					return nil, usecase.Conflict("ZONE_OVERLAP",
						"zone '"+zone+"' overlaps an existing claim ('"+d.Zone+"') held by a different owner")
				}
			}

			d := functiondomain.New(zone, cmd.ClientID)
			d.CreatedBy = &ec.PrincipalID

			event := DomainClaimed{
				Metadata: usecase.NewEventMetadata(ec, DomainClaimedType, Source, subjectFor(d.ID)),
				DomainID: d.ID,
				Zone:     d.Zone,
				ClientID: d.ClientID,
			}
			return usecaseop.Save(d, repo, event), nil
		},
	}
}
