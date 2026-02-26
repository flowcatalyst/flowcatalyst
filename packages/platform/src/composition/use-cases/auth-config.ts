/**
 * Auth config use cases — internal, OIDC, settings.
 */

import type { CreateUseCasesDeps } from "./index.js";
import {
	createCreateInternalAuthConfigUseCase,
	createCreateOidcAuthConfigUseCase,
	createUpdateOidcSettingsUseCase,
	createUpdateConfigTypeUseCase,
	createUpdateAdditionalClientsUseCase,
	createUpdateGrantedClientsUseCase,
	createDeleteAuthConfigUseCase,
} from "../../application/index.js";

export function createAuthConfigUseCases(deps: CreateUseCasesDeps) {
	const { repos, unitOfWork } = deps;

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

	return {
		createInternalAuthConfigUseCase,
		createOidcAuthConfigUseCase,
		updateOidcSettingsUseCase,
		updateConfigTypeUseCase,
		updateAdditionalClientsUseCase,
		updateGrantedClientsUseCase,
		deleteAuthConfigUseCase,
	};
}
