package events

import (
	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/principal"
)

// PrincipalUserCreated is emitted when a new user is created
type PrincipalUserCreated struct {
	common.BaseDomainEvent
	UserID         string `json:"userId"`
	Email          string `json:"email"`
	Name           string `json:"name"`
	Scope          string `json:"scope"`
	UserClientID   string `json:"clientId,omitempty"`
}

func (e *PrincipalUserCreated) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		UserID       string `json:"userId"`
		Email        string `json:"email"`
		Name         string `json:"name"`
		Scope        string `json:"scope"`
		UserClientID string `json:"clientId,omitempty"`
	}{
		UserID:       e.UserID,
		Email:        e.Email,
		Name:         e.Name,
		Scope:        e.Scope,
		UserClientID: e.UserClientID,
	})
}

func NewPrincipalUserCreated(ctx *common.ExecutionContext, p *principal.Principal) *PrincipalUserCreated {
	email := ""
	if p.UserIdentity != nil {
		email = p.UserIdentity.Email
	}
	return &PrincipalUserCreated{
		BaseDomainEvent: newBase(ctx, EventTypePrincipalUserCreated, "platform", "principal", p.ID),
		UserID:          p.ID,
		Email:           email,
		Name:            p.Name,
		Scope:           string(p.Scope),
		UserClientID:    p.ClientID,
	}
}

// PrincipalUserUpdated is emitted when a user is updated
type PrincipalUserUpdated struct {
	common.BaseDomainEvent
	UserID string `json:"userId"`
	Email  string `json:"email"`
	Name   string `json:"name"`
}

func (e *PrincipalUserUpdated) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		UserID string `json:"userId"`
		Email  string `json:"email"`
		Name   string `json:"name"`
	}{
		UserID: e.UserID,
		Email:  e.Email,
		Name:   e.Name,
	})
}

func NewPrincipalUserUpdated(ctx *common.ExecutionContext, p *principal.Principal) *PrincipalUserUpdated {
	email := ""
	if p.UserIdentity != nil {
		email = p.UserIdentity.Email
	}
	return &PrincipalUserUpdated{
		BaseDomainEvent: newBase(ctx, EventTypePrincipalUserUpdated, "platform", "principal", p.ID),
		UserID:          p.ID,
		Email:           email,
		Name:            p.Name,
	}
}

// PrincipalUserActivated is emitted when a user is activated
type PrincipalUserActivated struct {
	common.BaseDomainEvent
	UserID string `json:"userId"`
	Email  string `json:"email"`
}

func (e *PrincipalUserActivated) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		UserID string `json:"userId"`
		Email  string `json:"email"`
	}{
		UserID: e.UserID,
		Email:  e.Email,
	})
}

func NewPrincipalUserActivated(ctx *common.ExecutionContext, p *principal.Principal) *PrincipalUserActivated {
	email := ""
	if p.UserIdentity != nil {
		email = p.UserIdentity.Email
	}
	return &PrincipalUserActivated{
		BaseDomainEvent: newBase(ctx, EventTypePrincipalUserActivated, "platform", "principal", p.ID),
		UserID:          p.ID,
		Email:           email,
	}
}

// PrincipalUserDeactivated is emitted when a user is deactivated
type PrincipalUserDeactivated struct {
	common.BaseDomainEvent
	UserID string `json:"userId"`
	Email  string `json:"email"`
}

func (e *PrincipalUserDeactivated) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		UserID string `json:"userId"`
		Email  string `json:"email"`
	}{
		UserID: e.UserID,
		Email:  e.Email,
	})
}

func NewPrincipalUserDeactivated(ctx *common.ExecutionContext, p *principal.Principal) *PrincipalUserDeactivated {
	email := ""
	if p.UserIdentity != nil {
		email = p.UserIdentity.Email
	}
	return &PrincipalUserDeactivated{
		BaseDomainEvent: newBase(ctx, EventTypePrincipalUserDeactivated, "platform", "principal", p.ID),
		UserID:          p.ID,
		Email:           email,
	}
}

// PrincipalUserDeleted is emitted when a user is deleted
type PrincipalUserDeleted struct {
	common.BaseDomainEvent
	UserID string `json:"userId"`
	Email  string `json:"email"`
}

func (e *PrincipalUserDeleted) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		UserID string `json:"userId"`
		Email  string `json:"email"`
	}{
		UserID: e.UserID,
		Email:  e.Email,
	})
}

func NewPrincipalUserDeleted(ctx *common.ExecutionContext, p *principal.Principal) *PrincipalUserDeleted {
	email := ""
	if p.UserIdentity != nil {
		email = p.UserIdentity.Email
	}
	return &PrincipalUserDeleted{
		BaseDomainEvent: newBase(ctx, EventTypePrincipalUserDeleted, "platform", "principal", p.ID),
		UserID:          p.ID,
		Email:           email,
	}
}

// PrincipalRolesAssigned is emitted when roles are assigned to a principal
type PrincipalRolesAssigned struct {
	common.BaseDomainEvent
	TargetID  string   `json:"targetId"`
	RoleIDs   []string `json:"roleIds"`
	RoleNames []string `json:"roleNames"`
}

func (e *PrincipalRolesAssigned) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		TargetID  string   `json:"targetId"`
		RoleIDs   []string `json:"roleIds"`
		RoleNames []string `json:"roleNames"`
	}{
		TargetID:  e.TargetID,
		RoleIDs:   e.RoleIDs,
		RoleNames: e.RoleNames,
	})
}

func NewPrincipalRolesAssigned(ctx *common.ExecutionContext, p *principal.Principal, roleIDs, roleNames []string) *PrincipalRolesAssigned {
	return &PrincipalRolesAssigned{
		BaseDomainEvent: newBase(ctx, EventTypePrincipalRolesAssigned, "platform", "principal", p.ID),
		TargetID:        p.ID,
		RoleIDs:         roleIDs,
		RoleNames:       roleNames,
	}
}

// PrincipalClientAccessGranted is emitted when a principal is granted access to a client
type PrincipalClientAccessGranted struct {
	common.BaseDomainEvent
	TargetID       string `json:"targetId"`
	GrantedClientID string `json:"clientId"`
}

func (e *PrincipalClientAccessGranted) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		TargetID       string `json:"targetId"`
		GrantedClientID string `json:"clientId"`
	}{
		TargetID:       e.TargetID,
		GrantedClientID: e.GrantedClientID,
	})
}

func NewPrincipalClientAccessGranted(ctx *common.ExecutionContext, principalID, clientID string) *PrincipalClientAccessGranted {
	return &PrincipalClientAccessGranted{
		BaseDomainEvent: newBase(ctx, EventTypePrincipalClientAccessGranted, "platform", "principal", principalID),
		TargetID:        principalID,
		GrantedClientID: clientID,
	}
}

// PrincipalClientAccessRevoked is emitted when a principal's access to a client is revoked
type PrincipalClientAccessRevoked struct {
	common.BaseDomainEvent
	TargetID        string `json:"targetId"`
	RevokedClientID string `json:"clientId"`
}

func (e *PrincipalClientAccessRevoked) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		TargetID        string `json:"targetId"`
		RevokedClientID string `json:"clientId"`
	}{
		TargetID:        e.TargetID,
		RevokedClientID: e.RevokedClientID,
	})
}

func NewPrincipalClientAccessRevoked(ctx *common.ExecutionContext, principalID, clientID string) *PrincipalClientAccessRevoked {
	return &PrincipalClientAccessRevoked{
		BaseDomainEvent: newBase(ctx, EventTypePrincipalClientAccessRevoked, "platform", "principal", principalID),
		TargetID:        principalID,
		RevokedClientID: clientID,
	}
}
