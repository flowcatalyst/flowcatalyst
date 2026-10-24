package operations

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
)

// CreateCommand is the input DTO. ClientID is the resolved client id: the
// caller turns the request's id-or-identifier into it.
type CreateCommand struct {
	// Application is the owning application's code; it must be INTEGRATION.
	Application string               `json:"application"`
	ClientID    ids.ClientID         `json:"clientId"`
	Code        string               `json:"code"`
	Definition  ingestion.Definition `json:"definition"`
	// Live makes the first version the one runtimes receive; otherwise it
	// waits to be made live.
	Live bool    `json:"live"`
	Note *string `json:"note,omitempty"`
}

// CreateIngestion validates the definition, resolves its application and
// target, enforces (application, client, code) uniqueness, saves the
// ingestion with its first version and emits [IngestionCreated].
func CreateIngestion(repo *ingestion.Repository, resolver ingestion.Resolver) usecaseop.Operation[CreateCommand, IngestionCreated] {
	return usecaseop.Operation[CreateCommand, IngestionCreated]{
		Name: "CreateIngestion",
		Validate: func(_ context.Context, cmd CreateCommand) error {
			problems := append(ingestion.ValidateCode(cmd.Code), cmd.Definition.Validate()...)
			if cmd.Application == "" {
				problems = append(problems, ingestion.FieldError{Field: "application", Code: "NOT_NULL", Message: "is required"})
			}
			return problems.Err()
		},
		// Resource-level authorization: the caller must reach the client. The
		// application's reach is checked once it is resolved, in Execute; the
		// coarse "may create ingestions" permission is the controller's.
		Authorize: func(ctx context.Context, cmd CreateCommand) error {
			return auth.CheckScopeAccess(auth.FromContext(ctx), new(string(cmd.ClientID)))
		},
		Execute: func(ctx context.Context, cmd CreateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[IngestionCreated], error) {
			ac := auth.FromContext(ctx)
			app, err := resolver.Application(ctx, cmd.Application)
			if err != nil {
				return nil, err
			}
			if err := ingestion.CheckApplicationReach(ac, string(app.ID)); err != nil {
				return nil, err
			}
			if err := requireKindPermission(ctx, repo, cmd.Definition.Source); err != nil {
				return nil, err
			}
			if _, err := resolver.Target(ctx, app.Code, cmd.ClientID, cmd.Definition.Target); err != nil {
				return nil, err
			}
			existing, err := repo.FindByCode(ctx, app.ID, cmd.ClientID, cmd.Code)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_code failed", err)
			}
			if existing != nil {
				return nil, usecase.Conflict("INGESTION_CODE_EXISTS",
					"an ingestion with code '"+cmd.Code+"' already exists for this application and client")
			}

			i := ingestion.New(app.ID, cmd.ClientID, cmd.Code, cmd.Definition, cmd.Live, cmd.Note, new(ec.PrincipalID))
			event := IngestionCreated{
				ingestionEvent: newEvent(ec, IngestionCreatedType, i.ID),
				ApplicationID:  string(i.ApplicationID),
				ClientID:       string(i.ClientID),
				Code:           i.Code,
				Version:        i.Version,
			}
			return usecaseop.Save(i, repo, event), nil
		},
	}
}
