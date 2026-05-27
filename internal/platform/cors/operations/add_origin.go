package operations

import (
	"context"
	"regexp"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/cors"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
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

// AddOrigin registers a new CORS-allowed origin and emits CorsOriginAdded.
func AddOrigin(
	ctx context.Context,
	repo *cors.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd AddCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[CorsOriginAdded], error) {
	var zero commit.Committed[CorsOriginAdded]

	origin := strings.TrimSpace(cmd.Origin)
	if origin == "" {
		return zero, usecase.Validation("ORIGIN_REQUIRED", "Origin is required")
	}
	if !originPattern.MatchString(origin) {
		return zero, usecase.Validation("INVALID_ORIGIN_FORMAT",
			"Origin must be a valid URL (e.g. https://example.com or http://localhost:3000)")
	}

	existing, err := repo.FindByOrigin(ctx, origin)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_origin failed", err)
	}
	if existing != nil {
		return zero, usecase.Conflict("ORIGIN_ALREADY_EXISTS",
			"CORS origin '"+origin+"' already exists")
	}

	entity := cors.New(origin, cmd.Description, &ec.PrincipalID)
	event := CorsOriginAdded{
		Metadata: usecase.NewEventMetadata(ec, CorsOriginAddedType, CorsSource, subjectFor(entity.ID)),
		OriginID: entity.ID,
		Origin:   entity.Origin,
	}
	return commit.Save(ctx, uow, entity, repo, event, cmd)
}
