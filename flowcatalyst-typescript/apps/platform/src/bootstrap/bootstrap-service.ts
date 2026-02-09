/**
 * Bootstrap Service
 *
 * Syncs code-defined permissions and roles to the database at startup,
 * and creates an initial admin user if none exists.
 *
 * Matches the Java BootstrapService + RoleSyncService behavior.
 */

import { generate } from '@flowcatalyst/tsid';
import type { PasswordService } from '@flowcatalyst/platform-crypto';

import { ALL_PLATFORM_PERMISSIONS } from '../authorization/permissions/index.js';
import { ALL_PLATFORM_ROLES } from '../authorization/roles/index.js';
import { permissionToString, type PermissionDefinition } from '../authorization/permission-definition.js';
import type { RoleDefinition } from '../authorization/role-definition.js';
import type { RoleRepository, NewAuthRole } from '../infrastructure/persistence/repositories/role-repository.js';
import type { PermissionRepository, NewAuthPermission } from '../infrastructure/persistence/repositories/role-repository.js';
import type { PrincipalRepository } from '../infrastructure/persistence/repositories/principal-repository.js';
import type { ApplicationRepository } from '../infrastructure/persistence/repositories/application-repository.js';
import type { IdentityProviderRepository } from '../infrastructure/persistence/repositories/identity-provider-repository.js';
import type { EmailDomainMappingRepository } from '../infrastructure/persistence/repositories/email-domain-mapping-repository.js';
import { createApplication } from '../domain/application/application.js';
import { createIdentityProvider } from '../domain/identity-provider/identity-provider.js';
import { createEmailDomainMapping } from '../domain/email-domain-mapping/email-domain-mapping.js';
import { createRoleAssignment } from '../domain/principal/role-assignment.js';

export interface BootstrapDeps {
	roleRepository: RoleRepository;
	permissionRepository: PermissionRepository;
	principalRepository: PrincipalRepository;
	applicationRepository: ApplicationRepository;
	identityProviderRepository: IdentityProviderRepository;
	emailDomainMappingRepository: EmailDomainMappingRepository;
	passwordService: PasswordService;
	logger: { info: (obj: unknown, msg?: string) => void; warn: (obj: unknown, msg?: string) => void; debug: (obj: unknown, msg?: string) => void };
}

/**
 * Convert a RoleDefinition.code to the DB role name.
 *
 * PLATFORM_SUPER_ADMIN -> platform:super-admin
 * PLATFORM_IAM_ADMIN -> platform:iam-admin
 * PLATFORM_IAM_READONLY -> platform:iam-readonly
 */
export function roleCodeToDbName(code: string): string {
	// Strip known app prefix
	const withoutPrefix = code.replace(/^PLATFORM_/, '');
	return `platform:${withoutPrefix.toLowerCase().replace(/_/g, '-')}`;
}

/**
 * Format a role short name into a display name.
 * Converts "super-admin" to "Super Admin".
 */
function formatDisplayName(shortName: string): string {
	return shortName
		.split('-')
		.map((part) => part.charAt(0).toUpperCase() + part.slice(1))
		.join(' ');
}

/**
 * Sync all code-defined permissions to the auth_permissions table.
 */
export async function syncPlatformPermissions(deps: Pick<BootstrapDeps, 'permissionRepository' | 'logger'>): Promise<void> {
	const { permissionRepository, logger } = deps;

	let created = 0;
	let updated = 0;

	for (const permDef of ALL_PLATFORM_PERMISSIONS) {
		const code = permissionToString(permDef);
		const existing = await permissionRepository.findByCode(code);

		if (existing) {
			// Update description if changed
			if (existing.description !== permDef.description) {
				await permissionRepository.update({ ...existing, description: permDef.description });
				updated++;
			}
		} else {
			try {
				const newPerm: NewAuthPermission = {
					id: generate('PERMISSION'),
					code,
					subdomain: permDef.subdomain,
					context: permDef.context,
					aggregate: permDef.aggregate,
					action: permDef.action,
					description: permDef.description,
				};
				await permissionRepository.insert(newPerm);
				created++;
			} catch {
				// Permission already exists (concurrent startup or stale read) — treat as no-op
			}
		}
	}

	// Remove stale permissions no longer in code
	const codePermCodes = new Set(ALL_PLATFORM_PERMISSIONS.map(permissionToString));
	const allDbPerms = await permissionRepository.findBySubdomain('platform');
	let removed = 0;
	for (const dbPerm of allDbPerms) {
		if (!codePermCodes.has(dbPerm.code)) {
			await permissionRepository.deleteById(dbPerm.id);
			removed++;
		}
	}

	logger.info({ created, updated, removed }, 'Permission sync complete');
}

/**
 * Sync all code-defined roles to the auth_roles table.
 * Matches Java RoleSyncService.syncCodeDefinedRolesToDatabase().
 */
export async function syncPlatformRoles(
	deps: Pick<BootstrapDeps, 'roleRepository' | 'applicationRepository' | 'logger'>,
): Promise<void> {
	const { roleRepository, applicationRepository, logger } = deps;

	// Ensure the "platform" application exists
	let platformApp = await applicationRepository.findByCode('platform');
	if (!platformApp) {
		logger.info('Creating "platform" application for code-defined roles');
		const newApp = createApplication({ code: 'platform', name: 'Platform' });
		await applicationRepository.persist(newApp);
		platformApp = await applicationRepository.findByCode('platform');
		if (!platformApp) {
			logger.warn('Failed to create platform application');
			return;
		}
	}

	let created = 0;
	let updated = 0;

	for (const roleDef of ALL_PLATFORM_ROLES) {
		const roleName = roleCodeToDbName(roleDef.code);
		const shortName = roleName.substring(roleName.indexOf(':') + 1);
		const existing = await roleRepository.findByName(roleName);

		// Resolve permission strings (expand wildcards for storage)
		const permissionStrings = resolvePermissions(roleDef);

		if (existing) {
			if (existing.source === 'CODE') {
				// Update existing CODE role
				await roleRepository.update({
					...existing,
					displayName: formatDisplayName(shortName),
					description: roleDef.description,
					permissions: permissionStrings,
				});
				updated++;
			} else {
				logger.warn(
					{ roleName, source: existing.source },
					'Role exists with non-CODE source, not overwriting',
				);
			}
		} else {
			// Create new role
			const newRole: NewAuthRole = {
				id: generate('ROLE'),
				applicationId: platformApp.id,
				applicationCode: 'platform',
				name: roleName,
				displayName: formatDisplayName(shortName),
				description: roleDef.description,
				permissions: permissionStrings,
				source: 'CODE',
				clientManaged: false,
			};
			await roleRepository.insert(newRole);
			created++;
		}
	}

	// Remove stale CODE roles no longer in code definitions
	const codeRoleNames = new Set(ALL_PLATFORM_ROLES.map((r) => roleCodeToDbName(r.code)));
	const codeRolesInDb = await roleRepository.findCodeDefinedRoles();
	let removed = 0;

	for (const dbRole of codeRolesInDb) {
		if (!codeRoleNames.has(dbRole.name)) {
			logger.info({ roleName: dbRole.name }, 'Removing stale CODE role');
			await roleRepository.deleteById(dbRole.id);
			removed++;
		}
	}

	logger.info({ created, updated, removed }, 'Role sync complete');
}

/**
 * Resolve a role definition's permissions to concrete permission strings.
 * Wildcards like "platform:*:*:*" are expanded to all matching permissions.
 */
function resolvePermissions(roleDef: RoleDefinition): string[] {
	const result: string[] = [];
	for (const pattern of roleDef.permissions) {
		if (pattern.includes('*')) {
			// Expand wildcard against all known permissions
			for (const permDef of ALL_PLATFORM_PERMISSIONS) {
				const permStr = permissionToString(permDef);
				if (matchesPattern(permStr, pattern)) {
					result.push(permStr);
				}
			}
		} else {
			result.push(pattern);
		}
	}
	// Deduplicate
	return [...new Set(result)];
}

/**
 * Check if a permission string matches a pattern with wildcards.
 */
function matchesPattern(permission: string, pattern: string): boolean {
	const permParts = permission.split(':');
	const patternParts = pattern.split(':');

	if (permParts.length !== 4 || patternParts.length !== 4) {
		return false;
	}

	for (let i = 0; i < 4; i++) {
		if (patternParts[i] !== '*' && patternParts[i] !== permParts[i]) {
			return false;
		}
	}

	return true;
}

/**
 * Bootstrap an initial admin user if no ANCHOR users exist.
 * Matches Java BootstrapService behavior.
 */
export async function bootstrapAdminUser(deps: BootstrapDeps): Promise<void> {
	const {
		principalRepository,
		identityProviderRepository,
		emailDomainMappingRepository,
		passwordService,
		logger,
	} = deps;

	// Check if any ANCHOR scope users exist
	const allPrincipals = await principalRepository.findByType('USER');
	const hasAnchorUser = allPrincipals.some((p) => p.scope === 'ANCHOR');

	if (hasAnchorUser) {
		logger.debug('Anchor users already exist, skipping bootstrap');
		return;
	}

	logger.info('No anchor users found, checking for bootstrap configuration...');

	// Read bootstrap env vars
	const bootstrapEmail = process.env['FLOWCATALYST_BOOTSTRAP_ADMIN_EMAIL'];
	const bootstrapPassword = process.env['FLOWCATALYST_BOOTSTRAP_ADMIN_PASSWORD'];
	const bootstrapName = process.env['FLOWCATALYST_BOOTSTRAP_ADMIN_NAME'] ?? 'Bootstrap Admin';

	if (!bootstrapEmail || !bootstrapPassword) {
		logger.warn(
			'No bootstrap admin configured. Set FLOWCATALYST_BOOTSTRAP_ADMIN_EMAIL and FLOWCATALYST_BOOTSTRAP_ADMIN_PASSWORD to create an initial admin.',
		);
		return;
	}

	// Validate email format
	if (!bootstrapEmail.includes('@')) {
		logger.warn({ email: bootstrapEmail }, 'Invalid bootstrap email format');
		return;
	}

	// Check if user already exists (idempotency)
	const existingUser = await principalRepository.findByEmail(bootstrapEmail);
	if (existingUser) {
		logger.info({ email: bootstrapEmail }, 'Bootstrap user already exists');
		return;
	}

	const emailDomain = bootstrapEmail.substring(bootstrapEmail.indexOf('@') + 1);

	// Ensure internal identity provider exists
	const internalIdpId = await ensureInternalIdentityProvider(identityProviderRepository, logger);

	// Create anchor domain mapping if it doesn't exist
	const existingMapping = await emailDomainMappingRepository.findByEmailDomain(emailDomain);
	if (!existingMapping) {
		const mapping = createEmailDomainMapping({
			emailDomain,
			identityProviderId: internalIdpId,
			scopeType: 'ANCHOR',
		});
		await emailDomainMappingRepository.insert(mapping);
		logger.info({ emailDomain }, 'Created anchor domain mapping');
	}

	// Hash password (try validate first, fall back to plain hash)
	const hashResult = await passwordService.validateAndHash(bootstrapPassword);
	let passwordHash: string;

	if (hashResult.isOk()) {
		passwordHash = hashResult.value;
	} else {
		logger.warn('Bootstrap password does not meet complexity requirements, hashing anyway');
		const fallbackResult = await passwordService.hash(bootstrapPassword);
		if (fallbackResult.isErr()) {
			logger.warn('Failed to hash bootstrap password');
			return;
		}
		passwordHash = fallbackResult.value;
	}

	// Create the bootstrap admin user
	const principalId = generate('PRINCIPAL');
	const newPrincipal = {
		id: principalId,
		type: 'USER' as const,
		scope: 'ANCHOR' as const,
		clientId: null,
		applicationId: null,
		name: bootstrapName,
		active: true,
		userIdentity: {
			email: bootstrapEmail.toLowerCase(),
			emailDomain,
			idpType: 'INTERNAL' as const,
			externalIdpId: null,
			passwordHash,
			lastLoginAt: null,
		},
		serviceAccount: null,
		roles: [createRoleAssignment('platform:super-admin', 'BOOTSTRAP')],
		accessibleApplicationIds: [] as string[],
	};

	await principalRepository.insert(newPrincipal);

	logger.info(
		{ name: bootstrapName, email: bootstrapEmail },
		'Created bootstrap admin with platform:super-admin role and ANCHOR scope',
	);
}

/**
 * Ensure an internal identity provider exists for password-based auth.
 */
async function ensureInternalIdentityProvider(
	identityProviderRepository: IdentityProviderRepository,
	logger: BootstrapDeps['logger'],
): Promise<string> {
	const existing = await identityProviderRepository.findByCode('internal');
	if (existing) {
		return existing.id;
	}

	const idp = createIdentityProvider({
		code: 'internal',
		name: 'Internal Authentication',
		type: 'INTERNAL',
	});

	await identityProviderRepository.insert(idp);
	logger.info('Created internal identity provider');
	return idp.id;
}
