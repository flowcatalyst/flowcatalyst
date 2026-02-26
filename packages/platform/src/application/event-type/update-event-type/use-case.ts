/**
 * Update EventType Use Case
 */

import type { UseCase } from "@flowcatalyst/application";
import { Result, UseCaseError } from "@flowcatalyst/application";
import type { ExecutionContext, UnitOfWork } from "@flowcatalyst/domain";

import type { EventTypeRepository } from "../../../infrastructure/persistence/index.js";
import { updateEventType, EventTypeUpdated } from "../../../domain/index.js";

import type { UpdateEventTypeCommand } from "./command.js";

export interface UpdateEventTypeUseCaseDeps {
	readonly eventTypeRepository: EventTypeRepository;
	readonly unitOfWork: UnitOfWork;
}

export function createUpdateEventTypeUseCase(
	deps: UpdateEventTypeUseCaseDeps,
): UseCase<UpdateEventTypeCommand, EventTypeUpdated> {
	const { eventTypeRepository, unitOfWork } = deps;

	return {
		async execute(
			command: UpdateEventTypeCommand,
			context: ExecutionContext,
		): Promise<Result<EventTypeUpdated>> {
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

			// Must have at least one field to update
			if (command.name === undefined && command.description === undefined) {
				return Result.failure(
					UseCaseError.validation(
						"NO_FIELDS",
						"At least one field must be provided",
					),
				);
			}

			if (command.name !== undefined && command.name.length > 100) {
				return Result.failure(
					UseCaseError.validation(
						"NAME_TOO_LONG",
						"Name must be 100 characters or less",
					),
				);
			}

			if (
				command.description !== undefined &&
				command.description !== null &&
				command.description.length > 255
			) {
				return Result.failure(
					UseCaseError.validation(
						"DESCRIPTION_TOO_LONG",
						"Description must be 255 characters or less",
					),
				);
			}

			const updated = updateEventType(eventType, {
				...(command.name !== undefined ? { name: command.name } : {}),
				...(command.description !== undefined
					? { description: command.description }
					: {}),
			});

			const event = new EventTypeUpdated(context, {
				eventTypeId: updated.id,
				name: updated.name,
				description: updated.description,
			});

			return unitOfWork.commit(updated, event, command);
		},
	};
}
