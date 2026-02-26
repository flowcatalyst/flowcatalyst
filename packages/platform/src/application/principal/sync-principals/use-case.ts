/**
 * Sync Principals Use Case
 *
 * Syncs user principals from an application SDK. For each principal:
 * - Find by email: if exists, update name and sync roles
 * - If not found: create new user and assign roles
 *
 * Role names are short names (e.g. "transport-planner") that get prefixed
 * with the application code (e.g. "logistics_portal:transport-planner").
 *
 * If removeUnlisted is true, SDK-synced principals for this application
 * that are not in the current sync list will be deactivated (not deleted).
 */

import type { UseCase } from "@flowcatalyst/application";
import {
	validateRequired,
	Result,
	UseCaseError,
} from "@flowcatalyst/application";
import type { ExecutionContext, UnitOfWork } from "@flowcatalyst/domain";
import type { TransactionContext } from "@flowcatalyst/persistence";

import type {
	PrincipalRepository,
	ApplicationRepository,
	RoleRepository,
	AnchorDomainRepository,
	EmailDomainMappingRepository,
	IdentityProviderRepository,
} from "../../../infrastructure/persistence/index.js";
import {
	createUserPrincipal,
	createUserIdentity,
	createRoleAssignment,
	extractEmailDomain,
	updatePrincipal,
	assignRoles,
	PrincipalsSynced,
	IdpType,
	PrincipalScope,
} from "../../../domain/index.js";

import type { SyncPrincipalsCommand } from "./command.js";

export interface SyncPrincipalsUseCaseDeps {
	readonly principalRepository: PrincipalRepository;
	readonly applicationRepository: ApplicationRepository;
	readonly roleRepository: RoleRepository;
	readonly anchorDomainRepository: AnchorDomainRepository;
	readonly emailDomainMappingRepository: EmailDomainMappingRepository;
	readonly identityProviderRepository: IdentityProviderRepository;
	readonly unitOfWork: UnitOfWork;
}

export function createSyncPrincipalsUseCase(
	deps: SyncPrincipalsUseCaseDeps,
): UseCase<SyncPrincipalsCommand, PrincipalsSynced> {
	const {
		principalRepository,
		applicationRepository,
		roleRepository: _roleRepository,
		anchorDomainRepository,
		emailDomainMappingRepository,
		identityProviderRepository,
		unitOfWork,
	} = deps;

	return {
		async execute(
			command: SyncPrincipalsCommand,
			context: ExecutionContext,
		): Promise<Result<PrincipalsSynced>> {
			const appCodeResult = validateRequired(
				command.applicationCode,
				"applicationCode",
				"APPLICATION_CODE_REQUIRED",
			);
			if (Result.isFailure(appCodeResult)) return appCodeResult;

			if (!command.principals || command.principals.length === 0) {
				return Result.failure(
					UseCaseError.validation(
						"PRINCIPALS_REQUIRED",
						"At least one principal must be provided",
					),
				);
			}

			// Look up the application to get its ID
			const application = await applicationRepository.findByCode(
				command.applicationCode,
			);
			if (!application) {
				return Result.failure(
					UseCaseError.notFound(
						"APPLICATION_NOT_FOUND",
						`Application not found: ${command.applicationCode}`,
					),
				);
			}

			let created = 0;
			let updated = 0;
			let deactivated = 0;
			const syncedEmails: string[] = [];

			const eventData = {
				applicationCode: command.applicationCode,
				principalsCreated: 0,
				principalsUpdated: 0,
				principalsDeactivated: 0,
				syncedEmails: [] as string[],
			};

			const event = new PrincipalsSynced(context, eventData);

			return unitOfWork.commitOperations(event, command, async (tx) => {
				const txCtx = tx as TransactionContext;

				for (const item of command.principals) {
					const email = item.email.toLowerCase();
					syncedEmails.push(email);

					// Role names are used as-is (they already contain the correct prefix from the source app)
					const roleNames = (item.roles ?? []).map((r) => r.toLowerCase());

					// Build role assignments
					const roleAssignments = roleNames.map((roleName) =>
						createRoleAssignment(roleName, "SDK_SYNC"),
					);

					const existing = await principalRepository.findByEmail(email, txCtx);

					if (existing) {
						// Update existing principal: update name, active status, and sync roles
						// Merge existing non-SDK roles with new SDK-synced roles
						const existingNonSdkRoles = existing.roles.filter(
							(r) => r.assignmentSource !== "SDK_SYNC",
						);
						const mergedRoles = [...existingNonSdkRoles, ...roleAssignments];

						const updatedPrincipal = updatePrincipal(
							assignRoles(existing, mergedRoles),
							{
								name: item.name,
								active: item.active !== false,
							},
						);

						await principalRepository.update(updatedPrincipal, txCtx);
						updated++;
					} else {
						// Create new principal
						const emailDomain = extractEmailDomain(email);

						// Determine scope and IDP type from email domain mapping
						let scope: PrincipalScope = PrincipalScope.CLIENT;
						let idpType: IdpType = IdpType.INTERNAL;

						const mapping =
							await emailDomainMappingRepository.findByEmailDomain(emailDomain);
						if (mapping) {
							scope = mapping.scopeType as PrincipalScope;
							const idp = await identityProviderRepository.findById(
								mapping.identityProviderId,
								txCtx,
							);
							if (idp && idp.type === "OIDC") {
								idpType = IdpType.OIDC;
							}
						} else {
							// Fall back to legacy anchor domain check
							const isAnchorDomain =
								await anchorDomainRepository.existsByDomain(emailDomain);
							if (isAnchorDomain) {
								scope = PrincipalScope.ANCHOR;
							}
						}

						const userIdentity = createUserIdentity({
							email,
							idpType,
						});

						const newPrincipal = createUserPrincipal({
							name: item.name,
							scope,
							clientId: null,
							userIdentity,
						});

						// Assign roles to the new principal
						const principalWithRoles = {
							...newPrincipal,
							roles: roleAssignments,
							active: item.active !== false,
						};

						await principalRepository.insert(principalWithRoles, txCtx);
						created++;
					}
				}

				// Remove SDK_SYNC roles from unlisted principals
				if (command.removeUnlisted) {
					const allPrincipals = await principalRepository.findAll(txCtx);

					for (const principal of allPrincipals) {
						if (principal.type !== "USER") continue;
						if (!principal.userIdentity?.email) continue;
						if (syncedEmails.includes(principal.userIdentity.email)) continue;

						const hasSdkRoles = principal.roles.some(
							(r) => r.assignmentSource === "SDK_SYNC",
						);

						if (hasSdkRoles) {
							const remainingRoles = principal.roles.filter(
								(r) => r.assignmentSource !== "SDK_SYNC",
							);

							const updatedPrincipal = assignRoles(principal, remainingRoles);
							await principalRepository.update(updatedPrincipal, txCtx);
							deactivated++;
						}
					}
				}

				// Update event data with final counts
				eventData.principalsCreated = created;
				eventData.principalsUpdated = updated;
				eventData.principalsDeactivated = deactivated;
				eventData.syncedEmails = syncedEmails;
			});
		},
	};
}
