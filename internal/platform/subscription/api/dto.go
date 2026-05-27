// dto.go contains the wire-format types for the subscription API.
package api

import (
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription/operations"
)

// EventTypeBindingDTO mirrors subscription.EventTypeBinding for the wire.
type EventTypeBindingDTO struct {
	EventTypeID   *string `json:"eventTypeId,omitempty"`
	EventTypeCode string  `json:"eventTypeCode"`
	SpecVersion   *string `json:"specVersion,omitempty"`
	Filter        *string `json:"filter,omitempty"`
}

func (b EventTypeBindingDTO) toEntity() subscription.EventTypeBinding {
	return subscription.EventTypeBinding{
		EventTypeID:   b.EventTypeID,
		EventTypeCode: b.EventTypeCode,
		SpecVersion:   b.SpecVersion,
		Filter:        b.Filter,
	}
}

func eventTypeBindingFromEntity(b subscription.EventTypeBinding) EventTypeBindingDTO {
	return EventTypeBindingDTO{
		EventTypeID:   b.EventTypeID,
		EventTypeCode: b.EventTypeCode,
		SpecVersion:   b.SpecVersion,
		Filter:        b.Filter,
	}
}

// ConfigEntryDTO mirrors subscription.ConfigEntry.
type ConfigEntryDTO struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (c ConfigEntryDTO) toEntity() subscription.ConfigEntry {
	return subscription.ConfigEntry{Key: c.Key, Value: c.Value}
}

func configEntryFromEntity(c subscription.ConfigEntry) ConfigEntryDTO {
	return ConfigEntryDTO{Key: c.Key, Value: c.Value}
}

// CreateSubscriptionRequest is the wire body for POST /api/subscriptions.
type CreateSubscriptionRequest struct {
	Code             string                `json:"code"`
	Name             string                `json:"name"`
	Endpoint         string                `json:"endpoint" doc:"http(s) URL delivery target"`
	Description      *string               `json:"description,omitempty"`
	ClientID         *string               `json:"clientId,omitempty"`
	ConnectionID     *string               `json:"connectionId,omitempty"`
	DispatchPoolID   *string               `json:"dispatchPoolId,omitempty"`
	ServiceAccountID *string               `json:"serviceAccountId,omitempty"`
	EventTypes       []EventTypeBindingDTO `json:"eventTypes,omitempty"`
	CustomConfig     []ConfigEntryDTO      `json:"customConfig,omitempty"`
	Mode             string                `json:"mode,omitempty" doc:"Dispatch mode (IMMEDIATE, NEXT_ON_ERROR, BLOCK_ON_ERROR)"`
	TimeoutSeconds   *int32                `json:"timeoutSeconds,omitempty"`
	MaxRetries       *int32                `json:"maxRetries,omitempty"`
	DelaySeconds     *int32                `json:"delaySeconds,omitempty"`
	MaxAgeSeconds    *int32                `json:"maxAgeSeconds,omitempty"`
	DataOnly         *bool                 `json:"dataOnly,omitempty"`
}

func (r CreateSubscriptionRequest) toCommand() operations.CreateCommand {
	events := make([]subscription.EventTypeBinding, 0, len(r.EventTypes))
	for _, b := range r.EventTypes {
		events = append(events, b.toEntity())
	}
	var config []subscription.ConfigEntry
	if r.CustomConfig != nil {
		config = make([]subscription.ConfigEntry, 0, len(r.CustomConfig))
		for _, c := range r.CustomConfig {
			config = append(config, c.toEntity())
		}
	}
	return operations.CreateCommand{
		Code:             r.Code,
		Name:             r.Name,
		Endpoint:         r.Endpoint,
		Description:      r.Description,
		ClientID:         r.ClientID,
		ConnectionID:     r.ConnectionID,
		DispatchPoolID:   r.DispatchPoolID,
		ServiceAccountID: r.ServiceAccountID,
		EventTypes:       events,
		CustomConfig:     config,
		Mode:             r.Mode,
		TimeoutSeconds:   r.TimeoutSeconds,
		MaxRetries:       r.MaxRetries,
		DelaySeconds:     r.DelaySeconds,
		MaxAgeSeconds:    r.MaxAgeSeconds,
		DataOnly:         r.DataOnly,
	}
}

// UpdateSubscriptionRequest is the wire body for PUT /api/subscriptions/{id}.
type UpdateSubscriptionRequest struct {
	Name             *string               `json:"name,omitempty"`
	Description      *string               `json:"description,omitempty"`
	Endpoint         *string               `json:"endpoint,omitempty"`
	EventTypes       []EventTypeBindingDTO `json:"eventTypes,omitempty"`
	CustomConfig     []ConfigEntryDTO      `json:"customConfig,omitempty"`
	Mode             *string               `json:"mode,omitempty"`
	TimeoutSeconds   *int32                `json:"timeoutSeconds,omitempty"`
	MaxRetries       *int32                `json:"maxRetries,omitempty"`
	DelaySeconds     *int32                `json:"delaySeconds,omitempty"`
	MaxAgeSeconds    *int32                `json:"maxAgeSeconds,omitempty"`
	DispatchPoolID   *string               `json:"dispatchPoolId,omitempty"`
	ServiceAccountID *string               `json:"serviceAccountId,omitempty"`
	DataOnly         *bool                 `json:"dataOnly,omitempty"`
}

func (r UpdateSubscriptionRequest) toCommand(id string) operations.UpdateCommand {
	var events []subscription.EventTypeBinding
	if r.EventTypes != nil {
		events = make([]subscription.EventTypeBinding, 0, len(r.EventTypes))
		for _, b := range r.EventTypes {
			events = append(events, b.toEntity())
		}
	}
	var config []subscription.ConfigEntry
	if r.CustomConfig != nil {
		config = make([]subscription.ConfigEntry, 0, len(r.CustomConfig))
		for _, c := range r.CustomConfig {
			config = append(config, c.toEntity())
		}
	}
	return operations.UpdateCommand{
		ID:               id,
		Name:             r.Name,
		Description:      r.Description,
		Endpoint:         r.Endpoint,
		EventTypes:       events,
		CustomConfig:     config,
		Mode:             r.Mode,
		TimeoutSeconds:   r.TimeoutSeconds,
		MaxRetries:       r.MaxRetries,
		DelaySeconds:     r.DelaySeconds,
		MaxAgeSeconds:    r.MaxAgeSeconds,
		DispatchPoolID:   r.DispatchPoolID,
		ServiceAccountID: r.ServiceAccountID,
		DataOnly:         r.DataOnly,
	}
}

// SubscriptionResponse mirrors subscription.Subscription.
type SubscriptionResponse struct {
	ID               string                `json:"id"`
	Code             string                `json:"code"`
	ApplicationCode  *string               `json:"applicationCode,omitempty"`
	Name             string                `json:"name"`
	Description      *string               `json:"description,omitempty"`
	ClientID         *string               `json:"clientId,omitempty"`
	ClientIdentifier *string               `json:"clientIdentifier,omitempty"`
	ClientScoped     bool                  `json:"clientScoped"`
	EventTypes       []EventTypeBindingDTO `json:"eventTypes"`
	ConnectionID     *string               `json:"connectionId,omitempty"`
	Endpoint         string                `json:"endpoint"`
	Queue            *string               `json:"queue,omitempty"`
	CustomConfig     []ConfigEntryDTO      `json:"customConfig"`
	Source           string                `json:"source"`
	Status           string                `json:"status"`
	MaxAgeSeconds    int32                 `json:"maxAgeSeconds"`
	DispatchPoolID   *string               `json:"dispatchPoolId,omitempty"`
	DispatchPoolCode *string               `json:"dispatchPoolCode,omitempty"`
	DelaySeconds     int32                 `json:"delaySeconds"`
	Sequence         int32                 `json:"sequence"`
	Mode             string                `json:"mode"`
	TimeoutSeconds   int32                 `json:"timeoutSeconds"`
	MaxRetries       int32                 `json:"maxRetries"`
	ServiceAccountID *string               `json:"serviceAccountId,omitempty"`
	DataOnly         bool                  `json:"dataOnly"`
	CreatedBy        *string               `json:"createdBy,omitempty"`
	CreatedAt        httpcompat.Time       `json:"createdAt"`
	UpdatedAt        httpcompat.Time       `json:"updatedAt"`
}

func fromEntity(s *subscription.Subscription) SubscriptionResponse {
	events := make([]EventTypeBindingDTO, 0, len(s.EventTypes))
	for _, b := range s.EventTypes {
		events = append(events, eventTypeBindingFromEntity(b))
	}
	config := make([]ConfigEntryDTO, 0, len(s.CustomConfig))
	for _, c := range s.CustomConfig {
		config = append(config, configEntryFromEntity(c))
	}
	return SubscriptionResponse{
		ID:               s.ID,
		Code:             s.Code,
		ApplicationCode:  s.ApplicationCode,
		Name:             s.Name,
		Description:      s.Description,
		ClientID:         s.ClientID,
		ClientIdentifier: s.ClientIdentifier,
		ClientScoped:     s.ClientScoped,
		EventTypes:       events,
		ConnectionID:     s.ConnectionID,
		Endpoint:         s.Endpoint,
		Queue:            s.Queue,
		CustomConfig:     config,
		Source:           string(s.Source),
		Status:           string(s.Status),
		MaxAgeSeconds:    s.MaxAgeSeconds,
		DispatchPoolID:   s.DispatchPoolID,
		DispatchPoolCode: s.DispatchPoolCode,
		DelaySeconds:     s.DelaySeconds,
		Sequence:         s.Sequence,
		Mode:             string(s.Mode),
		TimeoutSeconds:   s.TimeoutSeconds,
		MaxRetries:       s.MaxRetries,
		ServiceAccountID: s.ServiceAccountID,
		DataOnly:         s.DataOnly,
		CreatedBy:        s.CreatedBy,
		CreatedAt:        jsontime.New(s.CreatedAt),
		UpdatedAt:        jsontime.New(s.UpdatedAt),
	}
}

// SubscriptionListResponse is the wire shape for GET /api/subscriptions.
type SubscriptionListResponse struct {
	Items []SubscriptionResponse `json:"items"`
}
