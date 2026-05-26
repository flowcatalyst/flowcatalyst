package operations

import (
	"context"
	"regexp"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/cors"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// originPattern matches the Rust regex: ^https?://[a-zA-Z0-9*]([a-zA-Z0-9*.-]*[a-zA-Z0-9*])?(:\d+)?$
var originPattern = regexp.MustCompile(`^https?://[a-zA-Z0-9*]([a-zA-Z0-9*.-]*[a-zA-Z0-9*])?(:\d+)?$`)

// AddCommand is the input DTO.
type AddCommand struct {
	Origin      string  `json:"origin"`
	Description *string `json:"description,omitempty"`
}

// AddUseCase implements UseCase[AddCommand, CorsOriginAdded].
type AddUseCase struct {
	repo *cors.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewAddUseCase wires the use case.
func NewAddUseCase(repo *cors.Repository, uow *usecasepgx.UnitOfWork) *AddUseCase {
	return &AddUseCase{repo: repo, uow: uow}
}

func (uc *AddUseCase) Validate(_ context.Context, cmd AddCommand) error {
	origin := strings.TrimSpace(cmd.Origin)
	if origin == "" {
		return usecase.Validation("ORIGIN_REQUIRED", "Origin is required")
	}
	if !originPattern.MatchString(origin) {
		return usecase.Validation("INVALID_ORIGIN_FORMAT",
			"Origin must be a valid URL (e.g. https://example.com or http://localhost:3000)")
	}
	return nil
}

func (uc *AddUseCase) Authorize(_ context.Context, _ AddCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *AddUseCase) Execute(ctx context.Context, cmd AddCommand, ec usecase.ExecutionContext) usecase.Result[CorsOriginAdded] {
	origin := strings.TrimSpace(cmd.Origin)

	existing, err := uc.repo.FindByOrigin(ctx, origin)
	if err != nil {
		return usecase.Failure[CorsOriginAdded](usecase.Internal("REPO", "find_by_origin failed", err))
	}
	if existing != nil {
		return usecase.Failure[CorsOriginAdded](usecase.Conflict(
			"ORIGIN_ALREADY_EXISTS",
			"CORS origin '"+origin+"' already exists"))
	}

	entity := cors.New(origin, cmd.Description, &ec.PrincipalID)
	event := CorsOriginAdded{
		Metadata: usecase.NewEventMetadata(ec, CorsOriginAddedType, CorsSource, subjectFor(entity.ID)),
		OriginID: entity.ID,
		Origin:   entity.Origin,
	}
	return usecasepgx.Commit[cors.AllowedOrigin, CorsOriginAdded, AddCommand](
		ctx, uc.uow, entity, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[AddCommand, CorsOriginAdded] = (*AddUseCase)(nil)
