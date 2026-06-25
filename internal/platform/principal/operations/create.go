package operations

import (
	"context"
	"regexp"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/passwordhash"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

var emailPattern = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// CreateCommand is the input DTO.
type CreateCommand struct {
	Email    string  `json:"email"`
	Name     *string `json:"name,omitempty"`
	Scope    string  `json:"scope"`
	ClientID *string `json:"clientId,omitempty"`
	Password *string `json:"password,omitempty"`
	IDPType  *string `json:"idpType,omitempty"`
}

// CreateUser creates a user principal and emits [UserCreated].
//
// Authorize is intentionally Public: this op is invoked by TWO entry points
// with DIFFERENT gating — the admin controller (coarse CanCreatePrincipals +
// per-resource RequireUserAdmin against the resolved client + the
// "client-admins create CLIENT-scope only" rule, done in the handler) AND the
// UNAUTHENTICATED login JIT-provisioning flow (auth/bridge, which provisions a
// just-authenticated federated user). Baking an admin check in here would break
// login JIT provisioning, so authorization stays at each caller.
func CreateUser(repo *principal.Repository) usecaseop.Operation[CreateCommand, UserCreated] {
	return usecaseop.Operation[CreateCommand, UserCreated]{
		Name: "CreateUser",
		Validate: func(_ context.Context, cmd CreateCommand) error {
			email := strings.ToLower(strings.TrimSpace(cmd.Email))
			if email == "" {
				return usecase.Validation("EMAIL_REQUIRED", "email is required")
			}
			if !emailPattern.MatchString(email) {
				return usecase.Validation("INVALID_EMAIL", "email must be a valid address")
			}
			switch cmd.Scope {
			case "ANCHOR", "PARTNER", "CLIENT":
			default:
				return usecase.Validation("INVALID_SCOPE", "scope must be ANCHOR, PARTNER, or CLIENT")
			}
			if (cmd.Scope == "CLIENT" || cmd.Scope == "PARTNER") && cmd.ClientID == nil {
				return usecase.Validation("CLIENT_REQUIRED", "clientId is required for PARTNER or CLIENT scope")
			}
			return nil
		},
		Authorize: usecaseop.Public[CreateCommand],
		Execute: func(ctx context.Context, cmd CreateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[UserCreated], error) {
			email := strings.ToLower(strings.TrimSpace(cmd.Email))

			existing, err := repo.FindByEmail(ctx, email)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_email failed", err)
			}
			if existing != nil {
				return nil, usecase.Conflict("EMAIL_EXISTS", "User with email '"+email+"' already exists")
			}

			p := principal.NewUser(email, principal.ParseScope(cmd.Scope))
			p.ClientID = cmd.ClientID
			if cmd.Name != nil {
				p.Name = strings.TrimSpace(*cmd.Name)
			}
			if cmd.Password != nil && *cmd.Password != "" {
				hash, err := passwordhash.Hash(*cmd.Password)
				if err != nil {
					return nil, usecase.Internal("HASH", "password hash failed", err)
				}
				p.SetPasswordHash(hash)
			}
			if cmd.IDPType != nil && *cmd.IDPType == "OIDC" {
				if p.UserIdentity != nil {
					p.UserIdentity.PasswordHash = nil
					provider := "OIDC"
					p.UserIdentity.Provider = &provider
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
