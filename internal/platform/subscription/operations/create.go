package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/netguard"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/dispatchqueue"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/validate"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

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
	// Queue is the dispatch priority — DEFAULT or HIGH_PRIORITY, matched
	// ignoring case. It selects which of the client's two dispatch queues a
	// job raised from this subscription publishes to. Omitted or blank leaves
	// it unset, which the publish path reads as DEFAULT.
	Queue *string `json:"queue,omitempty"`
	// ClientScoped marks a subscription to client-scoped event types (the
	// SPA's Client Scoped switch). Omitted: false.
	ClientScoped *bool `json:"clientScoped,omitempty"`
	// Sequence orders this subscription's dispatch jobs within a message
	// group (lower first). Omitted: the default, 99.
	Sequence *int32 `json:"sequence,omitempty"`
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
			if err := netguard.Default.ValidateURL(cmd.Endpoint); err != nil {
				return usecase.Validation("INVALID_ENDPOINT", "endpoint "+err.Error())
			}
			if len(cmd.EventTypes) == 0 {
				return usecase.Validation("EVENT_TYPES_REQUIRED", "at least one event type binding is required")
			}
			if cmd.Queue != nil {
				if _, err := dispatchqueue.Parse(*cmd.Queue); err != nil {
					return err
				}
			}
			if cmd.Sequence != nil && *cmd.Sequence < 0 {
				return usecase.Validation("INVALID_SEQUENCE", "sequence must not be negative")
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

			// A UI/API create never sets ApplicationCode (only SyncSubscriptions
			// does), so this always checks the "no application" partition of the
			// new (application_code, client_id, code) key — consistent with what
			// the row will actually be persisted with below.
			existing, err := repo.FindByCode(ctx, code, nil, cmd.ClientID)
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
			s.ClientID = ids.PtrOf[ids.ClientID](cmd.ClientID)
			if s.ClientIdentifier, err = repo.ClientIdentifier(ctx, cmd.ClientID); err != nil {
				return nil, usecase.Internal("REPO", "client identifier lookup failed", err)
			}
			s.ConnectionID = cmd.ConnectionID
			s.DispatchPoolID = cmd.DispatchPoolID
			if s.DispatchPoolCode, err = poolCode(ctx, repo, cmd.DispatchPoolID); err != nil {
				return nil, err
			}
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
			if cmd.ClientScoped != nil {
				s.ClientScoped = *cmd.ClientScoped
			}
			if cmd.Sequence != nil {
				s.Sequence = *cmd.Sequence
			}
			// Stored in its canonical upper-case form, so the publish path and
			// the served router-config document read one spelling however the
			// caller spelled it. Validate has already rejected anything else.
			if cmd.Queue != nil {
				if p, _ := dispatchqueue.Parse(*cmd.Queue); p != "" {
					stored := string(p)
					s.Queue = &stored
				}
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

// poolCode is the code of the dispatch pool a create or update points at (nil
// for no pool, or an id that names no pool): stored beside the id as the SDK
// sync stores it.
func poolCode(ctx context.Context, repo *subscription.Repository, poolID *string) (*string, error) {
	if poolID == nil || *poolID == "" {
		return nil, nil
	}
	code, err := repo.DispatchPoolCode(ctx, *poolID)
	if err != nil {
		return nil, usecase.Internal("REPO", "dispatch pool lookup failed", err)
	}
	return code, nil
}
