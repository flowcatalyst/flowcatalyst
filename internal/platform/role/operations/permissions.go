package operations

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// GrantPermissionCommand grants `Permission` on the role identified by
// name. Idempotent: re-granting an existing permission still emits the
// event so the audit trail records the admin action.
type GrantPermissionCommand struct {
	RoleName   string `json:"roleName"`
	Permission string `json:"permission"`
}

// GrantPermission adds Permission to the role's permission set and
// emits [RolePermissionGranted].
func GrantPermission(repo *role.Repository) usecaseop.Operation[GrantPermissionCommand, RolePermissionGranted] {
	return usecaseop.Operation[GrantPermissionCommand, RolePermissionGranted]{
		Name: "GrantPermission",
		Validate: func(_ context.Context, cmd GrantPermissionCommand) error {
			if strings.TrimSpace(cmd.RoleName) == "" {
				return usecase.Validation("ROLE_NAME_REQUIRED", "Role name is required")
			}
			if strings.TrimSpace(cmd.Permission) == "" {
				return usecase.Validation("PERMISSION_REQUIRED", "Permission is required")
			}
			return nil
		},
		// Roles are global (no per-client resource dimension), so there is no
		// use-case-level authz; the coarse CanWriteRoles permission is enforced
		// at the controller.
		Authorize: usecaseop.Public[GrantPermissionCommand],
		Execute: func(ctx context.Context, cmd GrantPermissionCommand, ec usecase.ExecutionContext) (usecaseop.Plan[RolePermissionGranted], error) {
			r, err := repo.FindByName(ctx, cmd.RoleName)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_name failed", err)
			}
			if r == nil {
				return nil, httperror.NotFound("Role", cmd.RoleName)
			}
			r.GrantPermission(cmd.Permission)
			event := RolePermissionGranted{
				Metadata:   usecase.NewEventMetadata(ec, RolePermissionGrantedType, Source, subjectFor(r.ID)),
				RoleID:     r.ID,
				RoleName:   r.Name,
				Permission: cmd.Permission,
			}
			return usecaseop.Save(r, repo, event), nil
		},
	}
}

// RevokePermissionCommand removes a permission from the role.
type RevokePermissionCommand struct {
	RoleName   string `json:"roleName"`
	Permission string `json:"permission"`
}

// RevokePermission removes the permission and emits [RolePermissionRevoked].
func RevokePermission(repo *role.Repository) usecaseop.Operation[RevokePermissionCommand, RolePermissionRevoked] {
	return usecaseop.Operation[RevokePermissionCommand, RolePermissionRevoked]{
		Name: "RevokePermission",
		Validate: func(_ context.Context, cmd RevokePermissionCommand) error {
			if strings.TrimSpace(cmd.RoleName) == "" {
				return usecase.Validation("ROLE_NAME_REQUIRED", "Role name is required")
			}
			if strings.TrimSpace(cmd.Permission) == "" {
				return usecase.Validation("PERMISSION_REQUIRED", "Permission is required")
			}
			return nil
		},
		// Roles are global (no per-client resource dimension), so there is no
		// use-case-level authz; the coarse CanWriteRoles permission is enforced
		// at the controller.
		Authorize: usecaseop.Public[RevokePermissionCommand],
		Execute: func(ctx context.Context, cmd RevokePermissionCommand, ec usecase.ExecutionContext) (usecaseop.Plan[RolePermissionRevoked], error) {
			r, err := repo.FindByName(ctx, cmd.RoleName)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_name failed", err)
			}
			if r == nil {
				return nil, httperror.NotFound("Role", cmd.RoleName)
			}
			r.RevokePermission(cmd.Permission)
			event := RolePermissionRevoked{
				Metadata:   usecase.NewEventMetadata(ec, RolePermissionRevokedType, Source, subjectFor(r.ID)),
				RoleID:     r.ID,
				RoleName:   r.Name,
				Permission: cmd.Permission,
			}
			return usecaseop.Save(r, repo, event), nil
		},
	}
}

// RolePermissionGranted — emitted on grant.
type RolePermissionGranted struct {
	Metadata   usecase.EventMetadata
	RoleID     string
	RoleName   string
	Permission string
}

func (e RolePermissionGranted) EventID() string       { return e.Metadata.EventID }
func (e RolePermissionGranted) EventType() string     { return RolePermissionGrantedType }
func (e RolePermissionGranted) SpecVersion() string   { return "1.0" }
func (e RolePermissionGranted) Source() string        { return Source }
func (e RolePermissionGranted) Subject() string       { return subjectFor(e.RoleID) }
func (e RolePermissionGranted) Time() time.Time       { return e.Metadata.OccurredAt }
func (e RolePermissionGranted) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e RolePermissionGranted) CorrelationID() string { return e.Metadata.CorrelationID }
func (e RolePermissionGranted) CausationID() string   { return e.Metadata.CausationID }
func (e RolePermissionGranted) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e RolePermissionGranted) MessageGroup() string  { return groupFor(e.RoleID) }
func (e RolePermissionGranted) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		RoleID     string `json:"roleId"`
		RoleName   string `json:"roleName"`
		Permission string `json:"permission"`
	}{e.RoleID, e.RoleName, e.Permission})
}

// RolePermissionRevoked — emitted on revoke.
type RolePermissionRevoked struct {
	Metadata   usecase.EventMetadata
	RoleID     string
	RoleName   string
	Permission string
}

func (e RolePermissionRevoked) EventID() string       { return e.Metadata.EventID }
func (e RolePermissionRevoked) EventType() string     { return RolePermissionRevokedType }
func (e RolePermissionRevoked) SpecVersion() string   { return "1.0" }
func (e RolePermissionRevoked) Source() string        { return Source }
func (e RolePermissionRevoked) Subject() string       { return subjectFor(e.RoleID) }
func (e RolePermissionRevoked) Time() time.Time       { return e.Metadata.OccurredAt }
func (e RolePermissionRevoked) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e RolePermissionRevoked) CorrelationID() string { return e.Metadata.CorrelationID }
func (e RolePermissionRevoked) CausationID() string   { return e.Metadata.CausationID }
func (e RolePermissionRevoked) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e RolePermissionRevoked) MessageGroup() string  { return groupFor(e.RoleID) }
func (e RolePermissionRevoked) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		RoleID     string `json:"roleId"`
		RoleName   string `json:"roleName"`
		Permission string `json:"permission"`
	}{e.RoleID, e.RoleName, e.Permission})
}
