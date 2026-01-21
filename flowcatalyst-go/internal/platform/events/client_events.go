package events

import (
	"go.flowcatalyst.tech/internal/platform/client"
	"go.flowcatalyst.tech/internal/platform/common"
)

// ClientCreated is emitted when a new client is created
type ClientCreated struct {
	common.BaseDomainEvent
	ClientID   string `json:"clientId"`
	Name       string `json:"name"`
	Identifier string `json:"identifier"`
}

func (e *ClientCreated) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		ClientID   string `json:"clientId"`
		Name       string `json:"name"`
		Identifier string `json:"identifier"`
	}{
		ClientID:   e.ClientID,
		Name:       e.Name,
		Identifier: e.Identifier,
	})
}

func NewClientCreated(ctx *common.ExecutionContext, c *client.Client) *ClientCreated {
	return &ClientCreated{
		BaseDomainEvent: newBase(ctx, EventTypeClientCreated, "platform", "client", c.ID),
		ClientID:        c.ID,
		Name:            c.Name,
		Identifier:      c.Identifier,
	}
}

// ClientUpdated is emitted when a client is updated
type ClientUpdated struct {
	common.BaseDomainEvent
	ClientID   string `json:"clientId"`
	Name       string `json:"name"`
	Identifier string `json:"identifier"`
}

func (e *ClientUpdated) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		ClientID   string `json:"clientId"`
		Name       string `json:"name"`
		Identifier string `json:"identifier"`
	}{
		ClientID:   e.ClientID,
		Name:       e.Name,
		Identifier: e.Identifier,
	})
}

func NewClientUpdated(ctx *common.ExecutionContext, c *client.Client) *ClientUpdated {
	return &ClientUpdated{
		BaseDomainEvent: newBase(ctx, EventTypeClientUpdated, "platform", "client", c.ID),
		ClientID:        c.ID,
		Name:            c.Name,
		Identifier:      c.Identifier,
	}
}

// ClientSuspended is emitted when a client is suspended
type ClientSuspended struct {
	common.BaseDomainEvent
	ClientID string `json:"clientId"`
	Name     string `json:"name"`
	Reason   string `json:"reason,omitempty"`
}

func (e *ClientSuspended) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		ClientID string `json:"clientId"`
		Name     string `json:"name"`
		Reason   string `json:"reason,omitempty"`
	}{
		ClientID: e.ClientID,
		Name:     e.Name,
		Reason:   e.Reason,
	})
}

func NewClientSuspended(ctx *common.ExecutionContext, c *client.Client, reason string) *ClientSuspended {
	return &ClientSuspended{
		BaseDomainEvent: newBase(ctx, EventTypeClientSuspended, "platform", "client", c.ID),
		ClientID:        c.ID,
		Name:            c.Name,
		Reason:          reason,
	}
}

// ClientActivated is emitted when a client is activated (unsuspended)
type ClientActivated struct {
	common.BaseDomainEvent
	ClientID string `json:"clientId"`
	Name     string `json:"name"`
}

func (e *ClientActivated) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		ClientID string `json:"clientId"`
		Name     string `json:"name"`
	}{
		ClientID: e.ClientID,
		Name:     e.Name,
	})
}

func NewClientActivated(ctx *common.ExecutionContext, c *client.Client) *ClientActivated {
	return &ClientActivated{
		BaseDomainEvent: newBase(ctx, EventTypeClientActivated, "platform", "client", c.ID),
		ClientID:        c.ID,
		Name:            c.Name,
	}
}
