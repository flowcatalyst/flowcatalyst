package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// AddNoteCommand is the input DTO.
type AddNoteCommand struct {
	ClientID string `json:"clientId"`
	Category string `json:"category"`
	Text     string `json:"text"`
}

// AddNote appends a note to a client and emits [ClientNoteAdded].
func AddNote(repo *client.Repository) usecaseop.Operation[AddNoteCommand, ClientNoteAdded] {
	return usecaseop.Operation[AddNoteCommand, ClientNoteAdded]{
		Name: "AddNote",
		Validate: func(_ context.Context, cmd AddNoteCommand) error {
			if strings.TrimSpace(cmd.ClientID) == "" {
				return usecase.Validation("ID_REQUIRED", "clientId is required")
			}
			if strings.TrimSpace(cmd.Category) == "" {
				return usecase.Validation("CATEGORY_REQUIRED", "category is required")
			}
			if strings.TrimSpace(cmd.Text) == "" {
				return usecase.Validation("TEXT_REQUIRED", "text is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[AddNoteCommand],
		Execute: func(ctx context.Context, cmd AddNoteCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ClientNoteAdded], error) {
			c, err := repo.FindByID(ctx, cmd.ClientID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if c == nil {
				return nil, httperror.NotFound("Client", cmd.ClientID)
			}
			c.AddNote(client.NewNote(cmd.Category, cmd.Text, &ec.PrincipalID))
			event := ClientNoteAdded{
				Metadata: usecase.NewEventMetadata(ec, ClientNoteAddedType, Source, subjectFor(c.ID)),
				ClientID: c.ID,
				Category: cmd.Category,
				Text:     cmd.Text,
			}
			return usecaseop.Save(c, repo, event), nil
		},
	}
}
