// dto.go contains the wire-format types for the role API. Kept separate
// from operations.CreateCommand/UpdateCommand so the wire shape can
// evolve independently of the use case input shape.
package api

import (
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
)

// CreateRoleRequest is the wire body for POST /api/roles.
type CreateRoleRequest struct {
	ApplicationCode string   `json:"applicationCode" doc:"Application code (e.g. platform, iam)"`
	RoleName        string   `json:"roleName" doc:"Role name within the application"`
	DisplayName     string   `json:"displayName" doc:"Human-readable role name"`
	Description     *string  `json:"description,omitempty"`
	Permissions     []string `json:"permissions,omitempty" doc:"Permission codes assigned to the role"`
	ClientManaged   bool     `json:"clientManaged" doc:"Whether the role is managed at client scope"`
}

func (r CreateRoleRequest) toCommand() operations.CreateCommand {
	return operations.CreateCommand{
		ApplicationCode: r.ApplicationCode,
		RoleName:        r.RoleName,
		DisplayName:     r.DisplayName,
		Description:     r.Description,
		Permissions:     r.Permissions,
		ClientManaged:   r.ClientManaged,
	}
}

// UpdateRoleRequest is the wire body for PUT /api/roles/{id}.
type UpdateRoleRequest struct {
	DisplayName   *string  `json:"displayName,omitempty"`
	Description   *string  `json:"description,omitempty"`
	Permissions   []string `json:"permissions,omitempty"`
	ClientManaged *bool    `json:"clientManaged,omitempty"`
}

func (r UpdateRoleRequest) toCommand(id string) operations.UpdateCommand {
	return operations.UpdateCommand{
		ID:            id,
		DisplayName:   r.DisplayName,
		Description:   r.Description,
		Permissions:   r.Permissions,
		ClientManaged: r.ClientManaged,
	}
}

// RoleResponse mirrors role.Role with explicit JSON tags. The wire shape
// is stable independent of entity-field renames.
type RoleResponse struct {
	ID              string          `json:"id"`
	ApplicationID   *string         `json:"applicationId,omitempty"`
	Name            string          `json:"name"`
	DisplayName     string          `json:"displayName"`
	Description     *string         `json:"description,omitempty"`
	ApplicationCode string          `json:"applicationCode"`
	Permissions     []string        `json:"permissions"`
	Source          string          `json:"source"`
	ClientManaged   bool            `json:"clientManaged"`
	CreatedAt       httpcompat.Time `json:"createdAt"`
	UpdatedAt       httpcompat.Time `json:"updatedAt"`
}

func fromEntity(r *role.Role) RoleResponse {
	perms := r.Permissions
	if perms == nil {
		perms = []string{}
	}
	return RoleResponse{
		ID:              r.ID,
		ApplicationID:   r.ApplicationID,
		Name:            r.Name,
		DisplayName:     r.DisplayName,
		Description:     r.Description,
		ApplicationCode: r.ApplicationCode,
		Permissions:     perms,
		Source:          string(r.Source),
		ClientManaged:   r.ClientManaged,
		CreatedAt:       jsontime.New(r.CreatedAt),
		UpdatedAt:       jsontime.New(r.UpdatedAt),
	}
}

// RoleListResponse is the wire shape for GET /api/roles. Matches the
// Rust fc-platform shape `{roles, total}`.
type RoleListResponse struct {
	Roles []RoleResponse `json:"roles"`
	Total int            `json:"total"`
}

// RolePermissionListResponse is the wire shape for
// GET /api/roles/{roleName}/permissions.
type RolePermissionListResponse struct {
	Permissions []string `json:"permissions"`
}

// PermissionResponse is the wire shape for a catalog row.
type PermissionResponse struct {
	Permission  string  `json:"permission"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	Category    *string `json:"category,omitempty"`
}

func permissionToResponse(p *role.Permission) PermissionResponse {
	return PermissionResponse{
		Permission:  p.Permission,
		Name:        p.Name,
		Description: p.Description,
		Category:    p.Category,
	}
}

// PermissionListResponse is the wire shape for GET /api/roles/permissions.
// Matches the Rust fc-platform shape `{permissions, total}`.
type PermissionListResponse struct {
	Permissions []PermissionResponse `json:"permissions"`
	Total       int                  `json:"total"`
}

// ApplicationFilterListResponse is the wire shape for
// GET /api/roles/filters/applications.
type ApplicationFilterListResponse struct {
	ApplicationCodes []string `json:"applicationCodes"`
}
