/**
 * Add Schema Use Case
 *
 * Adds a new spec version to an event type in FINALISING status.
 */

import type { UseCase } from "@flowcatalyst/application";
import {
	validateRequired,
	Result,
	UseCaseError,
} from "@flowcatalyst/application";
import type { ExecutionContext, UnitOfWork } from "@flowcatalyst/domain-core";

import type { EventTypeRepository } from "../../../infrastructure/persistence/index.js";
import {
	addSpecVersion,
	findSpecVersion,
	createSpecVersion,
	SchemaAdded,
} from "../../../domain/index.js";

import type { AddSchemaCommand } from "./command.js";

const VERSION_PATTERN = /^\d+\.\d+$/;

export interface AddSchemaUseCaseDeps {
	readonly eventTypeRepository: EventTypeRepository;
	readonly unitOfWork: UnitOfWork;
}

export function createAddSchemaUseCase(
	deps: AddSchemaUseCaseDeps,
): UseCase<AddSchemaCommand, SchemaAdded> {
	const { eventTypeRepository, unitOfWork } = deps;

	return {
		async execute(
			command: AddSchemaCommand,
			context: ExecutionContext,
		): Promise<Result<SchemaAdded>> {
			// Validate version format
			const versionResult = validateRequired(
				command.version,
				"version",
				"VERSION_REQUIRED",
			);
			if (Result.isFailure(versionResult)) return versionResult;

			if (!VERSION_PATTERN.test(command.version)) {
				return Result.failure(
					UseCaseError.validation(
						"INVALID_VERSION",
						'Version must be in MAJOR.MINOR format (e.g., "1.0")',
					),
				);
			}

			// Validate required fields
			const mimeResult = validateRequired(
				command.mimeType,
				"mimeType",
				"MIME_TYPE_REQUIRED",
			);
			if (Result.isFailure(mimeResult)) return mimeResult;

			if (
				command.schemaContent === null ||
				command.schemaContent === undefined
			) {
				return Result.failure(
					UseCaseError.validation(
						"SCHEMA_REQUIRED",
						"Schema content is required",
					),
				);
			}

			// Load event type
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

			// Cannot add schema to archived event types
			if (eventType.status === "ARCHIVED") {
				return Result.failure(
					UseCaseError.businessRule(
						"EVENT_TYPE_ARCHIVED",
						"Cannot add schema to an archived event type",
					),
				);
			}

			// Check version uniqueness
			const existing = findSpecVersion(eventType, command.version);
			if (existing) {
				return Result.failure(
					UseCaseError.businessRule(
						"VERSION_EXISTS",
						"Schema version already exists",
						{
							version: command.version,
						},
					),
				);
			}

			// Create spec version
			const specVersion = createSpecVersion({
				eventTypeId: eventType.id,
				version: command.version,
				mimeType: command.mimeType,
				schemaContent: command.schemaContent,
				schemaType: command.schemaType,
			});

			const updated = addSpecVersion(eventType, specVersion as any);

			const event = new SchemaAdded(context, {
				eventTypeId: eventType.id,
				version: command.version,
				mimeType: command.mimeType,
				schemaType: command.schemaType,
			});

			return unitOfWork.commit(updated, event, command);
		},
	};
}
