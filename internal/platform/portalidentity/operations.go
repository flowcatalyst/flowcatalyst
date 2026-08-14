package portalidentity

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// Operations are Authorize-Public: the portal-users admin API gates on the
// anchor rule at the controller (same posture as the old ensure endpoint),
// and the JIT path runs as the system actor during an authenticated SSO
// callback. There is no per-resource scope narrower than the client id the
// controller already checked.

type EnsureCommand struct {
	ClientID string  `json:"clientId"`
	Email    string  `json:"email"`
	Name     *string `json:"name,omitempty"`
	Source   string  `json:"source"`
}

// Ensure idempotently creates — or reactivates — the (client, email) portal
// identity and emits [IdentityEnsured]. Re-ensuring keeps the original
// id/source/created_at and any set password; a DISABLED identity converges
// back to ACTIVE (suspend-then-reinvite must work).
func Ensure(repo *Repository, clients *client.Repository) usecaseop.Operation[EnsureCommand, IdentityEnsured] {
	return usecaseop.Operation[EnsureCommand, IdentityEnsured]{
		Name: "EnsurePortalIdentity",
		Validate: func(_ context.Context, cmd EnsureCommand) error {
			if strings.TrimSpace(cmd.ClientID) == "" {
				return usecase.Validation("CLIENT_ID_REQUIRED", "clientId is required")
			}
			email := strings.TrimSpace(cmd.Email)
			if email == "" {
				return usecase.Validation("EMAIL_REQUIRED", "email is required")
			}
			at := strings.IndexByte(email, '@')
			if at <= 0 || at == len(email)-1 {
				return usecase.Validation("EMAIL_INVALID", "email is not valid")
			}
			return nil
		},
		Authorize: usecaseop.Public[EnsureCommand],
		Execute: func(ctx context.Context, cmd EnsureCommand, ec usecase.ExecutionContext) (usecaseop.Plan[IdentityEnsured], error) {
			c, err := clients.FindByID(ctx, cmd.ClientID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_client failed", err)
			}
			if c == nil {
				return nil, httperror.NotFound("Client", cmd.ClientID)
			}

			source := Source(cmd.Source)
			if source != SourceJIT {
				source = SourceInvite
			}
			existing, err := repo.FindByClientAndEmail(ctx, cmd.ClientID, cmd.Email)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_identity failed", err)
			}
			created := existing == nil
			ident := existing
			if ident == nil {
				name := ""
				if cmd.Name != nil {
					name = strings.TrimSpace(*cmd.Name)
				}
				ident = New(cmd.ClientID, cmd.Email, name, source)
			} else {
				ident.Status = StatusActive
				if cmd.Name != nil && strings.TrimSpace(*cmd.Name) != "" {
					ident.Name = strings.TrimSpace(*cmd.Name)
				}
			}

			event := IdentityEnsured{
				Metadata:   usecase.NewEventMetadata(ec, IdentityEnsuredType, EventSource, subjectFor(ident.ID)),
				IdentityID: ident.ID,
				ClientID:   ident.ClientID,
				Email:      ident.Email,
				Created:    created,
				Source_:    string(ident.Source),
			}
			return usecaseop.Save(ident, repo, event), nil
		},
	}
}

type SetStatusCommand struct {
	ClientID string `json:"clientId"`
	Email    string `json:"email,omitempty"`
	// ID targets the identity directly (preferred); Email+ClientID is the
	// fallback shape.
	ID     string `json:"id,omitempty"`
	Status string `json:"status"`
}

// SetStatus suspends (DISABLED) or reactivates (ACTIVE) an identity and
// emits [IdentityStatusSet].
func SetStatus(repo *Repository) usecaseop.Operation[SetStatusCommand, IdentityStatusSet] {
	return usecaseop.Operation[SetStatusCommand, IdentityStatusSet]{
		Name: "SetPortalIdentityStatus",
		Validate: func(_ context.Context, cmd SetStatusCommand) error {
			if strings.TrimSpace(cmd.ID) == "" && (strings.TrimSpace(cmd.ClientID) == "" || strings.TrimSpace(cmd.Email) == "") {
				return usecase.Validation("TARGET_REQUIRED", "id, or clientId + email, is required")
			}
			s := Status(cmd.Status)
			if s != StatusActive && s != StatusDisabled {
				return usecase.Validation("STATUS_INVALID", "status must be ACTIVE or DISABLED")
			}
			return nil
		},
		Authorize: usecaseop.Public[SetStatusCommand],
		Execute: func(ctx context.Context, cmd SetStatusCommand, ec usecase.ExecutionContext) (usecaseop.Plan[IdentityStatusSet], error) {
			ident, err := findTarget(ctx, repo, cmd.ID, cmd.ClientID, cmd.Email)
			if err != nil {
				return nil, err
			}
			// The admin surface is per-client: an id fetched for another
			// client's portal must not be mutable through this client's gate.
			if cmd.ClientID != "" && ident.ClientID != cmd.ClientID {
				return nil, httperror.NotFound("PortalIdentity", cmd.ID)
			}
			ident.Status = Status(cmd.Status)

			event := IdentityStatusSet{
				Metadata:   usecase.NewEventMetadata(ec, IdentityStatusSetType, EventSource, subjectFor(ident.ID)),
				IdentityID: ident.ID,
				ClientID:   ident.ClientID,
				Status:     cmd.Status,
			}
			return usecaseop.Save(ident, repo, event), nil
		},
	}
}

type DeleteCommand struct {
	ClientID string `json:"clientId"`
	ID       string `json:"id"`
}

// Delete removes the identity — offboarding is just deleting the row.
func Delete(repo *Repository) usecaseop.Operation[DeleteCommand, IdentityDeleted] {
	return usecaseop.Operation[DeleteCommand, IdentityDeleted]{
		Name: "DeletePortalIdentity",
		Validate: func(_ context.Context, cmd DeleteCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[DeleteCommand],
		Execute: func(ctx context.Context, cmd DeleteCommand, ec usecase.ExecutionContext) (usecaseop.Plan[IdentityDeleted], error) {
			ident, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_identity failed", err)
			}
			if ident == nil {
				return nil, httperror.NotFound("PortalIdentity", cmd.ID)
			}
			if cmd.ClientID != "" && ident.ClientID != cmd.ClientID {
				return nil, httperror.NotFound("PortalIdentity", cmd.ID)
			}

			event := IdentityDeleted{
				Metadata:   usecase.NewEventMetadata(ec, IdentityDeletedType, EventSource, subjectFor(ident.ID)),
				IdentityID: ident.ID,
				ClientID:   ident.ClientID,
				Email:      ident.Email,
			}
			return usecaseop.Delete(ident, repo, event), nil
		},
	}
}

func findTarget(ctx context.Context, repo *Repository, id, clientID, email string) (*Identity, error) {
	var ident *Identity
	var err error
	if strings.TrimSpace(id) != "" {
		ident, err = repo.FindByID(ctx, id)
	} else {
		ident, err = repo.FindByClientAndEmail(ctx, clientID, email)
	}
	if err != nil {
		return nil, usecase.Internal("REPO", "find_identity failed", err)
	}
	if ident == nil {
		return nil, httperror.NotFound("PortalIdentity", id)
	}
	return ident, nil
}
