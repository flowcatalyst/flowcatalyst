package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// ── Create ────────────────────────────────────────────────────────────────

type CreateAnchorDomainCommand struct {
	Domain string `json:"domain"`
}

func CreateAnchorDomain(
	ctx context.Context,
	repo *auth.AnchorDomainRepo,
	uow *usecasepgx.UnitOfWork,
	cmd CreateAnchorDomainCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[AnchorDomainCreated], error) {
	var zero commit.Committed[AnchorDomainCreated]
	d := strings.ToLower(strings.TrimSpace(cmd.Domain))
	if d == "" {
		return zero, usecase.Validation("DOMAIN_REQUIRED", "domain is required")
	}
	if !strings.Contains(d, ".") || strings.ContainsAny(d, " /@") {
		return zero, usecase.Validation("INVALID_DOMAIN", "domain must be a valid DNS name (e.g. example.com)")
	}
	existing, err := repo.FindByDomain(ctx, d)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_domain failed", err)
	}
	if existing != nil {
		return zero, usecase.Conflict("DOMAIN_EXISTS", "Anchor domain '"+d+"' already exists")
	}
	a := auth.NewAnchorDomain(d)
	event := AnchorDomainCreated{
		Metadata:       usecase.NewEventMetadata(ec, AnchorDomainCreatedType, Source, anchorSubject(a.ID)),
		AnchorDomainID: a.ID,
		Domain:         a.Domain,
	}
	return commit.Save(ctx, uow, a, repo, event, cmd)
}

// ── Update ────────────────────────────────────────────────────────────────

type UpdateAnchorDomainCommand struct {
	ID     string `json:"id"`
	Domain string `json:"domain"`
}

func UpdateAnchorDomain(
	ctx context.Context,
	repo *auth.AnchorDomainRepo,
	uow *usecasepgx.UnitOfWork,
	cmd UpdateAnchorDomainCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[AnchorDomainUpdated], error) {
	var zero commit.Committed[AnchorDomainUpdated]
	if strings.TrimSpace(cmd.ID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "id is required")
	}
	d := strings.ToLower(strings.TrimSpace(cmd.Domain))
	if d == "" || !strings.Contains(d, ".") || strings.ContainsAny(d, " /@") {
		return zero, usecase.Validation("INVALID_DOMAIN", "domain must be a valid DNS name")
	}
	a, err := repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if a == nil {
		return zero, httperror.NotFound("AnchorDomain", cmd.ID)
	}
	a.Domain = d
	event := AnchorDomainUpdated{
		Metadata:       usecase.NewEventMetadata(ec, AnchorDomainUpdatedType, Source, anchorSubject(a.ID)),
		AnchorDomainID: a.ID,
		Domain:         a.Domain,
	}
	return commit.Save(ctx, uow, a, repo, event, cmd)
}

// ── Delete ────────────────────────────────────────────────────────────────

type DeleteAnchorDomainCommand struct {
	ID string `json:"id"`
}

func DeleteAnchorDomain(
	ctx context.Context,
	repo *auth.AnchorDomainRepo,
	uow *usecasepgx.UnitOfWork,
	cmd DeleteAnchorDomainCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[AnchorDomainDeleted], error) {
	var zero commit.Committed[AnchorDomainDeleted]
	if strings.TrimSpace(cmd.ID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "id is required")
	}
	a, err := repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if a == nil {
		return zero, httperror.NotFound("AnchorDomain", cmd.ID)
	}
	event := AnchorDomainDeleted{
		Metadata:       usecase.NewEventMetadata(ec, AnchorDomainDeletedType, Source, anchorSubject(a.ID)),
		AnchorDomainID: a.ID,
		Domain:         a.Domain,
	}
	return commit.Delete(ctx, uow, a, repo, event, cmd)
}
