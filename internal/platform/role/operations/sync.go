package operations

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/syncdiff"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// SyncPlatformRolesCommand is intentionally empty — the catalogue is
// static code, not user input. The command exists so the audit log
// records "SyncPlatformRolesCommand" alongside each per-row event.
type SyncPlatformRolesCommand struct{}

// SyncPlatformRoles upserts the static `code_roles` catalogue (passed in by the
// caller, e.g. seed.PlatformRoles()) into the database and emits the per-row
// Created / Updated / Deleted events plus the [RolesSynced] rollup atomically
// via [usecaseop.Sync].
//
// Authorization is [usecaseop.Public]: the catalogue is static code with no
// client or application dimension, so there is no per-resource check. The sole
// entry point (the anchor-only BFF /bff/roles/sync-platform) does the coarse
// gate (RequireAnchor and the three role permissions). Confinement (F12): a
// catalogue role may gain only its own application's permissions (the
// built-in catalogue is all platform roles with platform permissions), with no
// super-admin exception, 400 role.OutsideApplicationCode. The permission ceiling
// (F10) bounds the change: every platform permission the sync adds to or
// removes from a role (restoring a role trimmed by hand, sweeping a stale one)
// must be the caller's, or the whole sync is refused before any write, 403
// role.PermissionCeilingCode.
//
// Rules:
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
				changed    []string
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
					// F30: a catalogue role that already matches is left alone: not
					// saved, not counted, no event.
					changedRow := existing.DisplayName != def.DisplayName ||
						!syncdiff.SameText(existing.Description, def.Description) ||
						!syncdiff.SameSet(existing.Permissions, def.Permissions)
					existing.DisplayName = def.DisplayName
					existing.Description = def.Description
					// F18: heal a row written without its application's id.
					if existing.ApplicationID == nil {
						appID, err := ownerApplicationID(ctx, repo, existing.OwningApplicationCode())
						if err != nil {
							return nil, err
						}
						if appID != nil {
							changedRow = true
						}
						existing.ApplicationID = appID
					}
					if !changedRow {
						continue
					}
					if err := role.RequirePermissionsInApplication(existing.OwningApplicationCode(),
						role.AddedPermissions(existing.Permissions, def.Permissions), false); err != nil {
						return nil, err
					}
					changed = append(changed, role.ChangedPermissions(existing.Permissions, def.Permissions)...)
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
				appID, err := ownerApplicationID(ctx, repo, row.OwningApplicationCode())
				if err != nil {
					return nil, err
				}
				row.ApplicationID = appID
				if err := role.RequirePermissionsInApplication(row.OwningApplicationCode(), row.Permissions, false); err != nil {
					return nil, err
				}
				changed = append(changed, row.Permissions...)
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
				changed = append(changed, cur.Permissions...)
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

			if err := role.RequirePermissionsWithinCaller(auth.FromContext(ctx), changed); err != nil {
				return nil, err
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

// SyncRoleInput is one role definition in an SDK sync payload (the
// camelCase wire shape lives in the sdksync API layer).
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
// transaction:
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
// The permission ceiling (F10) bounds the change: a platform permission a
// synced role gains or loses (a removed role loses them all) must be the
// caller's, or the whole sync is refused before any write, 403
// role.PermissionCeilingCode. An application's own permissions are outside
// it, so an SDK syncing them passes for any caller. Before it, confinement
// (F12): a synced role may gain only this application's permissions, with no
// super-admin exception (400 role.OutsideApplicationCode, whole sync refused);
// a role that already holds another application's permission keeps it.
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
			// F18: every role name and permission is shape-checked up front, so
			// one malformed entry refuses the whole sync before any write.
			for _, in := range cmd.Roles {
				short := splitRoleName(strings.ToLower(in.Name), cmd.ApplicationCode)
				if err := role.ValidateName(cmd.ApplicationCode + ":" + short); err != nil {
					return err
				}
				if err := role.ValidatePermissions(in.Permissions); err != nil {
					return err
				}
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
				changed     []string
			)

			// F18: a row of this application written without its id (the admin
			// create used to leave it null) is invisible to the lookup above, so
			// the sync would try to create it again and collide on the unique
			// name. Adopt it: it is this application's role, give it its id.
			adopted := map[string]bool{}
			for _, in := range cmd.Roles {
				full := cmd.ApplicationCode + ":" + splitRoleName(strings.ToLower(in.Name), cmd.ApplicationCode)
				if _, ok := existingByName[full]; ok {
					continue
				}
				orphan, err := repo.FindByName(ctx, full)
				if err != nil {
					return nil, usecase.Internal("REPO", "find_by_name failed", err)
				}
				if orphan != nil && orphan.ApplicationID == nil && orphan.OwningApplicationCode() == cmd.ApplicationCode {
					appID := ids.ApplicationID(cmd.ApplicationID)
					orphan.ApplicationID = &appID
					existingByName[full] = orphan
					adopted[full] = true
				}
			}

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
					// F30: a role that already matches the definition is left alone:
					// not saved, not counted, no event. An adopted id-less role
					// always needs its application id written.
					wantName := displayNameOr(in.DisplayName, in.Name)
					if !adopted[fullName] && cur.DisplayName == wantName &&
						syncdiff.SameText(cur.Description, in.Description) &&
						cur.ClientManaged == in.ClientManaged &&
						(len(in.Permissions) == 0 || syncdiff.SameSet(cur.Permissions, in.Permissions)) {
						continue
					}
					cur.DisplayName = wantName
					cur.Description = in.Description
					// Preserve existing permissions when the payload supplies none.
					// Apps typically declare role NAMES while permissions are curated
					// in the FlowCatalyst UI; an empty/omitted list must NOT wipe them.
					// Only a non-empty list replaces the stored permissions.
					if len(in.Permissions) > 0 {
						if err := role.RequirePermissionsInApplication(cmd.ApplicationCode,
							role.AddedPermissions(cur.Permissions, in.Permissions), false); err != nil {
							return nil, err
						}
						changed = append(changed, role.ChangedPermissions(cur.Permissions, in.Permissions)...)
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
					row.ApplicationID = ids.PtrOf[ids.ApplicationID](&appID)
				}
				row.Source = role.SourceSDK
				row.Description = in.Description
				row.Permissions = append([]string(nil), in.Permissions...)
				row.ClientManaged = in.ClientManaged
				if err := role.RequirePermissionsInApplication(cmd.ApplicationCode, row.Permissions, false); err != nil {
					return nil, err
				}
				changed = append(changed, row.Permissions...)
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
					changed = append(changed, cur.Permissions...)
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

			if err := role.RequirePermissionsWithinCaller(auth.FromContext(ctx), changed); err != nil {
				return nil, err
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

// displayNameOr returns *dn when non-nil, else the fallback.
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

// ownerApplicationID resolves the id a role of application code carries (F18).
// An application that does not exist yields nil with a warning rather than an
// error: the built-in catalogue must still be installable on a database whose
// platform application row has not been seeded yet.
func ownerApplicationID(ctx context.Context, repo *role.Repository, code string) (*ids.ApplicationID, error) {
	id, err := repo.ApplicationIDByCode(ctx, code)
	if err != nil {
		return nil, usecase.Internal("REPO", "application_by_code failed", err)
	}
	if id == "" {
		slog.Warn("role written without an application id: no such application", "applicationCode", code)
		return nil, nil
	}
	return &id, nil
}
