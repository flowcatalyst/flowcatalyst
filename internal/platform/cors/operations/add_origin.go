package operations

import (
	"context"
	"regexp"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/cors"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// originPattern matches the Rust regex: ^https?://[a-zA-Z0-9*]([a-zA-Z0-9*.-]*[a-zA-Z0-9*])?(:\d+)?$
var originPattern = regexp.MustCompile(`^https?://[a-zA-Z0-9*]([a-zA-Z0-9*.-]*[a-zA-Z0-9*])?(:\d+)?$`)

// AddCommand is the input DTO.
type AddCommand struct {
	Origin      string  `json:"origin"`
	Description *string `json:"description,omitempty"`
}

// AddOrigin validates cmd, enforces origin uniqueness, registers a new
// CORS-allowed origin, and emits CorsOriginAdded. The coarse anchor check lives
// on the controller; CORS origins have no per-client resource dimension, so the
// use case carries no resource-level authz (Authorize = usecaseop.Public).
func AddOrigin(repo *cors.Repository) usecaseop.Operation[AddCommand, CorsOriginAdded] {
	return usecaseop.Operation[AddCommand, CorsOriginAdded]{
		Name: "AddOrigin",
		Validate: func(_ context.Context, cmd AddCommand) error {
			origin := strings.TrimSpace(cmd.Origin)
			if origin == "" {
				return usecase.Validation("ORIGIN_REQUIRED", "Origin is required")
			}
			if !originPattern.MatchString(origin) {
				return usecase.Validation("INVALID_ORIGIN_FORMAT",
					"Origin must be a valid URL (e.g. https://example.com or http://localhost:3000)")
			}
			return nil
		},
		Authorize: usecaseop.Public[AddCommand],
		Execute: func(ctx context.Context, cmd AddCommand, ec usecase.ExecutionContext) (usecaseop.Plan[CorsOriginAdded], error) {
			origin := strings.TrimSpace(cmd.Origin)

			existing, err := repo.FindByOrigin(ctx, origin)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_origin failed", err)
			}
			if existing != nil {
				return nil, usecase.Conflict("ORIGIN_ALREADY_EXISTS",
					"CORS origin '"+origin+"' already exists")
			}

			entity := cors.New(origin, cmd.Description, &ec.PrincipalID)
			event := CorsOriginAdded{
				Metadata: usecase.NewEventMetadata(ec, CorsOriginAddedType, CorsSource, subjectFor(entity.ID)),
				OriginID: entity.ID,
				Origin:   entity.Origin,
			}
			return usecaseop.Save(entity, repo, event), nil
		},
	}
}
