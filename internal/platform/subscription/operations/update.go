package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// UpdateCommand applies optional updates. Nil pointers mean "don't change".
type UpdateCommand struct {
	ID               string                          `json:"id"`
	Name             *string                         `json:"name,omitempty"`
	Description      *string                         `json:"description,omitempty"`
	Endpoint         *string                         `json:"endpoint,omitempty"`
	ConnectionID     *string                         `json:"connectionId,omitempty"`
	EventTypes       []subscription.EventTypeBinding `json:"eventTypes,omitempty"`
	CustomConfig     []subscription.ConfigEntry      `json:"customConfig,omitempty"`
	Mode             *string                         `json:"mode,omitempty"`
	TimeoutSeconds   *int32                          `json:"timeoutSeconds,omitempty"`
	MaxRetries       *int32                          `json:"maxRetries,omitempty"`
	DelaySeconds     *int32                          `json:"delaySeconds,omitempty"`
	MaxAgeSeconds    *int32                          `json:"maxAgeSeconds,omitempty"`
	DispatchPoolID   *string                         `json:"dispatchPoolId,omitempty"`
	ServiceAccountID *string                         `json:"serviceAccountId,omitempty"`
	DataOnly         *bool                           `json:"dataOnly,omitempty"`
}

// UpdateSubscription mutates mutable fields and emits [SubscriptionUpdated].
func UpdateSubscription(
	ctx context.Context,
	repo *subscription.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd UpdateCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[SubscriptionUpdated], error) {
	var zero commit.Committed[SubscriptionUpdated]

	if strings.TrimSpace(cmd.ID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "id is required")
	}
	if cmd.Name != nil && strings.TrimSpace(*cmd.Name) == "" {
		return zero, usecase.Validation("NAME_REQUIRED", "name cannot be empty")
	}
	if cmd.Endpoint != nil && !urlPattern.MatchString(*cmd.Endpoint) {
		return zero, usecase.Validation("INVALID_ENDPOINT", "endpoint must be a http(s) URL")
	}

	s, err := repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if s == nil {
		return zero, httperror.NotFound("Subscription", cmd.ID)
	}

	if cmd.Name != nil {
		s.Name = strings.TrimSpace(*cmd.Name)
	}
	if cmd.Description != nil {
		s.Description = cmd.Description
	}
	if cmd.Endpoint != nil {
		s.Endpoint = *cmd.Endpoint
	}
	// Set-if-provided, matching Rust's update use case: a nil/omitted value
	// leaves the existing binding, so connectionId can be re-pointed but not
	// cleared via this endpoint.
	if cmd.ConnectionID != nil {
		s.ConnectionID = cmd.ConnectionID
	}
	if cmd.EventTypes != nil {
		s.EventTypes = cmd.EventTypes
	}
	if cmd.CustomConfig != nil {
		s.CustomConfig = cmd.CustomConfig
	}
	if cmd.Mode != nil {
		s.Mode = common.ParseDispatchMode(*cmd.Mode)
	}
	if cmd.TimeoutSeconds != nil {
		s.TimeoutSeconds = *cmd.TimeoutSeconds
	}
	if cmd.MaxRetries != nil {
		s.MaxRetries = *cmd.MaxRetries
	}
	if cmd.DelaySeconds != nil {
		s.DelaySeconds = *cmd.DelaySeconds
	}
	if cmd.MaxAgeSeconds != nil {
		s.MaxAgeSeconds = *cmd.MaxAgeSeconds
	}
	if cmd.DispatchPoolID != nil {
		s.DispatchPoolID = cmd.DispatchPoolID
	}
	if cmd.ServiceAccountID != nil {
		s.ServiceAccountID = cmd.ServiceAccountID
	}
	if cmd.DataOnly != nil {
		s.DataOnly = *cmd.DataOnly
	}

	event := SubscriptionUpdated{
		Metadata:       usecase.NewEventMetadata(ec, SubscriptionUpdatedType, Source, subjectFor(s.ID)),
		SubscriptionID: s.ID,
		Name:           s.Name,
	}
	return commit.Save(ctx, uow, s, repo, event, cmd)
}
