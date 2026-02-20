/**
 * Archive EventType Use Case
 *
 * Archives an event type. Requires all spec versions to be deprecated first.
 */

import type { UseCase } from "@flowcatalyst/application";
import { Result, UseCaseError } from "@flowcatalyst/application";
import type { ExecutionContext, UnitOfWork } from "@flowcatalyst/domain-core";

import type { EventTypeRepository } from "../../../infrastructure/persistence/index.js";
import {
	archiveEventType,
	allVersionsDeprecated,
	EventTypeArchived,
} from "../../../domain/index.js";

import type { ArchiveEventTypeCommand } from "./command.js";

export interface ArchiveEventTypeUseCaseDeps {
	readonly eventTypeRepository: EventTypeRepository;
	readonly unitOfWork: UnitOfWork;
}

export function createArchiveEventTypeUseCase(
	deps: ArchiveEventTypeUseCaseDeps,
): UseCase<ArchiveEventTypeCommand, EventTypeArchived> {
	const { eventTypeRepository, unitOfWork } = deps;

	return {
		async execute(
			command: ArchiveEventTypeCommand,
			context: ExecutionContext,
		): Promise<Result<EventTypeArchived>> {
			const eventType = await eventTypeRepository.findById(command.eventTypeId);
			if (!eventType) {
				return Result.failure(
					UseCaseError.notFound(
						"EVENT_TYPE_NOT_FOUND",
						"Event type not found",
						{
							eventTypeId: command.eventTypeId,
						},
					),
				);
			}

			if (eventType.status === "ARCHIVED") {
				return Result.failure(
					UseCaseError.businessRule(
						"ALREADY_ARCHIVED",
						"Event type is already archived",
					),
				);
			}

			if (!allVersionsDeprecated(eventType)) {
				return Result.failure(
					UseCaseError.businessRule(
						"VERSIONS_NOT_DEPRECATED",
						"All spec versions must be deprecated before archiving",
					),
				);
			}

			const archived = archiveEventType(eventType);

			const event = new EventTypeArchived(context, {
				eventTypeId: archived.id,
				code: archived.code,
			});

			return unitOfWork.commit(archived, event, command);
		},
	};
}
