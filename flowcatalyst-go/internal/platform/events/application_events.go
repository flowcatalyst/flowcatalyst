package events

import (
	"go.flowcatalyst.tech/internal/platform/application"
	"go.flowcatalyst.tech/internal/platform/common"
)

// ApplicationCreated is emitted when a new application is created
type ApplicationCreated struct {
	common.BaseDomainEvent
	ApplicationID string `json:"applicationId"`
	Code          string `json:"code"`
	Name          string `json:"name"`
	Type          string `json:"type"`
}

func (e *ApplicationCreated) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		ApplicationID string `json:"applicationId"`
		Code          string `json:"code"`
		Name          string `json:"name"`
		Type          string `json:"type"`
	}{
		ApplicationID: e.ApplicationID,
		Code:          e.Code,
		Name:          e.Name,
		Type:          e.Type,
	})
}

func NewApplicationCreated(ctx *common.ExecutionContext, app *application.Application) *ApplicationCreated {
	return &ApplicationCreated{
		BaseDomainEvent: newBase(ctx, EventTypeApplicationCreated, "platform", "application", app.ID),
		ApplicationID:   app.ID,
		Code:            app.Code,
		Name:            app.Name,
		Type:            string(app.Type),
	}
}

// ApplicationUpdated is emitted when an application is updated
type ApplicationUpdated struct {
	common.BaseDomainEvent
	ApplicationID string `json:"applicationId"`
	Code          string `json:"code"`
	Name          string `json:"name"`
}

func (e *ApplicationUpdated) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		ApplicationID string `json:"applicationId"`
		Code          string `json:"code"`
		Name          string `json:"name"`
	}{
		ApplicationID: e.ApplicationID,
		Code:          e.Code,
		Name:          e.Name,
	})
}

func NewApplicationUpdated(ctx *common.ExecutionContext, app *application.Application) *ApplicationUpdated {
	return &ApplicationUpdated{
		BaseDomainEvent: newBase(ctx, EventTypeApplicationUpdated, "platform", "application", app.ID),
		ApplicationID:   app.ID,
		Code:            app.Code,
		Name:            app.Name,
	}
}

// ApplicationDeactivated is emitted when an application is deactivated
type ApplicationDeactivated struct {
	common.BaseDomainEvent
	ApplicationID string `json:"applicationId"`
	Code          string `json:"code"`
}

func (e *ApplicationDeactivated) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		ApplicationID string `json:"applicationId"`
		Code          string `json:"code"`
	}{
		ApplicationID: e.ApplicationID,
		Code:          e.Code,
	})
}

func NewApplicationDeactivated(ctx *common.ExecutionContext, app *application.Application) *ApplicationDeactivated {
	return &ApplicationDeactivated{
		BaseDomainEvent: newBase(ctx, EventTypeApplicationDeactivated, "platform", "application", app.ID),
		ApplicationID:   app.ID,
		Code:            app.Code,
	}
}

// ApplicationProvisioned is emitted when an application is provisioned for a client
type ApplicationProvisioned struct {
	common.BaseDomainEvent
	ApplicationID string `json:"applicationId"`
	Code          string `json:"code"`
	ClientID      string `json:"clientId"`
	ConfigID      string `json:"configId,omitempty"`
}

func (e *ApplicationProvisioned) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		ApplicationID string `json:"applicationId"`
		Code          string `json:"code"`
		ClientID      string `json:"clientId"`
		ConfigID      string `json:"configId,omitempty"`
	}{
		ApplicationID: e.ApplicationID,
		Code:          e.Code,
		ClientID:      e.ClientID,
		ConfigID:      e.ConfigID,
	})
}

func NewApplicationProvisioned(ctx *common.ExecutionContext, app *application.Application, clientID, configID string) *ApplicationProvisioned {
	return &ApplicationProvisioned{
		BaseDomainEvent: newBase(ctx, EventTypeApplicationProvisioned, "platform", "application", app.ID),
		ApplicationID:   app.ID,
		Code:            app.Code,
		ClientID:        clientID,
		ConfigID:        configID,
	}
}
