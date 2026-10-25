package platformconfig

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
)

// DeletePropertyCommand is the input of the delete-property use case. The
// audit row records it (no value is in it).
type DeletePropertyCommand struct {
	ApplicationCode string `json:"applicationCode"`
	Section         string `json:"section"`
	Property        string `json:"property"`
	// ClientID is the canonical client id; nil for the application-wide value.
	ClientID *string `json:"clientId,omitempty"`
	// OnBehalfOf is who a service account acted for (X-On-Behalf-Of).
	OnBehalfOf *string `json:"onBehalfOf,omitempty"`
}

// DeletePropertyOp deletes one property and emits [PropertyDeleted], with an
// audit row. Its Authorize phase is the whole write rule (Service.AuthorizeSet:
// the application confinement, the permission or write grant, and the client
// confinement), so no caller relies on its own checks. A property that is not
// there is 404 Config_NOT_FOUND (Service.DeleteProperty treats that as done).
//
// It lives in this package, not beside SetProperty in operations, because
// Service.Delete runs it and operations imports this package.
func DeletePropertyOp(s *Service) usecaseop.Operation[DeletePropertyCommand, PropertyDeleted] {
	return usecaseop.Operation[DeletePropertyCommand, PropertyDeleted]{
		Name: "DeleteProperty",
		Validate: func(_ context.Context, cmd DeletePropertyCommand) error {
			for name, v := range map[string]string{
				"applicationCode": cmd.ApplicationCode, "section": cmd.Section, "property": cmd.Property,
			} {
				if strings.TrimSpace(v) == "" {
					return usecase.Validation("FIELD_REQUIRED", name+" is required")
				}
			}
			return nil
		},
		Authorize: func(ctx context.Context, cmd DeletePropertyCommand) error {
			return s.AuthorizeSet(ctx, auth.FromContext(ctx), cmd.ApplicationCode, deref(cmd.ClientID))
		},
		Execute: func(ctx context.Context, cmd DeletePropertyCommand, ec usecase.ExecutionContext) (usecaseop.Plan[PropertyDeleted], error) {
			scope, clientID := ScopeFor(deref(cmd.ClientID))
			c, err := s.Repo.FindByCoordinate(ctx, cmd.ApplicationCode, cmd.Section, cmd.Property, scope, clientID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_coordinate failed", err)
			}
			if c == nil {
				return nil, httperror.NotFound("Config", cmd.ApplicationCode+"/"+cmd.Section+"/"+cmd.Property)
			}
			event := PropertyDeleted{
				Metadata:        usecase.NewEventMetadata(ec, PropertyDeletedType, "platform:admin", "platform.platformconfig."+c.ID),
				ConfigID:        c.ID,
				ApplicationCode: c.ApplicationCode,
				Section:         c.Section,
				Property:        c.Property,
				ClientID:        ids.StringPtr(c.ClientID),
			}
			return usecaseop.Delete(c, s.Repo, event), nil
		},
	}
}
