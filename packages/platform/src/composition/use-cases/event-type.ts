/**
 * Event type use cases — event types and schemas.
 */

import type { CreateUseCasesDeps } from "./index.js";
import {
	createCreateEventTypeUseCase,
	createUpdateEventTypeUseCase,
	createArchiveEventTypeUseCase,
	createDeleteEventTypeUseCase,
	createAddSchemaUseCase,
	createFinaliseSchemaUseCase,
	createDeprecateSchemaUseCase,
	createSyncEventTypesUseCase,
} from "../../application/index.js";

export function createEventTypeUseCases(deps: CreateUseCasesDeps) {
	const { repos, unitOfWork } = deps;

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

	return {
		createEventTypeUseCase,
		updateEventTypeUseCase,
		archiveEventTypeUseCase,
		deleteEventTypeUseCase,
		addSchemaUseCase,
		finaliseSchemaUseCase,
		deprecateSchemaUseCase,
		syncEventTypesUseCase,
	};
}
