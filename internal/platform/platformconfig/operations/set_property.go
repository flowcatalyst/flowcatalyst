package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
)

// SetPropertyCommand is the input DTO.
type SetPropertyCommand struct {
	ApplicationCode string  `json:"applicationCode"`
	Section         string  `json:"section"`
	Property        string  `json:"property"`
	Value           string  `json:"value"`
	ValueType       *string `json:"valueType,omitempty"`
	Description     *string `json:"description,omitempty"`
	// ClientID is the canonical client id (callers resolve an identifier
	// first); nil for the application-wide value.
	ClientID *string `json:"clientId,omitempty"`
	// OnBehalfOf is who a service account acted for (X-On-Behalf-Of, see
	// platformconfig.OnBehalfOf), recorded in the audit row.
	OnBehalfOf *string `json:"onBehalfOf,omitempty"`
}

// AuditMaskedFields hides the value from the audit row unless the command
// says PLAIN explicitly — a nil type keeps the current type, which may be
// SECRET (the Java platform's SetPropertyCommand does the same).
func (c SetPropertyCommand) AuditMaskedFields() []string {
	if c.ValueType != nil && *c.ValueType == "PLAIN" {
		return nil
	}
	return []string{"value"}
}

// MaskedSecret is what every ordinary read returns for a SECRET's value. It is
// never accepted as a SECRET's new value: written back, it would replace the
// secret with the mask.
const MaskedSecret = "***"

// SetProperty upserts the (app, section, property, scope, client_id)
// coordinate with the supplied value and emits [PropertySet].
//
// Its Authorize phase is the whole write rule (platformconfig.Service.
// AuthorizeSet), whoever runs it: the application confinement of an
// application-scoped caller, config:update for an anchor-tier caller (anchor
// scope is reach, not authority) or write access through the role grants
// below anchor tier, and the client confinement (an application's service
// account to its linked clients, a caller below anchor tier to its own
// clients and never the application-wide value). A value for an application
// code that names no application is refused (404), and a SECRET's value may
// not be the mask "***" or empty (400).
func SetProperty(repo *platformconfig.Repository) usecaseop.Operation[SetPropertyCommand, PropertySet] {
	svc := &platformconfig.Service{Repo: repo}
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
			if cmd.ValueType != nil {
				vt, ok := platformconfig.ParseValueType(*cmd.ValueType)
				if !ok {
					return usecase.Validation("INVALID_VALUE_TYPE", "valueType must be PLAIN or SECRET")
				}
				if vt == platformconfig.ValueSecret {
					return validSecretValue(cmd.Value)
				}
			}
			return nil
		},
		Authorize: func(ctx context.Context, cmd SetPropertyCommand) error {
			var clientID string
			if cmd.ClientID != nil {
				clientID = *cmd.ClientID
			}
			return svc.AuthorizeSet(ctx, auth.FromContext(ctx), cmd.ApplicationCode, clientID)
		},
		Execute: func(ctx context.Context, cmd SetPropertyCommand, ec usecase.ExecutionContext) (usecaseop.Plan[PropertySet], error) {
			if err := svc.RequireApplication(ctx, cmd.ApplicationCode); err != nil {
				return nil, err
			}
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
			} else {
				c = platformconfig.NewConfig(cmd.ApplicationCode, cmd.Section, cmd.Property, cmd.Value)
				c.Scope = scope
				c.ClientID = ids.PtrOf[ids.ClientID](cmd.ClientID)
				c.Description = cmd.Description
			}
			if cmd.ValueType != nil {
				// Already restricted to a known value in Validate above.
				vt, ok := platformconfig.ParseValueType(*cmd.ValueType)
				if !ok {
					return nil, usecase.Internal("INVARIANT_VALUE_TYPE", "validated valueType failed to parse", nil)
				}
				c.ValueType = vt
			}
			// A SECRET kept by leaving valueType out is checked here, where the
			// stored type is known.
			if c.ValueType == platformconfig.ValueSecret {
				if err := validSecretValue(cmd.Value); err != nil {
					return nil, err
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

// validSecretValue refuses a SECRET value that is the mask every read
// returns, or empty: either written back would destroy the secret. A caller
// that does not mean to change a secret does not set it.
func validSecretValue(v string) error {
	switch strings.TrimSpace(v) {
	case MaskedSecret:
		return usecase.Validation("SECRET_VALUE_MASKED",
			`"***" is the mask a read returns for a secret, not its value; send the new secret value, or do not set the property to leave it unchanged`)
	case "":
		return usecase.Validation("SECRET_VALUE_REQUIRED", "a secret's value is required")
	}
	return nil
}
