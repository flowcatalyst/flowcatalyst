package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
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

// SetPropertyUseCase implements UseCase. Upserts the (app, section,
// property, scope, client_id) coordinate with the supplied value.
type SetPropertyUseCase struct {
	repo *platformconfig.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewSetPropertyUseCase wires the use case.
func NewSetPropertyUseCase(repo *platformconfig.Repository, uow *usecasepgx.UnitOfWork) *SetPropertyUseCase {
	return &SetPropertyUseCase{repo: repo, uow: uow}
}

func (uc *SetPropertyUseCase) Validate(_ context.Context, cmd SetPropertyCommand) error {
	for name, v := range map[string]string{
		"applicationCode": cmd.ApplicationCode, "section": cmd.Section, "property": cmd.Property,
	} {
		if strings.TrimSpace(v) == "" {
			return usecase.Validation("FIELD_REQUIRED", name+" is required")
		}
	}
	return nil
}

func (uc *SetPropertyUseCase) Authorize(_ context.Context, _ SetPropertyCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *SetPropertyUseCase) Execute(ctx context.Context, cmd SetPropertyCommand, ec usecase.ExecutionContext) usecase.Result[PropertySet] {
	scope := platformconfig.ScopeGlobal
	if cmd.ClientID != nil {
		scope = platformconfig.ScopeClient
	}
	existing, err := uc.repo.FindByCoordinate(ctx, cmd.ApplicationCode, cmd.Section, cmd.Property, scope, cmd.ClientID)
	if err != nil {
		return usecase.Failure[PropertySet](usecase.Internal("REPO", "find_by_coordinate failed", err))
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
	return usecasepgx.Commit[platformconfig.Config, PropertySet, SetPropertyCommand](
		ctx, uc.uow, c, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[SetPropertyCommand, PropertySet] = (*SetPropertyUseCase)(nil)
