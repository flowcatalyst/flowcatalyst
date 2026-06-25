package operations

import (
	"context"
	"regexp"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/validate"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

var urlPattern = regexp.MustCompile(`^https?://.+`)

// CreateCommand is the input DTO.
type CreateCommand struct {
	Code             string                          `json:"code"`
	Name             string                          `json:"name"`
	Endpoint         string                          `json:"endpoint"`
	Description      *string                         `json:"description,omitempty"`
	ClientID         *string                         `json:"clientId,omitempty"`
	ConnectionID     *string                         `json:"connectionId,omitempty"`
	DispatchPoolID   *string                         `json:"dispatchPoolId,omitempty"`
	ServiceAccountID *string                         `json:"serviceAccountId,omitempty"`
	EventTypes       []subscription.EventTypeBinding `json:"eventTypes,omitempty"`
	CustomConfig     []subscription.ConfigEntry      `json:"customConfig,omitempty"`
	Mode             string                          `json:"mode,omitempty"`
	TimeoutSeconds   *int32                          `json:"timeoutSeconds,omitempty"`
	MaxRetries       *int32                          `json:"maxRetries,omitempty"`
	DelaySeconds     *int32                          `json:"delaySeconds,omitempty"`
	MaxAgeSeconds    *int32                          `json:"maxAgeSeconds,omitempty"`
	DataOnly         *bool                           `json:"dataOnly,omitempty"`
}

// CreateSubscription validates cmd, enforces code uniqueness within the
// client scope, persists the subscription, and emits [SubscriptionCreated].
func CreateSubscription(repo *subscription.Repository) usecaseop.Operation[CreateCommand, SubscriptionCreated] {
	return usecaseop.Operation[CreateCommand, SubscriptionCreated]{
		Name: "CreateSubscription",
		Validate: func(_ context.Context, cmd CreateCommand) error {
			code := strings.ToLower(strings.TrimSpace(cmd.Code))
			if code == "" {
				return usecase.Validation("CODE_REQUIRED", "code is required")
			}
			if !validate.CodePattern.MatchString(code) {
				return usecase.Validation("INVALID_CODE_FORMAT",
					"code must start with a lowercase letter and contain only lowercase alphanumeric and hyphens")
			}
			if strings.TrimSpace(cmd.Name) == "" {
				return usecase.Validation("NAME_REQUIRED", "name is required")
			}
			if !urlPattern.MatchString(cmd.Endpoint) {
				return usecase.Validation("INVALID_ENDPOINT", "endpoint must be a http(s) URL")
			}
			if len(cmd.EventTypes) == 0 {
				return usecase.Validation("EVENT_TYPES_REQUIRED", "at least one event type binding is required")
			}
			return nil
		},
		// Resource-level authorization (the coarse "may write subscriptions"
		// permission is enforced at the controller). A subscription bound to a
		// client may only be created by a principal with access to that client;
		// a platform-wide subscription (nil ClientID) requires anchor. This is
		// exactly auth.CheckScopeAccess on the target client.
		Authorize: func(ctx context.Context, cmd CreateCommand) error {
			return auth.CheckScopeAccess(auth.FromContext(ctx), cmd.ClientID)
		},
		Execute: func(ctx context.Context, cmd CreateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[SubscriptionCreated], error) {
			code := strings.ToLower(strings.TrimSpace(cmd.Code))

			existing, err := repo.FindByCode(ctx, code, cmd.ClientID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_code failed", err)
			}
			if existing != nil {
				return nil, usecase.Conflict(
					"CODE_EXISTS",
					"Subscription with code '"+code+"' already exists")
			}

			s := subscription.New(code, strings.TrimSpace(cmd.Name), cmd.Endpoint)
			s.Description = cmd.Description
			s.ClientID = cmd.ClientID
			s.ConnectionID = cmd.ConnectionID
			s.DispatchPoolID = cmd.DispatchPoolID
			s.ServiceAccountID = cmd.ServiceAccountID
			s.EventTypes = cmd.EventTypes
			if cmd.CustomConfig != nil {
				s.CustomConfig = cmd.CustomConfig
			}
			if cmd.Mode != "" {
				s.Mode = common.ParseDispatchMode(cmd.Mode)
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
			if cmd.DataOnly != nil {
				s.DataOnly = *cmd.DataOnly
			}
			s.CreatedBy = &ec.PrincipalID

			event := SubscriptionCreated{
				Metadata:       usecase.NewEventMetadata(ec, SubscriptionCreatedType, Source, subjectFor(s.ID)),
				SubscriptionID: s.ID,
				Code:           s.Code,
				Name:           s.Name,
			}
			return usecaseop.Save(s, repo, event), nil
		},
	}
}
