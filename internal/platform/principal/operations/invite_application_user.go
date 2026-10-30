package operations

import (
	"context"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// InviteApplicationUserCommand creates one CLIENT-scope user on behalf of an
// application, holding that application's roles (F20).
type InviteApplicationUserCommand struct {
	ApplicationCode string
	Email           string
	Name            string
	// ClientID is the resolved `clt_` id of the user's home client. The caller
	// has already checked that the application is enabled for it and that the
	// caller reaches it.
	ClientID string
	// Roles are the application's roles to give the user, canonical
	// (`app:role`) or short (`role`, prefixed with the application code).
	Roles []string
	// IDPType "OIDC" marks a user whose email domain is federated: it is
	// created without a password and is never sent a set-password invitation.
	IDPType *string
}

// AdminAssignedSource tags role assignments an administrator (or an
// application acting for one) made by hand, as opposed to an SDK sync's
// (SdkSyncSource), which a sync sweep may strip.
const AdminAssignedSource = "ADMIN_ASSIGNED"

// InviteApplicationUser creates a CLIENT-scope user principal with the
// application's roles in one transaction and emits [UserCreated]. The
// invitation itself (the email or the link) is the caller's, after commit,
// through the same invite mint the other user-creation routes use.
//
// The roles are what makes the user the application's: an application's users
// are the CLIENT users homed at a client the application is enabled for that
// hold one of its roles (appusers repository). So at least one role is
// required, every role must belong to the application (403 ROLE_APP_FORBIDDEN,
// the sync's code), and a name that is not a role is refused (400
// UNKNOWN_ROLE; never silently dropped, unlike the sync). The role ceiling
// (role.RequireRolesWithinCaller, F8) bounds the grant as it does for every
// role change.
//
// Authorize is Public: the route enforces the permission, the application
// binding and the client reach before this runs, as the other create routes do.
func InviteApplicationUser(principals *principal.Repository, roles *role.Repository) usecaseop.Operation[InviteApplicationUserCommand, UserCreated] {
	return usecaseop.Operation[InviteApplicationUserCommand, UserCreated]{
		Name: "InviteApplicationUser",
		Validate: func(_ context.Context, cmd InviteApplicationUserCommand) error {
			email := strings.ToLower(strings.TrimSpace(cmd.Email))
			if email == "" {
				return usecase.Validation("EMAIL_REQUIRED", "email is required")
			}
			if !emailPattern.MatchString(email) {
				return usecase.Validation("INVALID_EMAIL", "email must be a valid address")
			}
			if strings.TrimSpace(cmd.ClientID) == "" {
				return usecase.Validation("CLIENT_REQUIRED", "clientId is required")
			}
			if strings.TrimSpace(cmd.ApplicationCode) == "" {
				return usecase.Validation("APPLICATION_REQUIRED", "application is required")
			}
			if len(cmd.Roles) == 0 {
				return usecase.Validation("ROLES_REQUIRED", "at least one role of the application is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[InviteApplicationUserCommand],
		Execute: func(ctx context.Context, cmd InviteApplicationUserCommand, ec usecase.ExecutionContext) (usecaseop.Plan[UserCreated], error) {
			email := strings.ToLower(strings.TrimSpace(cmd.Email))
			existing, err := principals.FindByEmail(ctx, email)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_email failed", err)
			}
			if existing != nil {
				return nil, usecase.Conflict("EMAIL_EXISTS", "User with email '"+email+"' already exists")
			}

			names := make([]string, 0, len(cmd.Roles))
			for _, r := range cmd.Roles {
				n := strings.ToLower(strings.TrimSpace(r))
				if n != "" && !strings.Contains(n, ":") {
					n = strings.ToLower(cmd.ApplicationCode) + ":" + n
				}
				names = append(names, n)
			}
			assign, unknown, err := applicationRoles(ctx, roles, cmd.ApplicationCode, names)
			if err != nil {
				return nil, err
			}
			if len(unknown) > 0 {
				return nil, usecase.Validation("UNKNOWN_ROLE", "role '"+unknown[0]+"' does not exist")
			}
			assigned := make([]string, 0, len(assign))
			for _, a := range assign {
				assigned = append(assigned, a.String())
			}
			if err := role.RequireRolesWithinCaller(ctx, auth.FromContext(ctx), assigned,
				role.PermissionsFrom(roles)); err != nil {
				return nil, err
			}

			now := time.Now().UTC()
			src := AdminAssignedSource
			p := principal.NewUser(email, principal.ScopeClient)
			p.ClientID = ids.PtrOf[ids.ClientID](&cmd.ClientID)
			if n := strings.TrimSpace(cmd.Name); n != "" {
				p.Name = n
			}
			if cmd.IDPType != nil && *cmd.IDPType == "OIDC" && p.UserIdentity != nil {
				p.UserIdentity.PasswordHash = nil
				provider := "OIDC"
				p.UserIdentity.Provider = &provider
			}
			p.Roles = make([]serviceaccount.RoleAssignment, 0, len(assign))
			for _, name := range assign {
				p.Roles = append(p.Roles, serviceaccount.RoleAssignment{Role: name, AssignmentSource: &src, AssignedAt: now})
			}
			event := UserCreated{
				Metadata: usecase.NewEventMetadata(ec, UserCreatedType, Source, subjectFor(p.ID)),
				UserID:   p.ID,
				Email:    email,
			}
			return usecaseop.Save(p, principal.RolesPersister{Repository: principals}, event), nil
		},
	}
}
