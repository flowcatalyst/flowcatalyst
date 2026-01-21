package events

import (
	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/serviceaccount"
)

// ServiceAccountCreated is emitted when a new service account is created
type ServiceAccountCreated struct {
	common.BaseDomainEvent
	ServiceAccountID string   `json:"serviceAccountId"`
	Code             string   `json:"code"`
	Name             string   `json:"name"`
	ApplicationID    string   `json:"applicationId,omitempty"`
	ClientIDs        []string `json:"clientIds,omitempty"`
}

func (e *ServiceAccountCreated) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		ServiceAccountID string   `json:"serviceAccountId"`
		Code             string   `json:"code"`
		Name             string   `json:"name"`
		ApplicationID    string   `json:"applicationId,omitempty"`
		ClientIDs        []string `json:"clientIds,omitempty"`
	}{
		ServiceAccountID: e.ServiceAccountID,
		Code:             e.Code,
		Name:             e.Name,
		ApplicationID:    e.ApplicationID,
		ClientIDs:        e.ClientIDs,
	})
}

func NewServiceAccountCreated(ctx *common.ExecutionContext, sa *serviceaccount.ServiceAccount) *ServiceAccountCreated {
	return &ServiceAccountCreated{
		BaseDomainEvent:  newBase(ctx, EventTypeServiceAccountCreated, "platform", "serviceaccount", sa.ID),
		ServiceAccountID: sa.ID,
		Code:             sa.Code,
		Name:             sa.Name,
		ApplicationID:    sa.ApplicationID,
		ClientIDs:        sa.ClientIDs,
	}
}

// ServiceAccountCredentialsRotated is emitted when service account credentials are rotated
type ServiceAccountCredentialsRotated struct {
	common.BaseDomainEvent
	ServiceAccountID string `json:"serviceAccountId"`
	Code             string `json:"code"`
	Name             string `json:"name"`
}

func (e *ServiceAccountCredentialsRotated) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		ServiceAccountID string `json:"serviceAccountId"`
		Code             string `json:"code"`
		Name             string `json:"name"`
	}{
		ServiceAccountID: e.ServiceAccountID,
		Code:             e.Code,
		Name:             e.Name,
	})
}

func NewServiceAccountCredentialsRotated(ctx *common.ExecutionContext, sa *serviceaccount.ServiceAccount) *ServiceAccountCredentialsRotated {
	return &ServiceAccountCredentialsRotated{
		BaseDomainEvent:  newBase(ctx, EventTypeServiceAccountCredentialsRotated, "platform", "serviceaccount", sa.ID),
		ServiceAccountID: sa.ID,
		Code:             sa.Code,
		Name:             sa.Name,
	}
}

// ServiceAccountDeleted is emitted when a service account is deleted
type ServiceAccountDeleted struct {
	common.BaseDomainEvent
	ServiceAccountID string `json:"serviceAccountId"`
	Code             string `json:"code"`
	Name             string `json:"name"`
}

func (e *ServiceAccountDeleted) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		ServiceAccountID string `json:"serviceAccountId"`
		Code             string `json:"code"`
		Name             string `json:"name"`
	}{
		ServiceAccountID: e.ServiceAccountID,
		Code:             e.Code,
		Name:             e.Name,
	})
}

func NewServiceAccountDeleted(ctx *common.ExecutionContext, sa *serviceaccount.ServiceAccount) *ServiceAccountDeleted {
	return &ServiceAccountDeleted{
		BaseDomainEvent:  newBase(ctx, EventTypeServiceAccountDeleted, "platform", "serviceaccount", sa.ID),
		ServiceAccountID: sa.ID,
		Code:             sa.Code,
		Name:             sa.Name,
	}
}
