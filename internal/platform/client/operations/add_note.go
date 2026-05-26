package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// AddNoteCommand is the input DTO.
type AddNoteCommand struct {
	ClientID string `json:"clientId"`
	Category string `json:"category"`
	Text     string `json:"text"`
}

// AddNoteUseCase implements UseCase.
type AddNoteUseCase struct {
	repo *client.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewAddNoteUseCase wires the use case.
func NewAddNoteUseCase(repo *client.Repository, uow *usecasepgx.UnitOfWork) *AddNoteUseCase {
	return &AddNoteUseCase{repo: repo, uow: uow}
}

func (uc *AddNoteUseCase) Validate(_ context.Context, cmd AddNoteCommand) error {
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
}

func (uc *AddNoteUseCase) Authorize(_ context.Context, _ AddNoteCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *AddNoteUseCase) Execute(ctx context.Context, cmd AddNoteCommand, ec usecase.ExecutionContext) usecase.Result[ClientNoteAdded] {
	c, err := uc.repo.FindByID(ctx, cmd.ClientID)
	if err != nil {
		return usecase.Failure[ClientNoteAdded](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if c == nil {
		return usecase.Failure[ClientNoteAdded](httperror.NotFound("Client", cmd.ClientID))
	}
	c.AddNote(client.NewNote(cmd.Category, cmd.Text, &ec.PrincipalID))
	event := ClientNoteAdded{
		Metadata: usecase.NewEventMetadata(ec, ClientNoteAddedType, Source, subjectFor(c.ID)),
		ClientID: c.ID,
		Category: cmd.Category,
		Text:     cmd.Text,
	}
	return usecasepgx.Commit[client.Client, ClientNoteAdded, AddNoteCommand](
		ctx, uc.uow, c, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[AddNoteCommand, ClientNoteAdded] = (*AddNoteUseCase)(nil)
