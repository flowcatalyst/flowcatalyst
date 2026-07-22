package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// CreatePortalUserCommand is the input DTO for CreatePortalUser.
type CreatePortalUserCommand struct {
	Email string  `json:"email"`
	Name  *string `json:"name,omitempty"`
	// Provider records which auth path minted this identity (e.g. "OIDC").
	// Portal identities never carry a password hash.
	Provider *string `json:"provider,omitempty"`
}

// CreatePortalUser provisions a PORTAL identity: a USER principal that exists
// so the platform can authenticate the person (docs/portal-identity-plan.md)
// but is deliberately INERT everywhere else — scope CLIENT with NO client_id
// (CanAccessClient always false, buildClients empty), no roles, no
// application access, AllApplications=false. What such a user may do is
// defined entirely by the consuming portal app's own membership tables; the
// platform only proves who they are.
//
// This is a separate operation from CreateUser on purpose: CreateUser's
// "CLIENT scope requires clientId" validation is a real invariant for
// admin-created tenant users and must not be weakened to admit this shape.
//
// Authorize is Public for the same reason as CreateUser: the primary caller
// is the UNAUTHENTICATED provider-direct OIDC callback (JIT provisioning of a
// just-authenticated federated user). The Phase-2 ensure/invite API adds its
// own gating at the controller.
func CreatePortalUser(repo *principal.Repository) usecaseop.Operation[CreatePortalUserCommand, UserCreated] {
	return usecaseop.Operation[CreatePortalUserCommand, UserCreated]{
		Name: "CreatePortalUser",
		Validate: func(_ context.Context, cmd CreatePortalUserCommand) error {
			email := strings.ToLower(strings.TrimSpace(cmd.Email))
			if email == "" {
				return usecase.Validation("EMAIL_REQUIRED", "email is required")
			}
			if !emailPattern.MatchString(email) {
				return usecase.Validation("INVALID_EMAIL", "email must be a valid address")
			}
			return nil
		},
		Authorize: usecaseop.Public[CreatePortalUserCommand],
		Execute: func(ctx context.Context, cmd CreatePortalUserCommand, ec usecase.ExecutionContext) (usecaseop.Plan[UserCreated], error) {
			email := strings.ToLower(strings.TrimSpace(cmd.Email))

			existing, err := repo.FindByEmail(ctx, email)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_email failed", err)
			}
			if existing != nil {
				return nil, usecase.Conflict("EMAIL_EXISTS", "User with email '"+email+"' already exists")
			}

			p := principal.NewUser(email, principal.ScopeClient)
			p.ClientID = nil
			// NewUser defaults AllApplications=true (right for tenant users,
			// wrong here): a portal identity must not pass application-axis
			// checks either.
			p.AllApplications = false
			if cmd.Name != nil && strings.TrimSpace(*cmd.Name) != "" {
				p.Name = strings.TrimSpace(*cmd.Name)
			}
			if p.UserIdentity != nil {
				p.UserIdentity.PasswordHash = nil
				if cmd.Provider != nil && *cmd.Provider != "" {
					p.UserIdentity.Provider = cmd.Provider
				}
			}

			event := UserCreated{
				Metadata: usecase.NewEventMetadata(ec, UserCreatedType, Source, subjectFor(p.ID)),
				UserID:   p.ID,
				Email:    email,
			}
			return usecaseop.Save(p, repo, event), nil
		},
	}
}
