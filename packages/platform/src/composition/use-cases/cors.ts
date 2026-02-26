/**
 * CORS use cases — allowed origins.
 */

import type { CreateUseCasesDeps } from "./index.js";
import {
	createAddCorsOriginUseCase,
	createDeleteCorsOriginUseCase,
} from "../../application/index.js";

export function createCorsUseCases(deps: CreateUseCasesDeps) {
	const { repos, unitOfWork } = deps;

	const addCorsOriginUseCase = createAddCorsOriginUseCase({
		corsAllowedOriginRepository: repos.corsAllowedOriginRepository,
		unitOfWork,
	});

	const deleteCorsOriginUseCase = createDeleteCorsOriginUseCase({
		corsAllowedOriginRepository: repos.corsAllowedOriginRepository,
		unitOfWork,
	});

	return {
		addCorsOriginUseCase,
		deleteCorsOriginUseCase,
	};
}
