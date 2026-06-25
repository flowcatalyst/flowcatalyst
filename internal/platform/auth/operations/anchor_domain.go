package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// ── Create ────────────────────────────────────────────────────────────────

type CreateAnchorDomainCommand struct {
	Domain string `json:"domain"`
}

// CreateAnchorDomain validates the domain, enforces uniqueness, persists it,
// and emits [AnchorDomainCreated]. Anchor domains are platform-level config
// with no per-client dimension (Authorize: Public); the controller gates
// writes with auth.RequireAnchor.
func CreateAnchorDomain(repo *auth.AnchorDomainRepo) usecaseop.Operation[CreateAnchorDomainCommand, AnchorDomainCreated] {
	return usecaseop.Operation[CreateAnchorDomainCommand, AnchorDomainCreated]{
		Name: "CreateAnchorDomain",
		Validate: func(_ context.Context, cmd CreateAnchorDomainCommand) error {
			d := strings.ToLower(strings.TrimSpace(cmd.Domain))
			if d == "" {
				return usecase.Validation("DOMAIN_REQUIRED", "domain is required")
			}
			if !strings.Contains(d, ".") || strings.ContainsAny(d, " /@") {
				return usecase.Validation("INVALID_DOMAIN", "domain must be a valid DNS name (e.g. example.com)")
			}
			return nil
		},
		Authorize: usecaseop.Public[CreateAnchorDomainCommand],
		Execute: func(ctx context.Context, cmd CreateAnchorDomainCommand, ec usecase.ExecutionContext) (usecaseop.Plan[AnchorDomainCreated], error) {
			d := strings.ToLower(strings.TrimSpace(cmd.Domain))
			existing, err := repo.FindByDomain(ctx, d)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_domain failed", err)
			}
			if existing != nil {
				return nil, usecase.Conflict("DOMAIN_EXISTS", "Anchor domain '"+d+"' already exists")
			}
			a := auth.NewAnchorDomain(d)
			event := AnchorDomainCreated{
				Metadata:       usecase.NewEventMetadata(ec, AnchorDomainCreatedType, Source, anchorSubject(a.ID)),
				AnchorDomainID: a.ID,
				Domain:         a.Domain,
			}
			return usecaseop.Save(a, repo, event), nil
		},
	}
}

// ── Update ────────────────────────────────────────────────────────────────

type UpdateAnchorDomainCommand struct {
	ID     string `json:"id"`
	Domain string `json:"domain"`
}

// UpdateAnchorDomain mutates the domain and emits [AnchorDomainUpdated].
// Platform-level config (Authorize: Public); the controller gates on anchor.
func UpdateAnchorDomain(repo *auth.AnchorDomainRepo) usecaseop.Operation[UpdateAnchorDomainCommand, AnchorDomainUpdated] {
	return usecaseop.Operation[UpdateAnchorDomainCommand, AnchorDomainUpdated]{
		Name: "UpdateAnchorDomain",
		Validate: func(_ context.Context, cmd UpdateAnchorDomainCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			d := strings.ToLower(strings.TrimSpace(cmd.Domain))
			if d == "" || !strings.Contains(d, ".") || strings.ContainsAny(d, " /@") {
				return usecase.Validation("INVALID_DOMAIN", "domain must be a valid DNS name")
			}
			return nil
		},
		Authorize: usecaseop.Public[UpdateAnchorDomainCommand],
		Execute: func(ctx context.Context, cmd UpdateAnchorDomainCommand, ec usecase.ExecutionContext) (usecaseop.Plan[AnchorDomainUpdated], error) {
			d := strings.ToLower(strings.TrimSpace(cmd.Domain))
			a, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if a == nil {
				return nil, httperror.NotFound("AnchorDomain", cmd.ID)
			}
			a.Domain = d
			event := AnchorDomainUpdated{
				Metadata:       usecase.NewEventMetadata(ec, AnchorDomainUpdatedType, Source, anchorSubject(a.ID)),
				AnchorDomainID: a.ID,
				Domain:         a.Domain,
			}
			return usecaseop.Save(a, repo, event), nil
		},
	}
}

// ── Delete ────────────────────────────────────────────────────────────────

type DeleteAnchorDomainCommand struct {
	ID string `json:"id"`
}

// DeleteAnchorDomain removes the domain and emits [AnchorDomainDeleted].
// Platform-level config (Authorize: Public); the controller gates on anchor.
func DeleteAnchorDomain(repo *auth.AnchorDomainRepo) usecaseop.Operation[DeleteAnchorDomainCommand, AnchorDomainDeleted] {
	return usecaseop.Operation[DeleteAnchorDomainCommand, AnchorDomainDeleted]{
		Name: "DeleteAnchorDomain",
		Validate: func(_ context.Context, cmd DeleteAnchorDomainCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[DeleteAnchorDomainCommand],
		Execute: func(ctx context.Context, cmd DeleteAnchorDomainCommand, ec usecase.ExecutionContext) (usecaseop.Plan[AnchorDomainDeleted], error) {
			a, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if a == nil {
				return nil, httperror.NotFound("AnchorDomain", cmd.ID)
			}
			event := AnchorDomainDeleted{
				Metadata:       usecase.NewEventMetadata(ec, AnchorDomainDeletedType, Source, anchorSubject(a.ID)),
				AnchorDomainID: a.ID,
				Domain:         a.Domain,
			}
			return usecaseop.Delete(a, repo, event), nil
		},
	}
}
