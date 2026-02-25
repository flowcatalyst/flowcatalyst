/**
 * Update Email Domain Mapping Use Case
 *
 * When the scopeType or primaryClientId changes, cascades scope changes
 * to all existing USER principals with the matching email domain.
 */

import type { UseCase } from "@flowcatalyst/application";
import { Result, UseCaseError } from "@flowcatalyst/application";
import type { ExecutionContext, UnitOfWork } from "@flowcatalyst/domain-core";

import type {
	IdentityProviderRepository,
	EmailDomainMappingRepository,
	PrincipalRepository,
	ClientAccessGrantRepository,
} from "../../../infrastructure/persistence/index.js";
import {
	updateEmailDomainMapping,
	EmailDomainMappingUpdated,
	updatePrincipal,
	PrincipalScope,
} from "../../../domain/index.js";

import type { UpdateEmailDomainMappingCommand } from "./command.js";

export interface UpdateEmailDomainMappingUseCaseDeps {
	readonly emailDomainMappingRepository: EmailDomainMappingRepository;
	readonly identityProviderRepository: IdentityProviderRepository;
	readonly principalRepository: PrincipalRepository;
	readonly clientAccessGrantRepository: ClientAccessGrantRepository;
	readonly unitOfWork: UnitOfWork;
}

export function createUpdateEmailDomainMappingUseCase(
	deps: UpdateEmailDomainMappingUseCaseDeps,
): UseCase<UpdateEmailDomainMappingCommand, EmailDomainMappingUpdated> {
	const {
		emailDomainMappingRepository,
		identityProviderRepository,
		principalRepository,
		clientAccessGrantRepository,
		unitOfWork,
	} = deps;

	return {
		async execute(
			command: UpdateEmailDomainMappingCommand,
			context: ExecutionContext,
		): Promise<Result<EmailDomainMappingUpdated>> {
			const mapping = await emailDomainMappingRepository.findById(
				command.emailDomainMappingId,
			);
			if (!mapping) {
				return Result.failure(
					UseCaseError.notFound(
						"MAPPING_NOT_FOUND",
						"Email domain mapping not found",
						{
							emailDomainMappingId: command.emailDomainMappingId,
						},
					),
				);
			}

			// Resolve the effective IDP (new one if changing, existing one otherwise)
			let idp;
			if (command.identityProviderId !== undefined) {
				idp = await identityProviderRepository.findById(
					command.identityProviderId,
				);
				if (!idp) {
					return Result.failure(
						UseCaseError.notFound(
							"IDP_NOT_FOUND",
							"Identity provider not found",
							{
								identityProviderId: command.identityProviderId,
							},
						),
					);
				}
			} else {
				idp = await identityProviderRepository.findById(
					mapping.identityProviderId,
				);
			}

			// Multi-tenant IDPs require a tenant ID
			if (idp?.oidcMultiTenant) {
				const effectiveTenantId =
					command.requiredOidcTenantId !== undefined
						? command.requiredOidcTenantId
						: mapping.requiredOidcTenantId;
				if (!effectiveTenantId?.trim()) {
					return Result.failure(
						UseCaseError.validation(
							"OIDC_TENANT_ID_REQUIRED",
							"Required OIDC Tenant ID must be set for multi-tenant identity providers",
							{ field: "requiredOidcTenantId" },
						),
					);
				}
			}

			// Track previous scope for cascade decision
			const previousScope = mapping.scopeType;
			const previousClientId = mapping.primaryClientId;

			const updated = updateEmailDomainMapping(mapping, {
				...(command.identityProviderId !== undefined
					? { identityProviderId: command.identityProviderId }
					: {}),
				...(command.scopeType !== undefined
					? { scopeType: command.scopeType }
					: {}),
				...(command.primaryClientId !== undefined
					? { primaryClientId: command.primaryClientId }
					: {}),
				...(command.additionalClientIds !== undefined
					? { additionalClientIds: command.additionalClientIds }
					: {}),
				...(command.grantedClientIds !== undefined
					? { grantedClientIds: command.grantedClientIds }
					: {}),
				...(command.requiredOidcTenantId !== undefined
					? { requiredOidcTenantId: command.requiredOidcTenantId }
					: {}),
				...(command.allowedRoleIds !== undefined
					? { allowedRoleIds: command.allowedRoleIds }
					: {}),
				...(command.syncRolesFromIdp !== undefined
					? { syncRolesFromIdp: command.syncRolesFromIdp }
					: {}),
			});

			const event = new EmailDomainMappingUpdated(context, {
				emailDomainMappingId: updated.id,
				emailDomain: updated.emailDomain,
				identityProviderId: updated.identityProviderId,
				scopeType: updated.scopeType,
				primaryClientId: updated.primaryClientId,
				additionalClientIds: updated.additionalClientIds,
				grantedClientIds: updated.grantedClientIds,
			});

			const result = await unitOfWork.commit(updated, event, command);

			// Cascade scope changes if scopeType or primaryClientId changed
			const scopeChanged = updated.scopeType !== previousScope;
			const clientIdChanged = updated.primaryClientId !== previousClientId;

			if (Result.isSuccess(result) && (scopeChanged || clientIdChanged)) {
				const newScope = updated.scopeType as PrincipalScope;
				const newClientId =
					newScope === "CLIENT" ? (updated.primaryClientId ?? null) : null;

				await cascadeScopeToUsers({
					emailDomain: updated.emailDomain,
					previousScope: previousScope as PrincipalScope,
					newScope,
					newClientId,
					principalRepository,
					clientAccessGrantRepository,
				});
			}

			return result;
		},
	};
}

/**
 * Cascade scope changes to all existing USER principals with the given email domain.
 * Updates scope/clientId and cleans up client access grants when downgrading to CLIENT.
 */
async function cascadeScopeToUsers(params: {
	emailDomain: string;
	previousScope: PrincipalScope;
	newScope: PrincipalScope;
	newClientId: string | null;
	principalRepository: PrincipalRepository;
	clientAccessGrantRepository: ClientAccessGrantRepository;
}): Promise<void> {
	const {
		emailDomain,
		previousScope,
		newScope,
		newClientId,
		principalRepository,
		clientAccessGrantRepository,
	} = params;

	const users = await principalRepository.findUsersByEmailDomain(emailDomain);

	for (const user of users) {
		if (user.scope === newScope && user.clientId === newClientId) {
			continue; // No change needed
		}

		const updated = updatePrincipal(user, {
			scope: newScope,
			clientId: newClientId,
		});
		await principalRepository.update(updated);

		// If downgrading from PARTNER to CLIENT, remove all client access grants
		if (
			newScope === PrincipalScope.CLIENT &&
			previousScope === PrincipalScope.PARTNER
		) {
			await clientAccessGrantRepository.deleteByPrincipalId(user.id);
		}
	}
}
