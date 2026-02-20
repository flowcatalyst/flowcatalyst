/**
 * Deprecate Schema Use Case
 *
 * Deprecates a CURRENT schema version.
 * Cannot deprecate FINALISING schemas.
 */

import type { UseCase } from "@flowcatalyst/application";
import { Result, UseCaseError } from "@flowcatalyst/application";
import type { ExecutionContext, UnitOfWork } from "@flowcatalyst/domain-core";

import type { EventTypeRepository } from "../../../infrastructure/persistence/index.js";
import {
	findSpecVersion,
	updateSpecVersion,
	withStatus,
	SchemaDeprecated,
} from "../../../domain/index.js";

import type { DeprecateSchemaCommand } from "./command.js";

export interface DeprecateSchemaUseCaseDeps {
	readonly eventTypeRepository: EventTypeRepository;
	readonly unitOfWork: UnitOfWork;
}

export function createDeprecateSchemaUseCase(
	deps: DeprecateSchemaUseCaseDeps,
): UseCase<DeprecateSchemaCommand, SchemaDeprecated> {
	const { eventTypeRepository, unitOfWork } = deps;

	return {
		async execute(
			command: DeprecateSchemaCommand,
			context: ExecutionContext,
		): Promise<Result<SchemaDeprecated>> {
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

			const specVersion = findSpecVersion(eventType, command.version);
			if (!specVersion) {
				return Result.failure(
					UseCaseError.notFound(
						"VERSION_NOT_FOUND",
						"Schema version not found",
						{
							version: command.version,
						},
					),
				);
			}

			if (specVersion.status === "FINALISING") {
				return Result.failure(
					UseCaseError.businessRule(
						"CANNOT_DEPRECATE_FINALISING",
						"Cannot deprecate a schema that is still in FINALISING status",
					),
				);
			}

			if (specVersion.status === "DEPRECATED") {
				return Result.failure(
					UseCaseError.businessRule(
						"ALREADY_DEPRECATED",
						"Schema is already deprecated",
					),
				);
			}

			const updated = updateSpecVersion(eventType, command.version, (sv) =>
				withStatus(sv, "DEPRECATED"),
			);

			const event = new SchemaDeprecated(context, {
				eventTypeId: eventType.id,
				version: command.version,
			});

			return unitOfWork.commit(updated, event, command);
		},
	};
}
