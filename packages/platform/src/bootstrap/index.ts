/**
 * Bootstrap Module
 *
 * Runs at platform startup to sync code-defined permissions/roles
 * to the database and create an initial admin user if none exists.
 */

export { type BootstrapDeps, roleCodeToDbName } from "./bootstrap-service.js";
import {
	syncPlatformPermissions,
	syncPlatformRoles,
	bootstrapAdminUser,
	type BootstrapDeps,
} from "./bootstrap-service.js";

/**
 * Run all bootstrap steps.
 *
 * 1. Sync permissions to auth_permissions table
 * 2. Sync roles to auth_roles table
 * 3. Create bootstrap admin user if no ANCHOR users exist
 */
export async function runBootstrap(deps: BootstrapDeps): Promise<void> {
	deps.logger.info("Starting platform bootstrap...");

	await syncPlatformPermissions(deps);
	await syncPlatformRoles(deps);
	await bootstrapAdminUser(deps);

	deps.logger.info("Platform bootstrap complete");
}
