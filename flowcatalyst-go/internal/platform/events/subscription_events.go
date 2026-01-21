package events

import (
	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/subscription"
)

// SubscriptionCreated is emitted when a new subscription is created
type SubscriptionCreated struct {
	common.BaseDomainEvent
	SubscriptionID string `json:"subscriptionId"`
	Code           string `json:"code"`
	Name           string `json:"name"`
	ClientID       string `json:"clientId"`
	Target         string `json:"target"`
}

func (e *SubscriptionCreated) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		SubscriptionID string `json:"subscriptionId"`
		Code           string `json:"code"`
		Name           string `json:"name"`
		ClientID       string `json:"clientId"`
		Target         string `json:"target"`
	}{
		SubscriptionID: e.SubscriptionID,
		Code:           e.Code,
		Name:           e.Name,
		ClientID:       e.ClientID,
		Target:         e.Target,
	})
}

func NewSubscriptionCreated(ctx *common.ExecutionContext, sub *subscription.Subscription) *SubscriptionCreated {
	return &SubscriptionCreated{
		BaseDomainEvent: newBase(ctx, EventTypeSubscriptionCreated, "platform", "subscription", sub.ID),
		SubscriptionID:  sub.ID,
		Code:            sub.Code,
		Name:            sub.Name,
		ClientID:        sub.ClientID,
		Target:          sub.Target,
	}
}

// SubscriptionUpdated is emitted when a subscription is updated
type SubscriptionUpdated struct {
	common.BaseDomainEvent
	SubscriptionID string `json:"subscriptionId"`
	Code           string `json:"code"`
	Name           string `json:"name"`
	Target         string `json:"target"`
}

func (e *SubscriptionUpdated) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		SubscriptionID string `json:"subscriptionId"`
		Code           string `json:"code"`
		Name           string `json:"name"`
		Target         string `json:"target"`
	}{
		SubscriptionID: e.SubscriptionID,
		Code:           e.Code,
		Name:           e.Name,
		Target:         e.Target,
	})
}

func NewSubscriptionUpdated(ctx *common.ExecutionContext, sub *subscription.Subscription) *SubscriptionUpdated {
	return &SubscriptionUpdated{
		BaseDomainEvent: newBase(ctx, EventTypeSubscriptionUpdated, "platform", "subscription", sub.ID),
		SubscriptionID:  sub.ID,
		Code:            sub.Code,
		Name:            sub.Name,
		Target:          sub.Target,
	}
}

// SubscriptionPaused is emitted when a subscription is paused
type SubscriptionPaused struct {
	common.BaseDomainEvent
	SubscriptionID string `json:"subscriptionId"`
	Code           string `json:"code"`
}

func (e *SubscriptionPaused) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		SubscriptionID string `json:"subscriptionId"`
		Code           string `json:"code"`
	}{
		SubscriptionID: e.SubscriptionID,
		Code:           e.Code,
	})
}

func NewSubscriptionPaused(ctx *common.ExecutionContext, sub *subscription.Subscription) *SubscriptionPaused {
	return &SubscriptionPaused{
		BaseDomainEvent: newBase(ctx, EventTypeSubscriptionPaused, "platform", "subscription", sub.ID),
		SubscriptionID:  sub.ID,
		Code:            sub.Code,
	}
}

// SubscriptionResumed is emitted when a subscription is resumed
type SubscriptionResumed struct {
	common.BaseDomainEvent
	SubscriptionID string `json:"subscriptionId"`
	Code           string `json:"code"`
}

func (e *SubscriptionResumed) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		SubscriptionID string `json:"subscriptionId"`
		Code           string `json:"code"`
	}{
		SubscriptionID: e.SubscriptionID,
		Code:           e.Code,
	})
}

func NewSubscriptionResumed(ctx *common.ExecutionContext, sub *subscription.Subscription) *SubscriptionResumed {
	return &SubscriptionResumed{
		BaseDomainEvent: newBase(ctx, EventTypeSubscriptionResumed, "platform", "subscription", sub.ID),
		SubscriptionID:  sub.ID,
		Code:            sub.Code,
	}
}

// SubscriptionDeleted is emitted when a subscription is deleted
type SubscriptionDeleted struct {
	common.BaseDomainEvent
	SubscriptionID string `json:"subscriptionId"`
	Code           string `json:"code"`
}

func (e *SubscriptionDeleted) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		SubscriptionID string `json:"subscriptionId"`
		Code           string `json:"code"`
	}{
		SubscriptionID: e.SubscriptionID,
		Code:           e.Code,
	})
}

func NewSubscriptionDeleted(ctx *common.ExecutionContext, sub *subscription.Subscription) *SubscriptionDeleted {
	return &SubscriptionDeleted{
		BaseDomainEvent: newBase(ctx, EventTypeSubscriptionDeleted, "platform", "subscription", sub.ID),
		SubscriptionID:  sub.ID,
		Code:            sub.Code,
	}
}
