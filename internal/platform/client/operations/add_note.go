package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// AddNoteCommand is the input DTO.
type AddNoteCommand struct {
	ClientID string `json:"clientId"`
	Category string `json:"category"`
	Text     string `json:"text"`
}

// AddNote appends a note to a client and emits [ClientNoteAdded].
func AddNote(
	ctx context.Context,
	repo *client.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd AddNoteCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[ClientNoteAdded], error) {
	var zero commit.Committed[ClientNoteAdded]

	if strings.TrimSpace(cmd.ClientID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "clientId is required")
	}
	if strings.TrimSpace(cmd.Category) == "" {
		return zero, usecase.Validation("CATEGORY_REQUIRED", "category is required")
	}
	if strings.TrimSpace(cmd.Text) == "" {
		return zero, usecase.Validation("TEXT_REQUIRED", "text is required")
	}

	c, err := repo.FindByID(ctx, cmd.ClientID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if c == nil {
		return zero, httperror.NotFound("Client", cmd.ClientID)
	}
	c.AddNote(client.NewNote(cmd.Category, cmd.Text, &ec.PrincipalID))
	event := ClientNoteAdded{
		Metadata: usecase.NewEventMetadata(ec, ClientNoteAddedType, Source, subjectFor(c.ID)),
		ClientID: c.ID,
		Category: cmd.Category,
		Text:     cmd.Text,
	}
	return commit.Save(ctx, uow, c, repo, event, cmd)
}
