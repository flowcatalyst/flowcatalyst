/**
 * Who the SPA offers an action the BFF would refuse. Each check is the one the
 * endpoint makes (docs/platform-bug-fixes.md): anchor scope and the permission
 * for the operation. The pages showed Sync Platform Events and the role
 * editors to everyone, and most got a 403.
 */
import { userHasPermission, userScope } from "@/stores/permissions";

type SessionUser =
	| { permissions?: string[]; clientId?: string | null }
	| null
	| undefined;

export const EVENT_TYPE_SYNC = "platform:messaging:event-type:sync";
export const EVENT_TYPE_MANAGE = "platform:messaging:event-type:manage";
export const ROLE_CREATE = "platform:iam:role:create";
export const ROLE_UPDATE = "platform:iam:role:update";
export const ROLE_DELETE = "platform:iam:role:delete";
export const USER_ASSIGN_ROLES = "platform:iam:user:assign-roles";

function isAnchor(user: SessionUser): boolean {
	return !!user && userScope(user) === "anchor";
}

/** POST /bff/event-types/sync-platform: anchor and event-type:sync or :manage. */
export function canSyncPlatformEventTypes(user: SessionUser): boolean {
	return (
		isAnchor(user) &&
		(userHasPermission(user, EVENT_TYPE_SYNC) ||
			userHasPermission(user, EVENT_TYPE_MANAGE))
	);
}

/** POST /bff/roles and POST /bff/roles/permissions: anchor and role:create. */
export function canCreateRoles(user: SessionUser): boolean {
	return isAnchor(user) && userHasPermission(user, ROLE_CREATE);
}

/** PUT /bff/roles/{name}: anchor and role:update. */
export function canUpdateRoles(user: SessionUser): boolean {
	return isAnchor(user) && userHasPermission(user, ROLE_UPDATE);
}

/** DELETE /bff/roles/{name}: anchor and role:delete. */
export function canDeleteRoles(user: SessionUser): boolean {
	return isAnchor(user) && userHasPermission(user, ROLE_DELETE);
}

/**
 * POST /bff/roles/sync-platform creates, updates and removes platform roles:
 * anchor and all three role permissions.
 */
export function canSyncPlatformRoles(user: SessionUser): boolean {
	return canCreateRoles(user) && canUpdateRoles(user) && canDeleteRoles(user);
}

/**
 * PUT and POST /api/principals/{id}/roles, DELETE .../roles/{role}, and the
 * roles of a CSV import row: user:assign-roles, not user:update (owner ruling
 * 2026-10-11). No anchor check: a client administrator holding it assigns its
 * client's application roles, and the API bounds which.
 */
export function canAssignUserRoles(user: SessionUser): boolean {
	return userHasPermission(user, USER_ASSIGN_ROLES);
}

export const SERVICE_ACCOUNT_CREATE = "platform:iam:service-account:create";
export const APPLICATION_UPDATE = "platform:admin:application:update";
const SUPER_ADMIN = "platform:*:*:*";

/**
 * POST /api/applications/{id}/provision-service-account: anchor scope,
 * service-account:create and application:update (docs/platform-bug-fixes.md
 * F8). The role ceiling is the API's to apply on submit.
 */
export function canProvisionApplicationServiceAccount(
	user: SessionUser,
): boolean {
	return (
		isAnchor(user) &&
		userHasPermission(user, SERVICE_ACCOUNT_CREATE) &&
		userHasPermission(user, APPLICATION_UPDATE)
	);
}

/**
 * The role ceiling (docs/platform-bug-fixes.md F8): a role is above the user
 * when it holds a platform permission the user does not (wildcards honoured;
 * an application's own permissions are outside the ceiling). The API refuses
 * adding or removing such a role (403 ROLE_ABOVE_CALLER), for users and for
 * service accounts alike, so the role pickers lock it.
 */
export function isRoleAboveUser(
	user: SessionUser,
	role: { permissions?: string[] } | null | undefined,
): boolean {
	if (!role || userHasPermission(user, SUPER_ADMIN)) return false;
	return (role.permissions ?? []).some(
		(p) => p.startsWith("platform:") && !userHasPermission(user, p),
	);
}

/** The pickers' explanation for a locked role. */
export const ROLE_ABOVE_USER_HINT =
	"This role holds permissions you do not have, so you cannot add or remove it.";
