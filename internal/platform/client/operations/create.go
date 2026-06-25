package operations

import (
	"context"
	"regexp"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

var identifierPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$|^[a-z0-9]$`)

// CreateCommand is the input DTO.
type CreateCommand struct {
	Name       string `json:"name"`
	Identifier string `json:"identifier"`
}

// CreateClient validates cmd, enforces identifier uniqueness, persists
// the client, and atomically emits [ClientCreated]. The coarse anchor-only
// permission (CanCreateClients) is enforced at the controller; tenant
// management has no per-resource dimension, so the use case is Public.
func CreateClient(repo *client.Repository) usecaseop.Operation[CreateCommand, ClientCreated] {
	return usecaseop.Operation[CreateCommand, ClientCreated]{
		Name: "CreateClient",
		Validate: func(_ context.Context, cmd CreateCommand) error {
			if strings.TrimSpace(cmd.Name) == "" {
				return usecase.Validation("NAME_REQUIRED", "name is required")
			}
			id := strings.ToLower(strings.TrimSpace(cmd.Identifier))
			if id == "" {
				return usecase.Validation("IDENTIFIER_REQUIRED", "identifier is required")
			}
			if !identifierPattern.MatchString(id) {
				return usecase.Validation("INVALID_IDENTIFIER",
					"identifier must be lowercase alphanumeric with optional hyphens (URL-safe)")
			}
			return nil
		},
		Authorize: usecaseop.Public[CreateCommand],
		Execute: func(ctx context.Context, cmd CreateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ClientCreated], error) {
			id := strings.ToLower(strings.TrimSpace(cmd.Identifier))

			existing, err := repo.FindByIdentifier(ctx, id)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_identifier failed", err)
			}
			if existing != nil {
				return nil, usecase.Conflict(
					"IDENTIFIER_EXISTS", "Client with identifier '"+id+"' already exists")
			}
			c := client.New(strings.TrimSpace(cmd.Name), id)

			event := ClientCreated{
				Metadata:   usecase.NewEventMetadata(ec, ClientCreatedType, Source, subjectFor(c.ID)),
				ClientID:   c.ID,
				Name:       c.Name,
				Identifier: c.Identifier,
			}
			return usecaseop.Save(c, repo, event), nil
		},
	}
}
