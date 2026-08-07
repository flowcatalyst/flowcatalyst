package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// UpdateCommand mirrors CreateCommand but with the mapping ID + optional
// fields. A nil pointer means "do not change"; an empty slice means "clear".
// The identity provider is deliberately NOT updatable here — re-pointing a
// domain changes its users' auth method and has direction-specific side
// effects, so it goes through [MoveMappingToProvider].
type UpdateCommand struct {
	ID                   string   `json:"id"`
	PrimaryClientID      *string  `json:"primaryClientId,omitempty"`
	AdditionalClientIDs  []string `json:"additionalClientIds,omitempty"`
	GrantedClientIDs     []string `json:"grantedClientIds,omitempty"`
	RequiredOIDCTenantID *string  `json:"requiredOidcTenantId,omitempty"`
	// 2FA fields: nil pointer = unchanged; a non-nil Allowed2FAMethods slice
	// (incl. empty) replaces the set.
	Require2FA            *bool    `json:"require2fa,omitempty"`
	Allowed2FAMethods     []string `json:"allowed2faMethods,omitempty"`
	RememberDeviceEnabled *bool    `json:"rememberDeviceEnabled,omitempty"`
	RememberDeviceDays    *int     `json:"rememberDeviceDays,omitempty"`
}

// UpdateMapping mutates an existing mapping and emits
// EmailDomainMappingUpdated. The coarse anchor check lives on the controller;
// email-domain mappings have no per-client resource dimension, so the use case
// carries no resource-level authz (Authorize = usecaseop.Public).
func UpdateMapping(repo *emaildomainmapping.Repository) usecaseop.Operation[UpdateCommand, EmailDomainMappingUpdated] {
	return usecaseop.Operation[UpdateCommand, EmailDomainMappingUpdated]{
		Name: "UpdateMapping",
		Validate: func(_ context.Context, cmd UpdateCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[UpdateCommand],
		Execute: func(ctx context.Context, cmd UpdateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[EmailDomainMappingUpdated], error) {
			e, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if e == nil {
				return nil, httperror.NotFound("EmailDomainMapping", cmd.ID)
			}

			e.PrimaryClientID = cmd.PrimaryClientID
			e.RequiredOIDCTenantID = cmd.RequiredOIDCTenantID
			if cmd.AdditionalClientIDs != nil {
				e.AdditionalClientIDs = cmd.AdditionalClientIDs
			}
			if cmd.GrantedClientIDs != nil {
				e.GrantedClientIDs = cmd.GrantedClientIDs
			}
			if cmd.Require2FA != nil {
				e.Require2FA = *cmd.Require2FA
			}
			if cmd.Allowed2FAMethods != nil {
				e.Allowed2FAMethods = cmd.Allowed2FAMethods
			}
			if cmd.RememberDeviceEnabled != nil {
				e.RememberDeviceEnabled = *cmd.RememberDeviceEnabled
			}
			if cmd.RememberDeviceDays != nil {
				e.RememberDeviceDays = *cmd.RememberDeviceDays
			}
			// Validate the resulting 2FA state (require2fa ⇒ ≥1 valid method).
			if err := validate2FA(e.Require2FA, e.Allowed2FAMethods); err != nil {
				return nil, err
			}

			event := EmailDomainMappingUpdated{
				Metadata:    usecase.NewEventMetadata(ec, EmailDomainMappingUpdatedType, Source, subjectFor(e.ID)),
				MappingID:   e.ID,
				EmailDomain: e.EmailDomain,
			}
			return usecaseop.Save(e, repo, event), nil
		},
	}
}
