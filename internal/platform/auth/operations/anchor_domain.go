package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// ── Create ────────────────────────────────────────────────────────────────

type CreateAnchorDomainCommand struct {
	Domain string `json:"domain"`
}

type CreateAnchorDomainUseCase struct {
	repo *auth.AnchorDomainRepo
	uow  *usecasepgx.UnitOfWork
}

func NewCreateAnchorDomainUseCase(repo *auth.AnchorDomainRepo, uow *usecasepgx.UnitOfWork) *CreateAnchorDomainUseCase {
	return &CreateAnchorDomainUseCase{repo: repo, uow: uow}
}

func (uc *CreateAnchorDomainUseCase) Validate(_ context.Context, cmd CreateAnchorDomainCommand) error {
	d := strings.ToLower(strings.TrimSpace(cmd.Domain))
	if d == "" {
		return usecase.Validation("DOMAIN_REQUIRED", "domain is required")
	}
	if !strings.Contains(d, ".") || strings.ContainsAny(d, " /@") {
		return usecase.Validation("INVALID_DOMAIN", "domain must be a valid DNS name (e.g. example.com)")
	}
	return nil
}

func (uc *CreateAnchorDomainUseCase) Authorize(_ context.Context, _ CreateAnchorDomainCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *CreateAnchorDomainUseCase) Execute(ctx context.Context, cmd CreateAnchorDomainCommand, ec usecase.ExecutionContext) usecase.Result[AnchorDomainCreated] {
	d := strings.ToLower(strings.TrimSpace(cmd.Domain))
	existing, err := uc.repo.FindByDomain(ctx, d)
	if err != nil {
		return usecase.Failure[AnchorDomainCreated](usecase.Internal("REPO", "find_by_domain failed", err))
	}
	if existing != nil {
		return usecase.Failure[AnchorDomainCreated](usecase.Conflict(
			"DOMAIN_EXISTS", "Anchor domain '"+d+"' already exists"))
	}
	a := auth.NewAnchorDomain(d)
	event := AnchorDomainCreated{
		Metadata:       usecase.NewEventMetadata(ec, AnchorDomainCreatedType, Source, anchorSubject(a.ID)),
		AnchorDomainID: a.ID,
		Domain:         a.Domain,
	}
	return usecasepgx.Commit[auth.AnchorDomain, AnchorDomainCreated, CreateAnchorDomainCommand](
		ctx, uc.uow, a, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[CreateAnchorDomainCommand, AnchorDomainCreated] = (*CreateAnchorDomainUseCase)(nil)

// ── Update ────────────────────────────────────────────────────────────────

type UpdateAnchorDomainCommand struct {
	ID     string `json:"id"`
	Domain string `json:"domain"`
}

type UpdateAnchorDomainUseCase struct {
	repo *auth.AnchorDomainRepo
	uow  *usecasepgx.UnitOfWork
}

func NewUpdateAnchorDomainUseCase(repo *auth.AnchorDomainRepo, uow *usecasepgx.UnitOfWork) *UpdateAnchorDomainUseCase {
	return &UpdateAnchorDomainUseCase{repo: repo, uow: uow}
}

func (uc *UpdateAnchorDomainUseCase) Validate(_ context.Context, cmd UpdateAnchorDomainCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	d := strings.ToLower(strings.TrimSpace(cmd.Domain))
	if d == "" || !strings.Contains(d, ".") || strings.ContainsAny(d, " /@") {
		return usecase.Validation("INVALID_DOMAIN", "domain must be a valid DNS name")
	}
	return nil
}

func (uc *UpdateAnchorDomainUseCase) Authorize(_ context.Context, _ UpdateAnchorDomainCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *UpdateAnchorDomainUseCase) Execute(ctx context.Context, cmd UpdateAnchorDomainCommand, ec usecase.ExecutionContext) usecase.Result[AnchorDomainUpdated] {
	a, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[AnchorDomainUpdated](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if a == nil {
		return usecase.Failure[AnchorDomainUpdated](httperror.NotFound("AnchorDomain", cmd.ID))
	}
	a.Domain = strings.ToLower(strings.TrimSpace(cmd.Domain))
	event := AnchorDomainUpdated{
		Metadata:       usecase.NewEventMetadata(ec, AnchorDomainUpdatedType, Source, anchorSubject(a.ID)),
		AnchorDomainID: a.ID,
		Domain:         a.Domain,
	}
	return usecasepgx.Commit[auth.AnchorDomain, AnchorDomainUpdated, UpdateAnchorDomainCommand](
		ctx, uc.uow, a, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[UpdateAnchorDomainCommand, AnchorDomainUpdated] = (*UpdateAnchorDomainUseCase)(nil)

// ── Delete ────────────────────────────────────────────────────────────────

type DeleteAnchorDomainCommand struct {
	ID string `json:"id"`
}

type DeleteAnchorDomainUseCase struct {
	repo *auth.AnchorDomainRepo
	uow  *usecasepgx.UnitOfWork
}

func NewDeleteAnchorDomainUseCase(repo *auth.AnchorDomainRepo, uow *usecasepgx.UnitOfWork) *DeleteAnchorDomainUseCase {
	return &DeleteAnchorDomainUseCase{repo: repo, uow: uow}
}

func (uc *DeleteAnchorDomainUseCase) Validate(_ context.Context, cmd DeleteAnchorDomainCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	return nil
}

func (uc *DeleteAnchorDomainUseCase) Authorize(_ context.Context, _ DeleteAnchorDomainCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *DeleteAnchorDomainUseCase) Execute(ctx context.Context, cmd DeleteAnchorDomainCommand, ec usecase.ExecutionContext) usecase.Result[AnchorDomainDeleted] {
	a, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[AnchorDomainDeleted](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if a == nil {
		return usecase.Failure[AnchorDomainDeleted](httperror.NotFound("AnchorDomain", cmd.ID))
	}
	event := AnchorDomainDeleted{
		Metadata:       usecase.NewEventMetadata(ec, AnchorDomainDeletedType, Source, anchorSubject(a.ID)),
		AnchorDomainID: a.ID,
		Domain:         a.Domain,
	}
	return usecasepgx.CommitDelete[auth.AnchorDomain, AnchorDomainDeleted, DeleteAnchorDomainCommand](
		ctx, uc.uow, a, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[DeleteAnchorDomainCommand, AnchorDomainDeleted] = (*DeleteAnchorDomainUseCase)(nil)
