package operations

import (
	"context"
	"regexp"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

var codePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// CreateCommand is the input DTO.
type CreateCommand struct {
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	RateLimit   *int32  `json:"rateLimit,omitempty"`
	Concurrency *int32  `json:"concurrency,omitempty"`
	ClientID    *string `json:"clientId,omitempty"`
}

// CreateUseCase implements UseCase.
type CreateUseCase struct {
	repo *dispatchpool.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewCreateUseCase wires the use case.
func NewCreateUseCase(repo *dispatchpool.Repository, uow *usecasepgx.UnitOfWork) *CreateUseCase {
	return &CreateUseCase{repo: repo, uow: uow}
}

func (uc *CreateUseCase) Validate(_ context.Context, cmd CreateCommand) error {
	code := strings.ToLower(strings.TrimSpace(cmd.Code))
	if code == "" {
		return usecase.Validation("CODE_REQUIRED", "code is required")
	}
	if !codePattern.MatchString(code) {
		return usecase.Validation("INVALID_CODE_FORMAT",
			"code must start with a lowercase letter and contain only lowercase alphanumeric and hyphens")
	}
	if strings.TrimSpace(cmd.Name) == "" {
		return usecase.Validation("NAME_REQUIRED", "name is required")
	}
	if cmd.Concurrency != nil && *cmd.Concurrency < 1 {
		return usecase.Validation("INVALID_CONCURRENCY", "concurrency must be >= 1")
	}
	if cmd.RateLimit != nil && *cmd.RateLimit < 0 {
		return usecase.Validation("INVALID_RATE_LIMIT", "rateLimit cannot be negative")
	}
	return nil
}

func (uc *CreateUseCase) Authorize(_ context.Context, _ CreateCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *CreateUseCase) Execute(ctx context.Context, cmd CreateCommand, ec usecase.ExecutionContext) usecase.Result[DispatchPoolCreated] {
	code := strings.ToLower(strings.TrimSpace(cmd.Code))

	existing, err := uc.repo.FindByCode(ctx, code, cmd.ClientID)
	if err != nil {
		return usecase.Failure[DispatchPoolCreated](usecase.Internal("REPO", "find_by_code failed", err))
	}
	if existing != nil {
		return usecase.Failure[DispatchPoolCreated](usecase.Conflict(
			"CODE_EXISTS",
			"Dispatch pool with code '"+code+"' already exists"))
	}

	p := dispatchpool.New(code, strings.TrimSpace(cmd.Name))
	p.Description = cmd.Description
	p.RateLimit = cmd.RateLimit
	if cmd.Concurrency != nil {
		p.Concurrency = *cmd.Concurrency
	}
	p.ClientID = cmd.ClientID

	event := DispatchPoolCreated{
		Metadata: usecase.NewEventMetadata(ec, DispatchPoolCreatedType, Source, subjectFor(p.ID)),
		PoolID:   p.ID,
		Code:     p.Code,
		Name:     p.Name,
	}
	return usecasepgx.Commit[dispatchpool.DispatchPool, DispatchPoolCreated, CreateCommand](
		ctx, uc.uow, p, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[CreateCommand, DispatchPoolCreated] = (*CreateUseCase)(nil)
