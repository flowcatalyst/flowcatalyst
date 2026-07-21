package operations

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// SyncPlatformRolesCommand is intentionally empty — the catalogue is
// static code, not user input. The command exists so the audit log
// records "SyncPlatformRolesCommand" alongside each per-row event.
type SyncPlatformRolesCommand struct{}

// SyncPlatformRoles upserts the static `code_roles` catalogue (passed in by the
// caller, e.g. seed.PlatformRoles()) into the database and emits the per-row
// Created / Updated / Deleted events plus the [RolesSynced] rollup atomically
// via [usecaseop.Sync]. Drop-in parity with Rust's
// RoleSyncService::sync_code_defined_roles.
//
// Authorization is [usecaseop.Public]: the catalogue is static code with no
// client or application dimension, so there is no per-resource check. The sole
// entry point (the anchor-only BFF /bff/roles/sync-platform) does the coarse
// gate (RequireAnchor).
//
// Rules (matching Rust):
//   - For each role in the catalogue:
//   - If a row with the same name and source=CODE exists, update it.
//   - If a row with the same name and a non-CODE source exists, skip
//     (logged as a warning — the operator chose a non-CODE replacement).
//   - Otherwise insert a fresh row with source=CODE.
//   - For each CODE-sourced row in the database whose name no longer
//     appears in the catalogue: delete IF it has no assignments;
//     skip + warn if any principal still holds it.
func SyncPlatformRoles(repo *role.Repository, codeRoles []role.Role) usecaseop.Operation[SyncPlatformRolesCommand, RolesSynced] {
	return usecaseop.Operation[SyncPlatformRolesCommand, RolesSynced]{
		Name:      "SyncPlatformRoles",
		Authorize: usecaseop.Public[SyncPlatformRolesCommand],
		Execute: func(ctx context.Context, _ SyncPlatformRolesCommand, ec usecase.ExecutionContext) (usecaseop.Plan[RolesSynced], error) {
			var (
				saves      []usecasepgx.SyncSaveItem[role.Role]
				deletes    []usecasepgx.SyncDeleteItem[role.Role]
				created    uint32
				updated    uint32
				removed    uint32
				codeByName = make(map[string]struct{}, len(codeRoles))
			)

			for i := range codeRoles {
				def := codeRoles[i]
				codeByName[def.Name] = struct{}{}

				existing, err := repo.FindByName(ctx, def.Name)
				if err != nil {
					return nil, usecase.Internal("REPO", "find_by_name failed for "+def.Name, err)
				}
				if existing != nil {
					if existing.Source != role.SourceCode {
						slog.Warn("role exists with non-CODE source; skipping platform-role sync",
							"role", def.Name, "source", string(existing.Source))
						continue
					}
					existing.DisplayName = def.DisplayName
					existing.Description = def.Description
					existing.Permissions = append([]string(nil), def.Permissions...)
					saves = append(saves, usecasepgx.SyncSaveItem[role.Role]{
						Aggregate: existing,
						Event: RoleUpdated{
							Metadata: usecase.NewEventMetadata(ec, RoleUpdatedType, Source, subjectFor(existing.ID)),
							RoleID:   existing.ID,
							Name:     existing.Name,
						},
					})
					updated++
					continue
				}
				// Fresh insert — use a new id + now timestamps, then carry the
				// catalogue's display/description/permissions.
				row := role.New(def.ApplicationCode, splitRoleName(def.Name, def.ApplicationCode), def.DisplayName)
				row.Description = def.Description
				row.Permissions = append([]string(nil), def.Permissions...)
				row.Source = role.SourceCode
				saves = append(saves, usecasepgx.SyncSaveItem[role.Role]{
					Aggregate: row,
					Event: RoleCreated{
						Metadata: usecase.NewEventMetadata(ec, RoleCreatedType, Source, subjectFor(row.ID)),
						RoleID:   row.ID,
						Name:     row.Name,
					},
				})
				created++
			}

			// Stale CODE-sourced rows: present in DB, absent from the catalogue.
			codeRows, err := repo.FindBySource(ctx, role.SourceCode)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_source(CODE) failed", err)
			}
			for i := range codeRows {
				cur := codeRows[i]
				if _, present := codeByName[cur.Name]; present {
					continue
				}
				count, err := repo.CountAssignments(ctx, cur.Name)
				if err != nil {
					return nil, usecase.Internal("REPO", "count_assignments failed for "+cur.Name, err)
				}
				if count > 0 {
					slog.Warn("stale CODE role still assigned to principals; refusing to remove",
						"role", cur.Name, "assignments", count)
					continue
				}
				deletes = append(deletes, usecasepgx.SyncDeleteItem[role.Role]{
					Aggregate: &cur,
					Event: RoleDeleted{
						Metadata: usecase.NewEventMetadata(ec, RoleDeletedType, Source, subjectFor(cur.ID)),
						RoleID:   cur.ID,
						Name:     cur.Name,
					},
				})
				removed++
			}

			rollup := RolesSynced{
				Metadata: usecase.NewEventMetadata(ec, RolesSyncedType, Source, "platform.roles"),
				Created:  created,
				Updated:  updated,
				Removed:  removed,
				Total:    uint32(len(codeRoles)),
			}
			return usecaseop.Sync(repo, saves, deletes, rollup), nil
		},
	}
}

// ── Application-scoped SDK role sync ──────────────────────────────────────

// SyncRoleInput is one role definition in an SDK sync payload. Mirrors the
// Rust SyncRoleInput (camelCase wire shape lives in the sdksync API layer).
type SyncRoleInput struct {
	Name          string
	DisplayName   *string
	Description   *string
	Permissions   []string
	ClientManaged bool
}

// SyncRolesCommand syncs one application's SDK-sourced roles. ApplicationID
// is resolved by the caller (the sdksync handler resolves {appCode} → app);
// the use case authorizes against it (CanAccessApplication), scopes the
// existing-roles lookup, and stamps application_id on freshly-created rows.
type SyncRolesCommand struct {
	ApplicationCode string
	ApplicationID   string
	Roles           []SyncRoleInput
	RemoveUnlisted  bool
}

// SyncRoles bulk-upserts an application's SDK role catalogue within a single
// transaction. Mirrors Rust SyncRolesUseCase exactly:
//
//   - Each role's canonical name is "{applicationCode}:{name.toLower()}".
//   - Only SDK-sourced rows are created/updated/removed; CODE and DATABASE
//     rows are never touched (an existing non-SDK row with the same name is
//     left untouched, neither updated nor counted).
//   - RemoveUnlisted prunes SDK-sourced rows absent from the payload, but
//     REFUSES (business-rule error ROLE_HAS_ASSIGNMENTS) when a role still
//     has principal assignments — the junction has no DB-level FK, so a
//     silent drop would orphan user role assignments.
//
// Authorization: the controller does the coarse "may sync roles" check and
// resolves the application; the use case enforces the per-resource rule — the
// caller must have access to the target application (CanAccessApplication).
//
// Emits per-row [RoleCreated]/[RoleUpdated]/[RoleDeleted] events plus one
// [RolesSynced] rollup, all atomic with the row writes via [usecaseop.Sync].
func SyncRoles(repo *role.Repository) usecaseop.Operation[SyncRolesCommand, RolesSynced] {
	return usecaseop.Operation[SyncRolesCommand, RolesSynced]{
		Name: "SyncRoles",
		Validate: func(_ context.Context, cmd SyncRolesCommand) error {
			if strings.TrimSpace(cmd.ApplicationCode) == "" {
				return usecase.Validation("APPLICATION_CODE_REQUIRED", "Application code is required")
			}
			if len(cmd.Roles) == 0 {
				return usecase.Validation("ROLES_REQUIRED", "At least one role must be provided")
			}
			return nil
		},
		Authorize: func(ctx context.Context, cmd SyncRolesCommand) error {
			if !auth.FromContext(ctx).CanAccessApplication(cmd.ApplicationID) {
				return httperror.Forbidden("Not authorised for application '" + cmd.ApplicationCode + "'")
			}
			return nil
		},
		Execute: func(ctx context.Context, cmd SyncRolesCommand, ec usecase.ExecutionContext) (usecaseop.Plan[RolesSynced], error) {
			existing, err := repo.FindByApplicationID(ctx, cmd.ApplicationID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_application failed", err)
			}
			existingByName := make(map[string]*role.Role, len(existing))
			for i := range existing {
				existingByName[existing[i].Name] = &existing[i]
			}

			var (
				saves       []usecasepgx.SyncSaveItem[role.Role]
				deletes     []usecasepgx.SyncDeleteItem[role.Role]
				syncedNames = make([]string, 0, len(cmd.Roles))
				syncedSet   = make(map[string]struct{}, len(cmd.Roles))
				created     uint32
				updated     uint32
				deleted     uint32
			)

			for _, in := range cmd.Roles {
				// Accept both a bare name ("hr-manager") and an already-qualified
				// one ("hr:hr-manager", or a malformed multi-colon
				// "hr:dashboard:user"): strip a leading "{appCode}:" so we never
				// double-prefix, matching the definition-sync path above.
				short := splitRoleName(strings.ToLower(in.Name), cmd.ApplicationCode)
				fullName := cmd.ApplicationCode + ":" + short
				syncedNames = append(syncedNames, fullName)
				syncedSet[fullName] = struct{}{}

				if cur, ok := existingByName[fullName]; ok {
					if cur.Source != role.SourceSDK {
						continue // never touch CODE/DATABASE-managed rows
					}
					cur.DisplayName = displayNameOr(in.DisplayName, in.Name)
					cur.Description = in.Description
					// Preserve existing permissions when the payload supplies none.
					// Apps typically declare role NAMES while permissions are curated
					// in the FlowCatalyst UI; an empty/omitted list must NOT wipe them.
					// Only a non-empty list replaces the stored permissions.
					if len(in.Permissions) > 0 {
						cur.Permissions = append([]string(nil), in.Permissions...)
					}
					cur.ClientManaged = in.ClientManaged
					saves = append(saves, usecasepgx.SyncSaveItem[role.Role]{
						Aggregate: cur,
						Event: RoleUpdated{
							Metadata: usecase.NewEventMetadata(ec, RoleUpdatedType, Source, subjectFor(cur.ID)),
							RoleID:   cur.ID,
							Name:     cur.Name,
						},
					})
					updated++
					continue
				}

				row := role.New(cmd.ApplicationCode, short, displayNameOr(in.DisplayName, in.Name))
				if cmd.ApplicationID != "" {
					appID := cmd.ApplicationID
					row.ApplicationID = &appID
				}
				row.Source = role.SourceSDK
				row.Description = in.Description
				row.Permissions = append([]string(nil), in.Permissions...)
				row.ClientManaged = in.ClientManaged
				saves = append(saves, usecasepgx.SyncSaveItem[role.Role]{
					Aggregate: row,
					Event: RoleCreated{
						Metadata: usecase.NewEventMetadata(ec, RoleCreatedType, Source, subjectFor(row.ID)),
						RoleID:   row.ID,
						Name:     row.Name,
					},
				})
				created++
			}

			if cmd.RemoveUnlisted {
				for i := range existing {
					cur := &existing[i]
					if cur.Source != role.SourceSDK {
						continue
					}
					if _, present := syncedSet[cur.Name]; present {
						continue
					}
					count, err := repo.CountAssignments(ctx, cur.Name)
					if err != nil {
						return nil, usecase.Internal("REPO", "count_assignments failed for "+cur.Name, err)
					}
					if count > 0 {
						return nil, usecase.BusinessRule("ROLE_HAS_ASSIGNMENTS",
							fmt.Sprintf("Cannot remove role '%s' — %d principal(s) still hold it. "+
								"Strip the assignments before syncing.", cur.Name, count))
					}
					deletes = append(deletes, usecasepgx.SyncDeleteItem[role.Role]{
						Aggregate: cur,
						Event: RoleDeleted{
							Metadata: usecase.NewEventMetadata(ec, RoleDeletedType, Source, subjectFor(cur.ID)),
							RoleID:   cur.ID,
							Name:     cur.Name,
						},
					})
					deleted++
				}
			}

			rollup := RolesSynced{
				Metadata:        usecase.NewEventMetadata(ec, RolesSyncedType, Source, "platform.roles."+cmd.ApplicationCode),
				Created:         created,
				Updated:         updated,
				Removed:         deleted,
				Total:           uint32(len(cmd.Roles)),
				ApplicationCode: cmd.ApplicationCode,
				SyncedCodes:     syncedNames,
			}
			return usecaseop.Sync(repo, saves, deletes, rollup), nil
		},
	}
}

// displayNameOr returns *dn when non-nil, else the fallback. Mirrors Rust
// `display_name.unwrap_or_else(|| name.clone())`.
func displayNameOr(dn *string, fallback string) string {
	if dn != nil {
		return *dn
	}
	return fallback
}

// splitRoleName recovers the short role name from a fully-qualified
// "{applicationCode}:{roleName}" identifier. role.New(...) joins the
// two back together with ":", so we need to strip the prefix before
// re-construction.
func splitRoleName(fullName, applicationCode string) string {
	prefix := applicationCode + ":"
	if len(fullName) > len(prefix) && fullName[:len(prefix)] == prefix {
		return fullName[len(prefix):]
	}
	return fullName
}
