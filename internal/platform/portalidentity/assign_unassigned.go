package portalidentity

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

type AssignUnassignedCommand struct {
	ClientID    string `json:"clientId"`
	PortalAppID string `json:"portalAppId"`
}

// AssignUnassignedResult reports who was assigned.
type AssignUnassignedResult struct {
	AppID       string
	AppCode     string
	IdentityIDs []string
}

// AssignUnassignedToApp grants a portal app to every one of the client's
// portal identities that holds NO portal app — the gap left by identities
// that predate portal apps (or were created without one): once their portal
// OAuth client is linked to an app, the login gate would refuse them. One
// transaction, one [IdentityAppGranted] (source ADMIN) per identity.
// Identities that already hold any grant are untouched; status is not
// changed (a suspended identity is assigned but stays suspended).
func AssignUnassignedToApp(repo *Repository, apps *AppRepository) usecaseop.TxOperation[AssignUnassignedCommand, AssignUnassignedResult] {
	return usecaseop.TxOperation[AssignUnassignedCommand, AssignUnassignedResult]{
		Name: "AssignUnassignedPortalIdentitiesToApp",
		Validate: func(_ context.Context, cmd AssignUnassignedCommand) error {
			if strings.TrimSpace(cmd.ClientID) == "" || strings.TrimSpace(cmd.PortalAppID) == "" {
				return usecase.Validation("TARGET_REQUIRED", "clientId and portalAppId are required")
			}
			return nil
		},
		Authorize: usecaseop.Public[AssignUnassignedCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd AssignUnassignedCommand, ec usecase.ExecutionContext) (AssignUnassignedResult, error) {
			var zero AssignUnassignedResult
			app, err := loadClientApp(ctx, apps, cmd.ClientID, cmd.PortalAppID)
			if err != nil {
				return zero, err
			}
			idents, err := repo.FindUnassigned(ctx, cmd.ClientID)
			if err != nil {
				return zero, usecase.Internal("REPO", "find_unassigned failed", err)
			}
			res := AssignUnassignedResult{AppID: app.ID, AppCode: app.Code, IdentityIDs: []string{}}
			for n := range idents {
				ident := &idents[n]
				ident.Grant(app.ID, SourceAdmin)
				event := IdentityAppGranted{
					Metadata:   usecase.NewEventMetadata(ec, IdentityAppGrantedType, EventSource, subjectFor(ident.ID)),
					IdentityID: ident.ID, ClientID: ident.ClientID,
					AppID: app.ID, AppCode: app.Code, Source_: string(SourceAdmin),
				}
				if r := usecasepgx.CommitScoped(ctx, s, ident, repo, event, cmd); !usecase.IsSuccess(r) {
					_, e := usecase.Into(r)
					return zero, e
				}
				res.IdentityIDs = append(res.IdentityIDs, ident.ID)
			}
			return res, nil
		},
	}
}
