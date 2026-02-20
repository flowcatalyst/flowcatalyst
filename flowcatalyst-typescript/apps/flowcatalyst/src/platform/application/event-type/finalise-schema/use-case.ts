/**
 * Finalise Schema Use Case
 *
 * Finalises a schema from FINALISING to CURRENT status.
 * Auto-deprecates any existing CURRENT schema with the same major version.
 */

import type { UseCase } from "@flowcatalyst/application";
import { Result, UseCaseError } from "@flowcatalyst/application";
import type { ExecutionContext, UnitOfWork } from "@flowcatalyst/domain-core";

import type { EventTypeRepository } from "../../../infrastructure/persistence/index.js";
import {
	findSpecVersion,
	updateSpecVersion,
	majorVersion,
	withStatus,
	SchemaFinalised,
} from "../../../domain/index.js";

import type { FinaliseSchemaCommand } from "./command.js";

export interface FinaliseSchemaUseCaseDeps {
	readonly eventTypeRepository: EventTypeRepository;
	readonly unitOfWork: UnitOfWork;
}

export function createFinaliseSchemaUseCase(
	deps: FinaliseSchemaUseCaseDeps,
): UseCase<FinaliseSchemaCommand, SchemaFinalised> {
	const { eventTypeRepository, unitOfWork } = deps;

	return {
		async execute(
			command: FinaliseSchemaCommand,
			context: ExecutionContext,
		): Promise<Result<SchemaFinalised>> {
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

			if (specVersion.status !== "FINALISING") {
				return Result.failure(
					UseCaseError.businessRule(
						"NOT_FINALISING",
						"Only schemas in FINALISING status can be finalised",
						{ currentStatus: specVersion.status },
					),
				);
			}

			// Move target schema to CURRENT
			let updated = updateSpecVersion(eventType, command.version, (sv) =>
				withStatus(sv, "CURRENT"),
			);

			// Auto-deprecate existing CURRENT with same major version
			const targetMajor = majorVersion(command.version);
			let deprecatedVersion: string | null = null;

			for (const sv of eventType.specVersions) {
				if (
					sv.version !== command.version &&
					sv.status === "CURRENT" &&
					majorVersion(sv.version) === targetMajor
				) {
					updated = updateSpecVersion(updated, sv.version, (s) =>
						withStatus(s, "DEPRECATED"),
					);
					deprecatedVersion = sv.version;
					break;
				}
			}

			const event = new SchemaFinalised(context, {
				eventTypeId: eventType.id,
				version: command.version,
				deprecatedVersion,
			});

			return unitOfWork.commit(updated, event, command);
		},
	};
}
