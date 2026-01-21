package events

import (
	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/dispatchpool"
)

// DispatchPoolCreated is emitted when a new dispatch pool is created
type DispatchPoolCreated struct {
	common.BaseDomainEvent
	DispatchPoolID  string `json:"dispatchPoolId"`
	Code            string `json:"code"`
	Name            string `json:"name"`
	ClientID        string `json:"clientId"`
	RateLimitPerMin *int   `json:"rateLimitPerMin,omitempty"`
}

func (e *DispatchPoolCreated) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		DispatchPoolID  string `json:"dispatchPoolId"`
		Code            string `json:"code"`
		Name            string `json:"name"`
		ClientID        string `json:"clientId"`
		RateLimitPerMin *int   `json:"rateLimitPerMin,omitempty"`
	}{
		DispatchPoolID:  e.DispatchPoolID,
		Code:            e.Code,
		Name:            e.Name,
		ClientID:        e.ClientID,
		RateLimitPerMin: e.RateLimitPerMin,
	})
}

func NewDispatchPoolCreated(ctx *common.ExecutionContext, dp *dispatchpool.DispatchPool) *DispatchPoolCreated {
	return &DispatchPoolCreated{
		BaseDomainEvent: newBase(ctx, EventTypeDispatchPoolCreated, "platform", "dispatchpool", dp.ID),
		DispatchPoolID:  dp.ID,
		Code:            dp.Code,
		Name:            dp.Name,
		ClientID:        dp.ClientID,
		RateLimitPerMin: dp.RateLimitPerMin,
	}
}

// DispatchPoolUpdated is emitted when a dispatch pool is updated
type DispatchPoolUpdated struct {
	common.BaseDomainEvent
	DispatchPoolID  string `json:"dispatchPoolId"`
	Code            string `json:"code"`
	Name            string `json:"name"`
	RateLimitPerMin *int   `json:"rateLimitPerMin,omitempty"`
}

func (e *DispatchPoolUpdated) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		DispatchPoolID  string `json:"dispatchPoolId"`
		Code            string `json:"code"`
		Name            string `json:"name"`
		RateLimitPerMin *int   `json:"rateLimitPerMin,omitempty"`
	}{
		DispatchPoolID:  e.DispatchPoolID,
		Code:            e.Code,
		Name:            e.Name,
		RateLimitPerMin: e.RateLimitPerMin,
	})
}

func NewDispatchPoolUpdated(ctx *common.ExecutionContext, dp *dispatchpool.DispatchPool) *DispatchPoolUpdated {
	return &DispatchPoolUpdated{
		BaseDomainEvent: newBase(ctx, EventTypeDispatchPoolUpdated, "platform", "dispatchpool", dp.ID),
		DispatchPoolID:  dp.ID,
		Code:            dp.Code,
		Name:            dp.Name,
		RateLimitPerMin: dp.RateLimitPerMin,
	}
}

// DispatchPoolSuspended is emitted when a dispatch pool is suspended
type DispatchPoolSuspended struct {
	common.BaseDomainEvent
	DispatchPoolID string `json:"dispatchPoolId"`
	Code           string `json:"code"`
}

func (e *DispatchPoolSuspended) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		DispatchPoolID string `json:"dispatchPoolId"`
		Code           string `json:"code"`
	}{
		DispatchPoolID: e.DispatchPoolID,
		Code:           e.Code,
	})
}

func NewDispatchPoolSuspended(ctx *common.ExecutionContext, dp *dispatchpool.DispatchPool) *DispatchPoolSuspended {
	return &DispatchPoolSuspended{
		BaseDomainEvent: newBase(ctx, EventTypeDispatchPoolSuspended, "platform", "dispatchpool", dp.ID),
		DispatchPoolID:  dp.ID,
		Code:            dp.Code,
	}
}

// DispatchPoolArchived is emitted when a dispatch pool is archived
type DispatchPoolArchived struct {
	common.BaseDomainEvent
	DispatchPoolID string `json:"dispatchPoolId"`
	Code           string `json:"code"`
}

func (e *DispatchPoolArchived) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		DispatchPoolID string `json:"dispatchPoolId"`
		Code           string `json:"code"`
	}{
		DispatchPoolID: e.DispatchPoolID,
		Code:           e.Code,
	})
}

func NewDispatchPoolArchived(ctx *common.ExecutionContext, dp *dispatchpool.DispatchPool) *DispatchPoolArchived {
	return &DispatchPoolArchived{
		BaseDomainEvent: newBase(ctx, EventTypeDispatchPoolArchived, "platform", "dispatchpool", dp.ID),
		DispatchPoolID:  dp.ID,
		Code:            dp.Code,
	}
}
