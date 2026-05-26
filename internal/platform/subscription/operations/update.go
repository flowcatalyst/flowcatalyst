package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// UpdateCommand applies optional updates. Nil pointers mean "don't change".
type UpdateCommand struct {
	ID               string                          `json:"id"`
	Name             *string                         `json:"name,omitempty"`
	Description      *string                         `json:"description,omitempty"`
	Endpoint         *string                         `json:"endpoint,omitempty"`
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

// UpdateUseCase implements UseCase.
type UpdateUseCase struct {
	repo *subscription.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewUpdateUseCase wires the use case.
func NewUpdateUseCase(repo *subscription.Repository, uow *usecasepgx.UnitOfWork) *UpdateUseCase {
	return &UpdateUseCase{repo: repo, uow: uow}
}

func (uc *UpdateUseCase) Validate(_ context.Context, cmd UpdateCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	if cmd.Name != nil && strings.TrimSpace(*cmd.Name) == "" {
		return usecase.Validation("NAME_REQUIRED", "name cannot be empty")
	}
	if cmd.Endpoint != nil && !urlPattern.MatchString(*cmd.Endpoint) {
		return usecase.Validation("INVALID_ENDPOINT", "endpoint must be a http(s) URL")
	}
	return nil
}

func (uc *UpdateUseCase) Authorize(_ context.Context, _ UpdateCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *UpdateUseCase) Execute(ctx context.Context, cmd UpdateCommand, ec usecase.ExecutionContext) usecase.Result[SubscriptionUpdated] {
	s, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[SubscriptionUpdated](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if s == nil {
		return usecase.Failure[SubscriptionUpdated](httperror.NotFound("Subscription", cmd.ID))
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
	return usecasepgx.Commit[subscription.Subscription, SubscriptionUpdated, UpdateCommand](
		ctx, uc.uow, s, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[UpdateCommand, SubscriptionUpdated] = (*UpdateUseCase)(nil)
