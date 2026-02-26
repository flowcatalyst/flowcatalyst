/**
 * OAuth use cases — OAuth clients.
 */

import type { CreateUseCasesDeps } from "./index.js";
import {
	createCreateOAuthClientUseCase,
	createUpdateOAuthClientUseCase,
	createRegenerateOAuthClientSecretUseCase,
	createDeleteOAuthClientUseCase,
} from "../../application/index.js";

export function createOAuthUseCases(deps: CreateUseCasesDeps) {
	const { repos, unitOfWork } = deps;

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

	return {
		createOAuthClientUseCase,
		updateOAuthClientUseCase,
		regenerateOAuthClientSecretUseCase,
		deleteOAuthClientUseCase,
	};
}
