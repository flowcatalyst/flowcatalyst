package operations

import (
	"context"
	"regexp"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/passwordhash"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

var emailPattern = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// CreateCommand is the input DTO.
type CreateCommand struct {
	Email     string    `json:"email"`
	Name      *string   `json:"name,omitempty"`
	Scope     string    `json:"scope"`
	ClientID  *string   `json:"clientId,omitempty"`
	Password  *string   `json:"password,omitempty"`
	IDPType   *string   `json:"idpType,omitempty"`
}

// CreateUseCase implements UseCase.
type CreateUseCase struct {
	repo *principal.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewCreateUseCase wires the use case.
func NewCreateUseCase(repo *principal.Repository, uow *usecasepgx.UnitOfWork) *CreateUseCase {
	return &CreateUseCase{repo: repo, uow: uow}
}

func (uc *CreateUseCase) Validate(_ context.Context, cmd CreateCommand) error {
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
		return usecase.Validation("CLIENT_REQUIRED",
			"clientId is required for PARTNER or CLIENT scope")
	}
	return nil
}

func (uc *CreateUseCase) Authorize(_ context.Context, _ CreateCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *CreateUseCase) Execute(ctx context.Context, cmd CreateCommand, ec usecase.ExecutionContext) usecase.Result[UserCreated] {
	email := strings.ToLower(strings.TrimSpace(cmd.Email))
	existing, err := uc.repo.FindByEmail(ctx, email)
	if err != nil {
		return usecase.Failure[UserCreated](usecase.Internal("REPO", "find_by_email failed", err))
	}
	if existing != nil {
		return usecase.Failure[UserCreated](usecase.Conflict(
			"EMAIL_EXISTS", "User with email '"+email+"' already exists"))
	}

	p := principal.NewUser(email, principal.ParseScope(cmd.Scope))
	p.ClientID = cmd.ClientID
	if cmd.Name != nil {
		p.Name = strings.TrimSpace(*cmd.Name)
	}
	if cmd.Password != nil && *cmd.Password != "" {
		hash, err := passwordhash.Hash(*cmd.Password)
		if err != nil {
			return usecase.Failure[UserCreated](usecase.Internal("HASH", "password hash failed", err))
		}
		p.SetPasswordHash(hash)
	}
	if cmd.IDPType != nil && *cmd.IDPType == "OIDC" {
		// OIDC users authenticate through external IDP; no local password.
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
	return usecasepgx.Commit[principal.Principal, UserCreated, CreateCommand](
		ctx, uc.uow, p, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[CreateCommand, UserCreated] = (*CreateUseCase)(nil)
