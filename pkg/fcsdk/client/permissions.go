package client

import "context"

// Permissions are immutable platform constants; the only operations are
// list + get. Mutation of permission grants happens via the role itself
// — see RolesResource.GrantPermission / RevokePermission.

// PermissionResponse is one permission catalogue entry.
type PermissionResponse struct {
	Permission  string `json:"permission"`
	Application string `json:"application"`
	Context     string `json:"context"`
	Aggregate   string `json:"aggregate"`
	Action      string `json:"action"`
	Description string `json:"description"`
}

// PermissionListResponse — GET /api/roles/permissions.
type PermissionListResponse struct {
	Permissions []PermissionResponse `json:"permissions"`
	Total       uint64               `json:"total,omitempty"`
}

// PermissionsResource — /api/roles/permissions/*.
type PermissionsResource struct {
	c *FlowCatalystClient
}

// List — GET /api/roles/permissions.
func (r *PermissionsResource) List(ctx context.Context) (*PermissionListResponse, error) {
	var out PermissionListResponse
	if err := r.c.Get(ctx, "/api/roles/permissions", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Get — GET /api/roles/permissions/{name}.
func (r *PermissionsResource) Get(ctx context.Context, name string) (*PermissionResponse, error) {
	var out PermissionResponse
	if err := r.c.Get(ctx, "/api/roles/permissions/"+name, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
