package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
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
func SetProperty(
	ctx context.Context,
	repo *platformconfig.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd SetPropertyCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[PropertySet], error) {
	var zero commit.Committed[PropertySet]

	for name, v := range map[string]string{
		"applicationCode": cmd.ApplicationCode, "section": cmd.Section, "property": cmd.Property,
	} {
		if strings.TrimSpace(v) == "" {
			return zero, usecase.Validation("FIELD_REQUIRED", name+" is required")
		}
	}

	scope := platformconfig.ScopeGlobal
	if cmd.ClientID != nil {
		scope = platformconfig.ScopeClient
	}
	existing, err := repo.FindByCoordinate(ctx, cmd.ApplicationCode, cmd.Section, cmd.Property, scope, cmd.ClientID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_coordinate failed", err)
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
	return commit.Save(ctx, uow, c, repo, event, cmd)
}
