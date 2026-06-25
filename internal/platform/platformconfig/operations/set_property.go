package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// SetPropertyCommand is the input DTO.
type SetPropertyCommand struct {
	ApplicationCode string  `json:"applicationCode"`
	Section         string  `json:"section"`
	Property        string  `json:"property"`
	Value           string  `json:"value"`
	ValueType       *string `json:"valueType,omitempty"`
	Description     *string `json:"description,omitempty"`
	ClientID        *string `json:"clientId,omitempty"`
}

// SetProperty upserts the (app, section, property, scope, client_id)
// coordinate with the supplied value and emits [PropertySet].
//
// Unlike the access mutations, set-property is NOT anchor-only: the handler
// admitted any anchor, OR any non-anchor principal holding write access to the
// target application (repo.HasAccess(..., wantWrite=true)). The Authorize phase
// encodes that exact rule 1:1 — it captures repo to run the access check.
func SetProperty(repo *platformconfig.Repository) usecaseop.Operation[SetPropertyCommand, PropertySet] {
	return usecaseop.Operation[SetPropertyCommand, PropertySet]{
		Name: "SetProperty",
		Validate: func(_ context.Context, cmd SetPropertyCommand) error {
			for name, v := range map[string]string{
				"applicationCode": cmd.ApplicationCode, "section": cmd.Section, "property": cmd.Property,
			} {
				if strings.TrimSpace(v) == "" {
					return usecase.Validation("FIELD_REQUIRED", name+" is required")
				}
			}
			return nil
		},
		Authorize: func(ctx context.Context, cmd SetPropertyCommand) error {
			ac := auth.FromContext(ctx)
			if ac.IsAnchor() {
				return nil
			}
			ok, err := repo.HasAccess(ctx, cmd.ApplicationCode, ac.Roles, true)
			if err != nil {
				return usecase.Internal("REPO", "has_access failed", err)
			}
			if !ok {
				return httperror.Forbidden("No write access to platform config for " + cmd.ApplicationCode)
			}
			return nil
		},
		Execute: func(ctx context.Context, cmd SetPropertyCommand, ec usecase.ExecutionContext) (usecaseop.Plan[PropertySet], error) {
			scope := platformconfig.ScopeGlobal
			if cmd.ClientID != nil {
				scope = platformconfig.ScopeClient
			}
			existing, err := repo.FindByCoordinate(ctx, cmd.ApplicationCode, cmd.Section, cmd.Property, scope, cmd.ClientID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_coordinate failed", err)
			}
			var c *platformconfig.Config
			if existing != nil {
				c = existing
				c.Value = cmd.Value
				c.Description = cmd.Description
				if cmd.ValueType != nil {
					c.ValueType = platformconfig.ParseValueType(*cmd.ValueType)
				}
			} else {
				c = platformconfig.NewConfig(cmd.ApplicationCode, cmd.Section, cmd.Property, cmd.Value)
				c.Scope = scope
				c.ClientID = cmd.ClientID
				c.Description = cmd.Description
				if cmd.ValueType != nil {
					c.ValueType = platformconfig.ParseValueType(*cmd.ValueType)
				}
			}

			event := PropertySet{
				Metadata:        usecase.NewEventMetadata(ec, PropertySetType, Source, subjectFor(c.ID)),
				ConfigID:        c.ID,
				ApplicationCode: c.ApplicationCode,
				Section:         c.Section,
				Property:        c.Property,
			}
			return usecaseop.Save(c, repo, event), nil
		},
	}
}
