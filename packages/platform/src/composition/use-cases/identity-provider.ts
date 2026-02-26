/**
 * Identity provider use cases.
 */

import type { CreateUseCasesDeps } from "./index.js";
import {
	createCreateIdentityProviderUseCase,
	createUpdateIdentityProviderUseCase,
	createDeleteIdentityProviderUseCase,
} from "../../application/index.js";

export function createIdentityProviderUseCases(deps: CreateUseCasesDeps) {
	const { repos, unitOfWork } = deps;

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

	return {
		createIdentityProviderUseCase,
		updateIdentityProviderUseCase,
		deleteIdentityProviderUseCase,
	};
}
