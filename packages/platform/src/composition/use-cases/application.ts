/**
 * Application use cases — CRUD, activate/deactivate, enable/disable for client.
 */

import type { CreateUseCasesDeps } from "./index.js";
import {
	createGuardedUseCase,
	clientAccessGuard,
} from "../../authorization/index.js";
import {
	createCreateApplicationUseCase,
	createUpdateApplicationUseCase,
	createDeleteApplicationUseCase,
	createActivateApplicationUseCase,
	createDeactivateApplicationUseCase,
	createEnableApplicationForClientUseCase,
	createDisableApplicationForClientUseCase,
	createAssignApplicationAccessUseCase,
} from "../../application/index.js";

export function createApplicationUseCases(deps: CreateUseCasesDeps) {
	const { repos, unitOfWork } = deps;

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

	const assignApplicationAccessUseCase = createAssignApplicationAccessUseCase({
		principalRepository: repos.principalRepository,
		applicationRepository: repos.applicationRepository,
		applicationClientConfigRepository: repos.applicationClientConfigRepository,
		clientAccessGrantRepository: repos.clientAccessGrantRepository,
		unitOfWork,
	});

	return {
		createApplicationUseCase,
		updateApplicationUseCase,
		deleteApplicationUseCase,
		enableApplicationForClientUseCase,
		disableApplicationForClientUseCase,
		activateApplicationUseCase,
		deactivateApplicationUseCase,
		assignApplicationAccessUseCase,
	};
}
