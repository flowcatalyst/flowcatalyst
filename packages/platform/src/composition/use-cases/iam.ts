/**
 * IAM use cases — principals, roles, client access, service accounts, sync.
 */

import type { CreateUseCasesDeps } from "./index.js";
import {
	createGuardedUseCase,
	clientAccessGuard,
} from "../../authorization/index.js";
import {
	createCreateUserUseCase,
	createUpdateUserUseCase,
	createActivateUserUseCase,
	createDeactivateUserUseCase,
	createDeleteUserUseCase,
	createCreateRoleUseCase,
	createUpdateRoleUseCase,
	createDeleteRoleUseCase,
	createAssignRolesUseCase,
	createGrantClientAccessUseCase,
	createRevokeClientAccessUseCase,
	createCreateServiceAccountUseCase,
	createUpdateServiceAccountUseCase,
	createDeleteServiceAccountUseCase,
	createRegenerateAuthTokenUseCase,
	createRegenerateSigningSecretUseCase,
	createAssignServiceAccountRolesUseCase,
	createSyncRolesUseCase,
	createSyncPrincipalsUseCase,
} from "../../application/index.js";

export function createIamUseCases(deps: CreateUseCasesDeps) {
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
		emailDomainMappingRepository: repos.emailDomainMappingRepository,
		anchorDomainRepository: repos.anchorDomainRepository,
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

	const assignRolesUseCase = createAssignRolesUseCase({
		principalRepository: repos.principalRepository,
		roleRepository: repos.roleRepository,
		unitOfWork,
	});

	// --- Client access use cases ---
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

	// --- Service account use cases ---
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

	// --- Sync use cases ---
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

	return {
		createUserUseCase,
		updateUserUseCase,
		activateUserUseCase,
		deactivateUserUseCase,
		deleteUserUseCase,
		createRoleUseCase,
		updateRoleUseCase,
		deleteRoleUseCase,
		assignRolesUseCase,
		grantClientAccessUseCase,
		revokeClientAccessUseCase,
		createServiceAccountUseCase,
		updateServiceAccountUseCase,
		deleteServiceAccountUseCase,
		regenerateAuthTokenUseCase,
		regenerateSigningSecretUseCase,
		assignServiceAccountRolesUseCase,
		syncRolesUseCase,
		syncPrincipalsUseCase,
	};
}

export type IamUseCases = ReturnType<typeof createIamUseCases>;
