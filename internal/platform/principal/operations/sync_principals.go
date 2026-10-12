package operations

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// SdkSyncSource is the AssignmentSource value tagging role assignments that
// came from an SDK principal sync. Distinguishes them from ADMIN_ASSIGNED /
// IDP_SYNC rows so a sync only ever replaces its own set.
const SdkSyncSource = "SDK_SYNC"

// SyncPrincipalInput is one principal definition in an SDK sync payload.
// Roles are role short-names (the SDK prefixes them with the applicationCode).
type SyncPrincipalInput struct {
	Email  string
	Name   string
	Roles  []string
	Active bool
	// PasswordHash, when non-empty, is an already-hashed credential stored
	// verbatim on a principal the sync creates (no re-hashing) so a migrated
	// user keeps their existing password — the login flow verifies it
	// (bcrypt/argon2i/argon2id) and re-encodes it to the native scheme on
	// first success. It is never applied to an existing principal (F13, see
	// PasswordHashesIgnored), so a sync never changes a user's password.
	PasswordHash *string
}

// SyncPrincipalsCommand syncs user principals from an application SDK.
type SyncPrincipalsCommand struct {
	ApplicationCode string
	Principals      []SyncPrincipalInput
	RemoveUnlisted  bool
}

// SyncPrincipals upserts user principals from an application SDK within a
// single transaction:
//
//   - Matches by lowercased email. An existing principal keeps its non-SDK_SYNC
//     role assignments and other applications' SDK_SYNC assignments, and has
//     this sync's application's SDK_SYNC assignments (every SDK_SYNC one on the
//     platform route) replaced with the incoming set (F13); name and active are updated; a passwordHash is never
//     applied to it (F13). A new principal is created as a CLIENT-scoped USER
//     with the incoming SDK_SYNC roles and the passwordHash, if any.
//   - RemoveUnlisted does NOT delete principals: it strips SDK_SYNC role
//     assignments from any USER principal whose email is absent from the
//     payload (counted as "deactivated" in the rollup).
//
// Authorization is mostly delegated to two entry points with different gating
// — the app-scoped SDK sync (CanSyncPrincipals + per-application access) and
// the platform-level POST /api/principals/sync (CanSyncPrincipals only, no
// application). Each entry point keeps its own gate; baking either into the
// op would break the other. Users are global (matched by email), so there is
// no per-resource dimension the op itself could check — EXCEPT one: X-02(d)
// (docs/owner-rulings-todo.md #27) — a RemoveUnlisted sweep with no
// ApplicationCode is a platform-wide sweep (every application's SDK_SYNC
// roles are now up for stripping, see removeUnlisted below), so THAT
// combination is gated here, anchor-only, regardless of entry point.
//
// Target reach (F12, as Java and Rust): an existing principal is touched only
// within the caller's reach (administers: a non-anchor reaches only
// CLIENT-scope users homed at a client it can access). A listed email matching
// one out of reach refuses the whole sync, 403 SYNC_TARGET_FORBIDDEN, before
// any write; the RemoveUnlisted sweep skips such principals instead (it ranges
// over every user, so refusing would make any non-anchor sweep impossible).
// An application's provisioned service account is anchor-tier, so reach does
// not narrow it; its own roles bound what it can hand out.
//
// Application confinement (F12, as Java's requireSyncableRoles): on the
// application's route every role named must be that application's, whoever
// the caller (requireApplicationRoles, 403 ROLE_APP_FORBIDDEN); on the platform
// route a non-anchor may name only roles it could assign through
// PUT /api/principals/{id}/roles, application roles of an application the
// target's home client is entitled to, so none on a principal the sync creates
// (RequireAssignableRoles: 400 UNKNOWN_ROLE, 403 PLATFORM_ROLE_FORBIDDEN or
// ROLE_APP_FORBIDDEN). An anchor may name any role there, as before.
// clientConfigs resolves the entitlements; the application's route does not
// use it (nil is fine there).
//
// The role ceiling (role.RequireRolesWithinCaller, F8) bounds every role the
// sync adds or removes, on both entry points: the caller must hold each such
// role's platform permissions, or the whole sync is refused with 403
// ROLE_ABOVE_CALLER and nothing is written. Application roles holding only
// their application's permissions pass for any caller, so an SDK syncing its
// own roles is unaffected. On the application's route confinement already
// stops platform roles; the ceiling stops an own-application role a
// super-admin gave platform permissions (Java and Rust do the same since
// Go's F8).
//
// Emits per-row [UserCreated]/[UserUpdated] events plus one [PrincipalsSynced]
// rollup, all atomic with the writes via [usecaseop.Sync].
func SyncPrincipals(principals *principal.Repository, roles *role.Repository, clientConfigs *application.ClientConfigRepo) usecaseop.Operation[SyncPrincipalsCommand, PrincipalsSynced] {
	return usecaseop.Operation[SyncPrincipalsCommand, PrincipalsSynced]{
		Name: "SyncPrincipals",
		Validate: func(_ context.Context, cmd SyncPrincipalsCommand) error {
			// ApplicationCode is optional: the app-scoped SDK sync supplies it
			// (purely as event provenance — it is NOT used to scope or prefix
			// anything), while the platform-level POST /api/principals/sync
			// leaves it empty.
			if len(cmd.Principals) == 0 {
				return usecase.Validation("PRINCIPALS_REQUIRED", "At least one principal must be provided")
			}
			return nil
		},
		Authorize: func(ctx context.Context, cmd SyncPrincipalsCommand) error {
			if !cmd.RemoveUnlisted || cmd.ApplicationCode != "" {
				return nil
			}
			ac := auth.FromContext(ctx)
			if ac == nil || (!ac.IsAnchor() && !ac.IsSuperAdmin()) {
				return usecase.Authorization("ANCHOR_REQUIRED_FOR_PLATFORM_SWEEP",
					"Only anchor users may sweep (removeUnlisted) principal roles with no application scope")
			}
			return nil
		},
		Execute: func(ctx context.Context, cmd SyncPrincipalsCommand, ec usecase.ExecutionContext) (usecaseop.Plan[PrincipalsSynced], error) {
			ac := auth.FromContext(ctx)
			now := time.Now().UTC()
			sdkSource := SdkSyncSource

			var (
				saves        []usecasepgx.SyncSaveItem[principal.Principal]
				syncedEmails = make([]string, 0, len(cmd.Principals))
				syncedSet    = make(map[string]struct{}, len(cmd.Principals))
				created      uint32
				updated      uint32
				deactivated  uint32
				// changed collects every role the sync adds or removes, for
				// the role ceiling below.
				changed []string
			)
			// belongsToThisSync selects the assignments this sync owns, the
			// ones its merge replaces and its sweep strips (X-02(c), F13):
			// SDK_SYNC roles of this sync's application (role names are
			// app-prefixed "app:role", see role.New), or every SDK_SYNC role
			// on the platform-level sync (no ApplicationCode). Java
			// Principal.belongsToApplicationSync, Rust is_own_sdk_role.
			appPrefix := cmd.ApplicationCode + ":"
			belongsToThisSync := func(ra serviceaccount.RoleAssignment) bool {
				if ra.AssignmentSource == nil || *ra.AssignmentSource != SdkSyncSource {
					return false
				}
				return cmd.ApplicationCode == "" || strings.HasPrefix(ra.Role, appPrefix)
			}
			roleNames := func(ras []serviceaccount.RoleAssignment) []string {
				out := make([]string, 0, len(ras))
				for _, ra := range ras {
					out = append(out, ra.Role)
				}
				return out
			}

			for _, in := range cmd.Principals {
				email := strings.ToLower(in.Email)
				syncedEmails = append(syncedEmails, email)
				syncedSet[email] = struct{}{}

				names := make([]string, 0, len(in.Roles))
				roleAssignments := make([]serviceaccount.RoleAssignment, 0, len(in.Roles))
				for _, r := range in.Roles {
					names = append(names, strings.ToLower(r))
					roleAssignments = append(roleAssignments, serviceaccount.RoleAssignment{
						Role:             strings.ToLower(r),
						AssignmentSource: &sdkSource,
						AssignedAt:       now,
					})
				}

				existing, err := principals.FindByEmail(ctx, email)
				if err != nil {
					return nil, usecase.Internal("REPO", "find_by_email failed", err)
				}
				if existing != nil && !administers(ac, existing) {
					return nil, usecase.Authorization("SYNC_TARGET_FORBIDDEN",
						"Not authorised to manage principal '"+email+"'")
				}
				if err := requireSyncableRoles(ctx, roles, clientConfigs, ac, cmd.ApplicationCode, names, existing); err != nil {
					return nil, err
				}
				if existing != nil {
					// Merge (F13, Java Principal.syncSourcedRoles): replace only
					// this sync's own SDK_SYNC set (belongsToThisSync), keeping
					// every other assignment, another application's SDK_SYNC
					// roles included. A named role the principal still holds
					// keeps that assignment and its source, so an administrator's
					// grant is never relabelled SDK_SYNC (and later swept).
					merged := make([]serviceaccount.RoleAssignment, 0, len(existing.Roles)+len(roleAssignments))
					held := make(map[string]struct{}, len(existing.Roles))
					for _, ra := range existing.Roles {
						if belongsToThisSync(ra) {
							continue
						}
						merged = append(merged, ra)
						held[ra.Role] = struct{}{}
					}
					for _, ra := range roleAssignments {
						if _, ok := held[ra.Role]; ok {
							continue
						}
						held[ra.Role] = struct{}{}
						merged = append(merged, ra)
					}
					changed = append(changed, role.ChangedRoles(roleNames(existing.Roles), roleNames(merged))...)
					existing.Roles = merged
					existing.Name = in.Name
					existing.Active = in.Active
					existing.UpdatedAt = now
					// Never on an existing principal, whoever the caller (F13, owner
					// ruling 2026-09-25, as Java and Rust): a hash is a credential for
					// the account, and a password change belongs to the explicit,
					// audited routes. The rest of the entry applies; the route's
					// response names the email (PasswordHashesIgnored). The stored
					// hash (hydrated by FindByEmail) is written back unchanged.
					if in.PasswordHash != nil && *in.PasswordHash != "" {
						slog.InfoContext(ctx, "principal sync: passwordHash ignored for an existing principal",
							"principalId", existing.ID)
					}
					saves = append(saves, usecasepgx.SyncSaveItem[principal.Principal]{
						Aggregate: existing,
						Event: UserUpdated{
							Metadata: usecase.NewEventMetadata(ec, UserUpdatedType, Source, subjectFor(existing.ID)),
							UserID:   existing.ID,
							Name:     existing.Name,
						},
					})
					updated++
					continue
				}

				p := principal.NewUser(email, principal.ScopeClient)
				p.Name = in.Name
				p.Active = in.Active
				p.Roles = roleAssignments
				changed = append(changed, roleNames(roleAssignments)...)
				// Carry a migrated credential verbatim (e.g. an upstream Laravel
				// bcrypt hash) so the user keeps their password; login verifies and
				// re-encodes it to the native scheme on first success.
				if in.PasswordHash != nil && *in.PasswordHash != "" {
					p.SetPasswordHash(*in.PasswordHash)
				}
				saves = append(saves, usecasepgx.SyncSaveItem[principal.Principal]{
					Aggregate: p,
					Event: UserCreated{
						Metadata: usecase.NewEventMetadata(ec, UserCreatedType, Source, subjectFor(p.ID)),
						UserID:   p.ID,
						Email:    email,
					},
				})
				created++
			}

			if cmd.RemoveUnlisted {
				// X-02(c): strip only SDK_SYNC roles belonging to THIS sync's
				// application (belongsToThisSync). The platform-level sweep (no
				// ApplicationCode) strips every SDK_SYNC role; that combination
				// is anchor-gated above.
				all, err := principals.FindAll(ctx)
				if err != nil {
					return nil, usecase.Internal("REPO", "find_all failed", err)
				}
				for i := range all {
					pr := &all[i]
					if !pr.IsUser() || pr.UserIdentity == nil {
						continue
					}
					if _, present := syncedSet[strings.ToLower(pr.UserIdentity.Email)]; present {
						continue
					}
					// Never past the caller's reach (F12).
					if !administers(ac, pr) {
						continue
					}
					hasSdkRoles := slices.ContainsFunc(pr.Roles, belongsToThisSync)
					if !hasSdkRoles {
						continue
					}
					kept := make([]serviceaccount.RoleAssignment, 0, len(pr.Roles))
					for _, ra := range pr.Roles {
						if belongsToThisSync(ra) {
							continue
						}
						kept = append(kept, ra)
					}
					changed = append(changed, role.ChangedRoles(roleNames(pr.Roles), roleNames(kept))...)
					pr.Roles = kept
					pr.UpdatedAt = now
					saves = append(saves, usecasepgx.SyncSaveItem[principal.Principal]{
						Aggregate: pr,
						Event: UserUpdated{
							Metadata: usecase.NewEventMetadata(ec, UserUpdatedType, Source, subjectFor(pr.ID)),
							UserID:   pr.ID,
							Name:     pr.Name,
						},
					})
					deactivated++
				}
			}

			if err := role.RequireRolesWithinCaller(ctx, ac, changed,
				role.PermissionsFrom(roles)); err != nil {
				return nil, err
			}

			subject := "platform.principals"
			if cmd.ApplicationCode != "" {
				subject += "." + cmd.ApplicationCode
			}
			rollup := PrincipalsSynced{
				Metadata:        usecase.NewEventMetadata(ec, PrincipalsSyncedType, Source, subject),
				ApplicationCode: cmd.ApplicationCode,
				Created:         created,
				Updated:         updated,
				Deactivated:     deactivated,
				SyncedEmails:    syncedEmails,
			}
			// RolesPersister rewrites iam_principal_roles for each synced user from
			// its merged Roles slice — the base principal Persist writes only the
			// iam_principals row, so without this the SDK's SDK_SYNC roles never land.
			return usecaseop.Sync(principal.RolesPersister{Repository: principals}, saves, nil, rollup), nil
		},
	}
}

// PasswordHashesIgnored returns the emails (lower-cased, trimmed, each once,
// in payload order) of the entries carrying a non-empty passwordHash for a
// principal that already exists: the sync will not apply those hashes (F13,
// owner ruling 2026-09-25; Java SyncPrincipals.passwordHashesIgnored, Rust
// password_hashes_ignored). The routes read it before the sync runs, for
// their passwordHashIgnored response field. A principal created concurrently
// in between is missed here, and its hash is still not applied.
func PasswordHashesIgnored(ctx context.Context, principals *principal.Repository, inputs []SyncPrincipalInput) ([]string, error) {
	var out []string
	for _, in := range inputs {
		if in.PasswordHash == nil || *in.PasswordHash == "" {
			continue
		}
		email := strings.ToLower(strings.TrimSpace(in.Email))
		if slices.Contains(out, email) {
			continue
		}
		p, err := principals.FindByEmail(ctx, email)
		if err != nil {
			return nil, usecase.Internal("REPO", "find_by_email failed", err)
		}
		if p != nil {
			out = append(out, email)
		}
	}
	return out, nil
}

// requireSyncableRoles is the sync's role rule (F12): on an application's
// route every name is that application's; on the platform route a non-anchor
// names only roles assignable to target (nil: a principal the sync creates,
// which has no client, so no role at all).
func requireSyncableRoles(ctx context.Context, roles *role.Repository, clientConfigs *application.ClientConfigRepo,
	ac *auth.AuthContext, applicationCode string, names []string, target *principal.Principal,
) error {
	if len(names) == 0 {
		return nil
	}
	if applicationCode != "" {
		return requireApplicationRoles(ctx, roles, applicationCode, names)
	}
	if ac.IsAnchor() {
		return nil
	}
	clientID := ""
	if target != nil && target.ClientID != nil {
		clientID = string(*target.ClientID)
	}
	allowed, err := ClientApplicationIDs(ctx, clientConfigs, clientID)
	if err != nil {
		return err
	}
	return RequireAssignableRoles(ctx, roles, names, allowed)
}
