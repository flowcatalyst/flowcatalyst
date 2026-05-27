package operations

import (
	"context"
	"regexp"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

var identifierPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$|^[a-z0-9]$`)

// CreateCommand is the input DTO.
type CreateCommand struct {
	Name       string `json:"name"`
	Identifier string `json:"identifier"`
}

// CreateClient validates cmd, enforces identifier uniqueness, persists
// the client, and atomically emits [ClientCreated].
func CreateClient(
	ctx context.Context,
	repo *client.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd CreateCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[ClientCreated], error) {
	var zero commit.Committed[ClientCreated]

	if strings.TrimSpace(cmd.Name) == "" {
		return zero, usecase.Validation("NAME_REQUIRED", "name is required")
	}
	id := strings.ToLower(strings.TrimSpace(cmd.Identifier))
	if id == "" {
		return zero, usecase.Validation("IDENTIFIER_REQUIRED", "identifier is required")
	}
	if !identifierPattern.MatchString(id) {
		return zero, usecase.Validation("INVALID_IDENTIFIER",
			"identifier must be lowercase alphanumeric with optional hyphens (URL-safe)")
	}

	existing, err := repo.FindByIdentifier(ctx, id)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_identifier failed", err)
	}
	if existing != nil {
		return zero, usecase.Conflict(
			"IDENTIFIER_EXISTS", "Client with identifier '"+id+"' already exists")
	}
	c := client.New(strings.TrimSpace(cmd.Name), id)

	event := ClientCreated{
		Metadata:   usecase.NewEventMetadata(ec, ClientCreatedType, Source, subjectFor(c.ID)),
		ClientID:   c.ID,
		Name:       c.Name,
		Identifier: c.Identifier,
	}
	return commit.Save(ctx, uow, c, repo, event, cmd)
}
