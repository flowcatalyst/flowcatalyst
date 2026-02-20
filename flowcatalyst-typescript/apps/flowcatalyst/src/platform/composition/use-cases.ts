/**
 * Use case creation — instantiates all platform use cases with guards.
 */

import type { UnitOfWork } from "@flowcatalyst/domain-core";
import type { PasswordService } from "@flowcatalyst/platform-crypto";
import type { EncryptionService } from "@flowcatalyst/platform-crypto";
import {
	createGuardedUseCase,
	clientScopedGuard,
	clientAccessGuard,
} from "../authorization/index.js";
import type { Repositories } from "./repositories.js";
import {
	createCreateUserUseCase,
	createUpdateUserUseCase,
	createActivateUserUseCase,
	createDeactivateUserUseCase,
	createDeleteUserUseCase,
	createCreateClientUseCase,
	createUpdateClientUseCase,
	createChangeClientStatusUseCase,
	createDeleteClientUseCase,
	createAddClientNoteUseCase,
	createCreateAnchorDomainUseCase,
	createUpdateAnchorDomainUseCase,
	createDeleteAnchorDomainUseCase,
	createCreateApplicationUseCase,
	createUpdateApplicationUseCase,
	createDeleteApplicationUseCase,
	createActivateApplicationUseCase,
	createDeactivateApplicationUseCase,
	createEnableApplicationForClientUseCase,
	createDisableApplicationForClientUseCase,
	createCreateRoleUseCase,
	createUpdateRoleUseCase,
	createDeleteRoleUseCase,
	createAssignRolesUseCase,
	createGrantClientAccessUseCase,
	createRevokeClientAccessUseCase,
	createCreateInternalAuthConfigUseCase,
	createCreateOidcAuthConfigUseCase,
	createUpdateOidcSettingsUseCase,
	createUpdateConfigTypeUseCase,
	createUpdateAdditionalClientsUseCase,
	createUpdateGrantedClientsUseCase,
	createDeleteAuthConfigUseCase,
	createCreateOAuthClientUseCase,
	createUpdateOAuthClientUseCase,
	createRegenerateOAuthClientSecretUseCase,
	createDeleteOAuthClientUseCase,
	createCreateEventTypeUseCase,
	createUpdateEventTypeUseCase,
	createArchiveEventTypeUseCase,
	createDeleteEventTypeUseCase,
	createAddSchemaUseCase,
	createFinaliseSchemaUseCase,
	createDeprecateSchemaUseCase,
	createSyncEventTypesUseCase,
	createCreateDispatchPoolUseCase,
	createUpdateDispatchPoolUseCase,
	createDeleteDispatchPoolUseCase,
	createSyncDispatchPoolsUseCase,
	createCreateSubscriptionUseCase,
	createUpdateSubscriptionUseCase,
	createDeleteSubscriptionUseCase,
	createSyncSubscriptionsUseCase,
	createCreateIdentityProviderUseCase,
	createUpdateIdentityProviderUseCase,
	createDeleteIdentityProviderUseCase,
	createCreateEmailDomainMappingUseCase,
	createUpdateEmailDomainMappingUseCase,
	createDeleteEmailDomainMappingUseCase,
	createCreateServiceAccountUseCase,
	createUpdateServiceAccountUseCase,
	createDeleteServiceAccountUseCase,
	createRegenerateAuthTokenUseCase,
	createRegenerateSigningSecretUseCase,
	createAssignServiceAccountRolesUseCase,
	createAssignApplicationAccessUseCase,
	createAddCorsOriginUseCase,
	createDeleteCorsOriginUseCase,
	createSyncRolesUseCase,
	createSyncPrincipalsUseCase,
} from "../application/index.js";

export interface CreateUseCasesDeps {
	repos: Repositories;
	unitOfWork: UnitOfWork;
	passwordService: PasswordService;
	encryptionService: EncryptionService;
}

export function createUseCases(deps: CreateUseCasesDeps) {
	const { repos, unitOfWork, passwordService, encryptionService } = deps;

	// --- Principal use cases ---
	const createUserUseCase = createCreateUserUseCase({
		principalRepository: repos.principalRepository,
		anchorDomainRepository: repos.anchorDomainRepository,
		emailDomainMappingRepository: repos.emailDomainMappingRepository,
		identityProviderRepository: repos.identityProviderRepository,
		passwordService,
		unitOfWork,
	});

	const updateUserUseCase = createUpdateUserUseCase({
		principalRepository: repos.principalRepository,
		unitOfWork,
	});

	const activateUserUseCase = createActivateUserUseCase({
		principalRepository: repos.principalRepository,
		unitOfWork,
	});

	const deactivateUserUseCase = createDeactivateUserUseCase({
		principalRepository: repos.principalRepository,
		unitOfWork,
	});

	const deleteUserUseCase = createDeleteUserUseCase({
		principalRepository: repos.principalRepository,
		unitOfWork,
	});

	// --- Client use cases (with resource-level guards) ---
	const createClientUseCase = createCreateClientUseCase({
		clientRepository: repos.clientRepository,
		unitOfWork,
	});

	const updateClientUseCase = createGuardedUseCase(
		createUpdateClientUseCase({
			clientRepository: repos.clientRepository,
			unitOfWork,
		}),
		clientAccessGuard((cmd) => cmd.clientId),
	);

	const changeClientStatusUseCase = createGuardedUseCase(
		createChangeClientStatusUseCase({
			clientRepository: repos.clientRepository,
			unitOfWork,
		}),
		clientAccessGuard((cmd) => cmd.clientId),
	);

	const deleteClientUseCase = createGuardedUseCase(
		createDeleteClientUseCase({
			clientRepository: repos.clientRepository,
			unitOfWork,
		}),
		clientAccessGuard((cmd) => cmd.clientId),
	);

	const addClientNoteUseCase = createGuardedUseCase(
		createAddClientNoteUseCase({
			clientRepository: repos.clientRepository,
			unitOfWork,
		}),
		clientAccessGuard((cmd) => cmd.clientId),
	);

	// --- Anchor domain use cases ---
	const createAnchorDomainUseCase = createCreateAnchorDomainUseCase({
		anchorDomainRepository: repos.anchorDomainRepository,
		unitOfWork,
	});

	const updateAnchorDomainUseCase = createUpdateAnchorDomainUseCase({
		anchorDomainRepository: repos.anchorDomainRepository,
		unitOfWork,
	});

	const deleteAnchorDomainUseCase = createDeleteAnchorDomainUseCase({
		anchorDomainRepository: repos.anchorDomainRepository,
		unitOfWork,
	});

	// --- Application use cases ---
	const createApplicationUseCase = createCreateApplicationUseCase({
		applicationRepository: repos.applicationRepository,
		unitOfWork,
	});

	const updateApplicationUseCase = createUpdateApplicationUseCase({
		applicationRepository: repos.applicationRepository,
		unitOfWork,
	});

	const deleteApplicationUseCase = createDeleteApplicationUseCase({
		applicationRepository: repos.applicationRepository,
		unitOfWork,
	});

	const enableApplicationForClientUseCase = createGuardedUseCase(
		createEnableApplicationForClientUseCase({
			applicationRepository: repos.applicationRepository,
			clientRepository: repos.clientRepository,
			applicationClientConfigRepository:
				repos.applicationClientConfigRepository,
			unitOfWork,
		}),
		clientAccessGuard((cmd) => cmd.clientId),
	);

	const disableApplicationForClientUseCase = createGuardedUseCase(
		createDisableApplicationForClientUseCase({
			applicationClientConfigRepository:
				repos.applicationClientConfigRepository,
			unitOfWork,
		}),
		clientAccessGuard((cmd) => cmd.clientId),
	);

	const activateApplicationUseCase = createActivateApplicationUseCase({
		applicationRepository: repos.applicationRepository,
		unitOfWork,
	});

	const deactivateApplicationUseCase = createDeactivateApplicationUseCase({
		applicationRepository: repos.applicationRepository,
		unitOfWork,
	});

	// --- Role use cases ---
	const createRoleUseCase = createCreateRoleUseCase({
		roleRepository: repos.roleRepository,
		unitOfWork,
	});

	const updateRoleUseCase = createUpdateRoleUseCase({
		roleRepository: repos.roleRepository,
		unitOfWork,
	});

	const deleteRoleUseCase = createDeleteRoleUseCase({
		roleRepository: repos.roleRepository,
		unitOfWork,
	});

	const syncRolesUseCase = createSyncRolesUseCase({
		roleRepository: repos.roleRepository,
		applicationRepository: repos.applicationRepository,
		unitOfWork,
	});

	const syncPrincipalsUseCase = createSyncPrincipalsUseCase({
		principalRepository: repos.principalRepository,
		applicationRepository: repos.applicationRepository,
		roleRepository: repos.roleRepository,
		anchorDomainRepository: repos.anchorDomainRepository,
		emailDomainMappingRepository: repos.emailDomainMappingRepository,
		identityProviderRepository: repos.identityProviderRepository,
		unitOfWork,
	});

	// --- User role and client access use cases ---
	const assignRolesUseCase = createAssignRolesUseCase({
		principalRepository: repos.principalRepository,
		roleRepository: repos.roleRepository,
		unitOfWork,
	});

	const grantClientAccessUseCase = createGuardedUseCase(
		createGrantClientAccessUseCase({
			principalRepository: repos.principalRepository,
			clientRepository: repos.clientRepository,
			clientAccessGrantRepository: repos.clientAccessGrantRepository,
			unitOfWork,
		}),
		clientAccessGuard((cmd) => cmd.clientId),
	);

	const revokeClientAccessUseCase = createGuardedUseCase(
		createRevokeClientAccessUseCase({
			principalRepository: repos.principalRepository,
			clientAccessGrantRepository: repos.clientAccessGrantRepository,
			unitOfWork,
		}),
		clientAccessGuard((cmd) => cmd.clientId),
	);

	// --- Auth config use cases ---
	const createInternalAuthConfigUseCase = createCreateInternalAuthConfigUseCase(
		{
			clientAuthConfigRepository: repos.clientAuthConfigRepository,
			unitOfWork,
		},
	);

	const createOidcAuthConfigUseCase = createCreateOidcAuthConfigUseCase({
		clientAuthConfigRepository: repos.clientAuthConfigRepository,
		unitOfWork,
	});

	const updateOidcSettingsUseCase = createUpdateOidcSettingsUseCase({
		clientAuthConfigRepository: repos.clientAuthConfigRepository,
		unitOfWork,
	});

	const updateConfigTypeUseCase = createUpdateConfigTypeUseCase({
		clientAuthConfigRepository: repos.clientAuthConfigRepository,
		unitOfWork,
	});

	const updateAdditionalClientsUseCase = createUpdateAdditionalClientsUseCase({
		clientAuthConfigRepository: repos.clientAuthConfigRepository,
		unitOfWork,
	});

	const updateGrantedClientsUseCase = createUpdateGrantedClientsUseCase({
		clientAuthConfigRepository: repos.clientAuthConfigRepository,
		unitOfWork,
	});

	const deleteAuthConfigUseCase = createDeleteAuthConfigUseCase({
		clientAuthConfigRepository: repos.clientAuthConfigRepository,
		unitOfWork,
	});

	// --- OAuth client use cases ---
	const createOAuthClientUseCase = createCreateOAuthClientUseCase({
		oauthClientRepository: repos.oauthClientRepository,
		unitOfWork,
	});

	const updateOAuthClientUseCase = createUpdateOAuthClientUseCase({
		oauthClientRepository: repos.oauthClientRepository,
		unitOfWork,
	});

	const regenerateOAuthClientSecretUseCase =
		createRegenerateOAuthClientSecretUseCase({
			oauthClientRepository: repos.oauthClientRepository,
			unitOfWork,
		});

	const deleteOAuthClientUseCase = createDeleteOAuthClientUseCase({
		oauthClientRepository: repos.oauthClientRepository,
		unitOfWork,
	});

	// --- EventType use cases ---
	const createEventTypeUseCase = createCreateEventTypeUseCase({
		eventTypeRepository: repos.eventTypeRepository,
		unitOfWork,
	});

	const updateEventTypeUseCase = createUpdateEventTypeUseCase({
		eventTypeRepository: repos.eventTypeRepository,
		unitOfWork,
	});

	const archiveEventTypeUseCase = createArchiveEventTypeUseCase({
		eventTypeRepository: repos.eventTypeRepository,
		unitOfWork,
	});

	const deleteEventTypeUseCase = createDeleteEventTypeUseCase({
		eventTypeRepository: repos.eventTypeRepository,
		unitOfWork,
	});

	const addSchemaUseCase = createAddSchemaUseCase({
		eventTypeRepository: repos.eventTypeRepository,
		unitOfWork,
	});

	const finaliseSchemaUseCase = createFinaliseSchemaUseCase({
		eventTypeRepository: repos.eventTypeRepository,
		unitOfWork,
	});

	const deprecateSchemaUseCase = createDeprecateSchemaUseCase({
		eventTypeRepository: repos.eventTypeRepository,
		unitOfWork,
	});

	const syncEventTypesUseCase = createSyncEventTypesUseCase({
		eventTypeRepository: repos.eventTypeRepository,
		unitOfWork,
	});

	// --- Dispatch Pool use cases (with client-scope guard for client-scoped pools) ---
	const createDispatchPoolUseCase = createGuardedUseCase(
		createCreateDispatchPoolUseCase({
			dispatchPoolRepository: repos.dispatchPoolRepository,
			clientRepository: repos.clientRepository,
			unitOfWork,
		}),
		clientScopedGuard(),
	);

	const updateDispatchPoolUseCase = createUpdateDispatchPoolUseCase({
		dispatchPoolRepository: repos.dispatchPoolRepository,
		unitOfWork,
	});

	const deleteDispatchPoolUseCase = createDeleteDispatchPoolUseCase({
		dispatchPoolRepository: repos.dispatchPoolRepository,
		unitOfWork,
	});

	const syncDispatchPoolsUseCase = createSyncDispatchPoolsUseCase({
		dispatchPoolRepository: repos.dispatchPoolRepository,
		unitOfWork,
	});

	// --- Subscription use cases (with client-scope guard for client-scoped subs) ---
	const createSubscriptionUseCase = createGuardedUseCase(
		createCreateSubscriptionUseCase({
			subscriptionRepository: repos.subscriptionRepository,
			dispatchPoolRepository: repos.dispatchPoolRepository,
			unitOfWork,
		}),
		clientScopedGuard(),
	);

	const updateSubscriptionUseCase = createUpdateSubscriptionUseCase({
		subscriptionRepository: repos.subscriptionRepository,
		dispatchPoolRepository: repos.dispatchPoolRepository,
		unitOfWork,
	});

	const deleteSubscriptionUseCase = createDeleteSubscriptionUseCase({
		subscriptionRepository: repos.subscriptionRepository,
		unitOfWork,
	});

	const syncSubscriptionsUseCase = createSyncSubscriptionsUseCase({
		subscriptionRepository: repos.subscriptionRepository,
		dispatchPoolRepository: repos.dispatchPoolRepository,
		unitOfWork,
	});

	// --- Identity Provider use cases ---
	const createIdentityProviderUseCase = createCreateIdentityProviderUseCase({
		identityProviderRepository: repos.identityProviderRepository,
		unitOfWork,
	});

	const updateIdentityProviderUseCase = createUpdateIdentityProviderUseCase({
		identityProviderRepository: repos.identityProviderRepository,
		unitOfWork,
	});

	const deleteIdentityProviderUseCase = createDeleteIdentityProviderUseCase({
		identityProviderRepository: repos.identityProviderRepository,
		unitOfWork,
	});

	// --- Email Domain Mapping use cases ---
	const createEmailDomainMappingUseCase = createCreateEmailDomainMappingUseCase(
		{
			emailDomainMappingRepository: repos.emailDomainMappingRepository,
			identityProviderRepository: repos.identityProviderRepository,
			unitOfWork,
		},
	);

	const updateEmailDomainMappingUseCase = createUpdateEmailDomainMappingUseCase(
		{
			emailDomainMappingRepository: repos.emailDomainMappingRepository,
			identityProviderRepository: repos.identityProviderRepository,
			unitOfWork,
		},
	);

	const deleteEmailDomainMappingUseCase = createDeleteEmailDomainMappingUseCase(
		{
			emailDomainMappingRepository: repos.emailDomainMappingRepository,
			unitOfWork,
		},
	);

	// --- Service Account use cases ---
	const createServiceAccountUseCase = createCreateServiceAccountUseCase({
		principalRepository: repos.principalRepository,
		oauthClientRepository: repos.oauthClientRepository,
		encryptionService,
		unitOfWork,
	});

	const updateServiceAccountUseCase = createUpdateServiceAccountUseCase({
		principalRepository: repos.principalRepository,
		unitOfWork,
	});

	const deleteServiceAccountUseCase = createDeleteServiceAccountUseCase({
		principalRepository: repos.principalRepository,
		oauthClientRepository: repos.oauthClientRepository,
		unitOfWork,
	});

	const regenerateAuthTokenUseCase = createRegenerateAuthTokenUseCase({
		principalRepository: repos.principalRepository,
		encryptionService,
		unitOfWork,
	});

	const regenerateSigningSecretUseCase = createRegenerateSigningSecretUseCase({
		principalRepository: repos.principalRepository,
		encryptionService,
		unitOfWork,
	});

	const assignServiceAccountRolesUseCase =
		createAssignServiceAccountRolesUseCase({
			principalRepository: repos.principalRepository,
			roleRepository: repos.roleRepository,
			unitOfWork,
		});

	// --- CORS use cases ---
	const addCorsOriginUseCase = createAddCorsOriginUseCase({
		corsAllowedOriginRepository: repos.corsAllowedOriginRepository,
		unitOfWork,
	});

	const deleteCorsOriginUseCase = createDeleteCorsOriginUseCase({
		corsAllowedOriginRepository: repos.corsAllowedOriginRepository,
		unitOfWork,
	});

	// --- Application access use case ---
	const assignApplicationAccessUseCase = createAssignApplicationAccessUseCase({
		principalRepository: repos.principalRepository,
		applicationRepository: repos.applicationRepository,
		applicationClientConfigRepository: repos.applicationClientConfigRepository,
		clientAccessGrantRepository: repos.clientAccessGrantRepository,
		unitOfWork,
	});

	return {
		createUserUseCase,
		updateUserUseCase,
		activateUserUseCase,
		deactivateUserUseCase,
		deleteUserUseCase,
		createClientUseCase,
		updateClientUseCase,
		changeClientStatusUseCase,
		deleteClientUseCase,
		addClientNoteUseCase,
		createAnchorDomainUseCase,
		updateAnchorDomainUseCase,
		deleteAnchorDomainUseCase,
		createApplicationUseCase,
		updateApplicationUseCase,
		deleteApplicationUseCase,
		enableApplicationForClientUseCase,
		disableApplicationForClientUseCase,
		activateApplicationUseCase,
		deactivateApplicationUseCase,
		createRoleUseCase,
		updateRoleUseCase,
		deleteRoleUseCase,
		syncRolesUseCase,
		syncPrincipalsUseCase,
		assignRolesUseCase,
		grantClientAccessUseCase,
		revokeClientAccessUseCase,
		createInternalAuthConfigUseCase,
		createOidcAuthConfigUseCase,
		updateOidcSettingsUseCase,
		updateConfigTypeUseCase,
		updateAdditionalClientsUseCase,
		updateGrantedClientsUseCase,
		deleteAuthConfigUseCase,
		createOAuthClientUseCase,
		updateOAuthClientUseCase,
		regenerateOAuthClientSecretUseCase,
		deleteOAuthClientUseCase,
		createEventTypeUseCase,
		updateEventTypeUseCase,
		archiveEventTypeUseCase,
		deleteEventTypeUseCase,
		addSchemaUseCase,
		finaliseSchemaUseCase,
		deprecateSchemaUseCase,
		syncEventTypesUseCase,
		createDispatchPoolUseCase,
		updateDispatchPoolUseCase,
		deleteDispatchPoolUseCase,
		syncDispatchPoolsUseCase,
		createSubscriptionUseCase,
		updateSubscriptionUseCase,
		deleteSubscriptionUseCase,
		syncSubscriptionsUseCase,
		createIdentityProviderUseCase,
		updateIdentityProviderUseCase,
		deleteIdentityProviderUseCase,
		createEmailDomainMappingUseCase,
		updateEmailDomainMappingUseCase,
		deleteEmailDomainMappingUseCase,
		createServiceAccountUseCase,
		updateServiceAccountUseCase,
		deleteServiceAccountUseCase,
		regenerateAuthTokenUseCase,
		regenerateSigningSecretUseCase,
		assignServiceAccountRolesUseCase,
		addCorsOriginUseCase,
		deleteCorsOriginUseCase,
		assignApplicationAccessUseCase,
	};
}

export type UseCases = ReturnType<typeof createUseCases>;
