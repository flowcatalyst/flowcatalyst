/**
 * Email domain mapping use cases.
 */

import type { CreateUseCasesDeps } from "./index.js";
import {
	createCreateEmailDomainMappingUseCase,
	createUpdateEmailDomainMappingUseCase,
	createDeleteEmailDomainMappingUseCase,
} from "../../application/index.js";

export function createEmailDomainUseCases(deps: CreateUseCasesDeps) {
	const { repos, unitOfWork } = deps;

	const createEmailDomainMappingUseCase = createCreateEmailDomainMappingUseCase(
		{
			emailDomainMappingRepository: repos.emailDomainMappingRepository,
			identityProviderRepository: repos.identityProviderRepository,
			principalRepository: repos.principalRepository,
			clientAccessGrantRepository: repos.clientAccessGrantRepository,
			unitOfWork,
		},
	);

	const updateEmailDomainMappingUseCase = createUpdateEmailDomainMappingUseCase(
		{
			emailDomainMappingRepository: repos.emailDomainMappingRepository,
			identityProviderRepository: repos.identityProviderRepository,
			principalRepository: repos.principalRepository,
			clientAccessGrantRepository: repos.clientAccessGrantRepository,
			unitOfWork,
		},
	);

	const deleteEmailDomainMappingUseCase = createDeleteEmailDomainMappingUseCase(
		{
			emailDomainMappingRepository: repos.emailDomainMappingRepository,
			unitOfWork,
		},
	);

	return {
		createEmailDomainMappingUseCase,
		updateEmailDomainMappingUseCase,
		deleteEmailDomainMappingUseCase,
	};
}
