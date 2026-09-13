package operations

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// UpdateCommand is the input DTO.
type UpdateCommand struct {
	ID                 string                             `json:"id"`
	Name               *string                            `json:"name,omitempty"`
	Description        *string                            `json:"description,omitempty"`
	Scope              *string                            `json:"scope,omitempty"`
	ClientIDs          []string                           `json:"clientIds,omitempty"`
	WebhookCredentials *serviceaccount.WebhookCredentials `json:"webhookCredentials,omitempty"`
}

// UpdateServiceAccount mutates mutable fields and emits [ServiceAccountUpdated].
//
// Changing the client links RE-DERIVES the linked SERVICE principal's reach in
// the same transaction, so an account moved between clients cannot keep the
// reach it had before. A token minted before the update keeps its claims until
// it expires — the principal is read at mint time, not per request.
func UpdateServiceAccount(
	repo *serviceaccount.Repository,
	principals *principal.Repository,
	clients *client.Repository,
	grants *principal.ClientAccessGrantRepo,
) usecaseop.TxOperation[UpdateCommand, ServiceAccountUpdated] {
	return usecaseop.TxOperation[UpdateCommand, ServiceAccountUpdated]{
		Name: "UpdateServiceAccount",
		Validate: func(_ context.Context, cmd UpdateCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			if cmd.Name != nil && strings.TrimSpace(*cmd.Name) == "" {
				return usecase.Validation("NAME_REQUIRED", "name cannot be empty")
			}
			return nil
		},
		// The coarse "may write service accounts" permission is enforced at the
		// controller; this admin-managed update has no per-client resource
		// check, so the operation is intentionally open.
		Authorize: usecaseop.Public[UpdateCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd UpdateCommand, ec usecase.ExecutionContext) (ServiceAccountUpdated, error) {
			var zero ServiceAccountUpdated
			sa, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return zero, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if sa == nil {
				return zero, httperror.NotFound("ServiceAccount", cmd.ID)
			}
			if cmd.Name != nil {
				sa.Name = strings.TrimSpace(*cmd.Name)
			}
			if cmd.Description != nil {
				sa.Description = cmd.Description
			}
			if cmd.Scope != nil {
				sa.Scope = cmd.Scope
			}
			reachChanged := cmd.ClientIDs != nil
			if reachChanged {
				if err := requireClientsExist(ctx, clients, cmd.ClientIDs); err != nil {
					return zero, err
				}
				sa.ClientIDs = cmd.ClientIDs
			}
			if cmd.WebhookCredentials != nil {
				sa.WebhookCredentials = *cmd.WebhookCredentials
			}

			event := ServiceAccountUpdated{
				Metadata:         usecase.NewEventMetadata(ec, ServiceAccountUpdatedType, Source, subjectFor(sa.ID)),
				ServiceAccountID: sa.ID,
				Name:             sa.Name,
			}
			if r := usecasepgx.CommitScoped(ctx, s, sa, repo, event, cmd); !usecase.IsSuccess(r) {
				_, e := usecase.Into(r)
				return zero, e
			}

			// Re-derive the linked principal's reach from the new links. An
			// account with no linked principal yet (created without
			// credentials) has nothing to re-derive: its reach is derived when
			// the principal is created.
			if !reachChanged {
				return event, nil
			}
			p, err := principals.FindByServiceAccount(ctx, sa.ID)
			if err != nil {
				return zero, usecase.Internal("REPO", "find_by_service_account failed", err)
			}
			if p == nil {
				return event, nil
			}
			grantClientIDs := applyClientReach(p, sa.ClientIDs)
			if err := s.WithTx(ctx, func(tx pgx.Tx) error {
				return principal.ClientAssociationPersister{
					Repository:     principals,
					Grants:         grants,
					GrantClientIDs: grantClientIDs,
					GrantedBy:      ec.PrincipalID,
				}.Persist(ctx, p, usecasepgx.WrapTxForBootstrap(tx))
			}); err != nil {
				return zero, usecase.Internal("PERSIST", "service principal reach persist failed", err)
			}
			return event, nil
		},
	}
}
