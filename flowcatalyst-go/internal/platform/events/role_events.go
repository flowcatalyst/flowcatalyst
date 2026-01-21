package events

import (
	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/role"
)

// RoleCreated is emitted when a new role is created
type RoleCreated struct {
	common.BaseDomainEvent
	RoleID      string   `json:"roleId"`
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Scope       string   `json:"scope,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
}

func (e *RoleCreated) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		RoleID      string   `json:"roleId"`
		Code        string   `json:"code"`
		Name        string   `json:"name"`
		Scope       string   `json:"scope,omitempty"`
		Permissions []string `json:"permissions,omitempty"`
	}{
		RoleID:      e.RoleID,
		Code:        e.Code,
		Name:        e.Name,
		Scope:       e.Scope,
		Permissions: e.Permissions,
	})
}

func NewRoleCreated(ctx *common.ExecutionContext, r *role.Role) *RoleCreated {
	return &RoleCreated{
		BaseDomainEvent: newBase(ctx, EventTypeRoleCreated, "platform", "role", r.ID),
		RoleID:          r.ID,
		Code:            r.Code,
		Name:            r.Name,
		Scope:           r.Scope,
		Permissions:     r.Permissions,
	}
}

// RoleUpdated is emitted when a role is updated
type RoleUpdated struct {
	common.BaseDomainEvent
	RoleID      string   `json:"roleId"`
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Permissions []string `json:"permissions,omitempty"`
}

func (e *RoleUpdated) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		RoleID      string   `json:"roleId"`
		Code        string   `json:"code"`
		Name        string   `json:"name"`
		Permissions []string `json:"permissions,omitempty"`
	}{
		RoleID:      e.RoleID,
		Code:        e.Code,
		Name:        e.Name,
		Permissions: e.Permissions,
	})
}

func NewRoleUpdated(ctx *common.ExecutionContext, r *role.Role) *RoleUpdated {
	return &RoleUpdated{
		BaseDomainEvent: newBase(ctx, EventTypeRoleUpdated, "platform", "role", r.ID),
		RoleID:          r.ID,
		Code:            r.Code,
		Name:            r.Name,
		Permissions:     r.Permissions,
	}
}

// RoleDeleted is emitted when a role is deleted
type RoleDeleted struct {
	common.BaseDomainEvent
	RoleID string `json:"roleId"`
	Code   string `json:"code"`
	Name   string `json:"name"`
}

func (e *RoleDeleted) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		RoleID string `json:"roleId"`
		Code   string `json:"code"`
		Name   string `json:"name"`
	}{
		RoleID: e.RoleID,
		Code:   e.Code,
		Name:   e.Name,
	})
}

func NewRoleDeleted(ctx *common.ExecutionContext, r *role.Role) *RoleDeleted {
	return &RoleDeleted{
		BaseDomainEvent: newBase(ctx, EventTypeRoleDeleted, "platform", "role", r.ID),
		RoleID:          r.ID,
		Code:            r.Code,
		Name:            r.Name,
	}
}
