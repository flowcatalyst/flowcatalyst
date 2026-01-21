package operations

import (
	"context"
	"strings"

	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/events"
	"go.flowcatalyst.tech/internal/platform/principal"
)

// CreateUserCommand contains the data needed to create a user
type CreateUserCommand struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Scope    string `json:"scope"`
	ClientID string `json:"clientId,omitempty"`
}

// CreateUserUseCase handles creating a new user
type CreateUserUseCase struct {
	repo       principal.Repository
	unitOfWork common.UnitOfWork
}

// NewCreateUserUseCase creates a new CreateUserUseCase
func NewCreateUserUseCase(repo principal.Repository, uow common.UnitOfWork) *CreateUserUseCase {
	return &CreateUserUseCase{
		repo:       repo,
		unitOfWork: uow,
	}
}

// Execute creates a new user
func (uc *CreateUserUseCase) Execute(
	ctx context.Context,
	cmd CreateUserCommand,
	execCtx *common.ExecutionContext,
) common.Result[common.DomainEvent] {
	// Validation
	if cmd.Email == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_EMAIL", "Email is required", nil),
		)
	}

	if cmd.Name == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_NAME", "Name is required", nil),
		)
	}

	email := strings.ToLower(strings.TrimSpace(cmd.Email))
	if !strings.Contains(email, "@") {
		return common.Failure[common.DomainEvent](
			common.ValidationError("INVALID_EMAIL", "Invalid email format", nil),
		)
	}

	// Extract email domain
	parts := strings.Split(email, "@")
	emailDomain := parts[1]

	// Validate scope
	scope := principal.UserScope(cmd.Scope)
	if scope == "" {
		scope = principal.UserScopeClient // Default
	}

	switch scope {
	case principal.UserScopeAnchor, principal.UserScopePartner, principal.UserScopeClient:
		// Valid
	default:
		return common.Failure[common.DomainEvent](
			common.ValidationError("INVALID_SCOPE",
				"Scope must be ANCHOR, PARTNER, or CLIENT",
				map[string]any{"scope": cmd.Scope}),
		)
	}

	// CLIENT scope requires clientId
	if scope == principal.UserScopeClient && cmd.ClientID == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_CLIENT_ID",
				"Client ID is required for CLIENT scope users",
				nil),
		)
	}

	// Check for existing user with same email
	exists, err := uc.repo.ExistsByEmail(ctx, email)
	if err != nil {
		return common.Failure[common.DomainEvent](
			common.InternalError("DB_ERROR", "Failed to check for existing user", map[string]any{"error": err.Error()}),
		)
	}
	if exists {
		return common.Failure[common.DomainEvent](
			common.BusinessRuleError("EMAIL_EXISTS",
				"A user with this email already exists",
				map[string]any{"email": email}),
		)
	}

	// Create user
	p := &principal.Principal{
		Type:     principal.PrincipalTypeUser,
		Scope:    scope,
		ClientID: cmd.ClientID,
		Name:     cmd.Name,
		Active:   true,
		UserIdentity: &principal.UserIdentity{
			Email:       email,
			EmailDomain: emailDomain,
			IdpType:     principal.IdpTypeInternal,
		},
	}

	// Create domain event
	event := events.NewPrincipalUserCreated(execCtx, p)

	// Atomic commit
	if cmd.ClientID != "" {
		return uc.unitOfWork.CommitWithClientID(ctx, p, event, cmd, cmd.ClientID)
	}
	return uc.unitOfWork.Commit(ctx, p, event, cmd)
}
