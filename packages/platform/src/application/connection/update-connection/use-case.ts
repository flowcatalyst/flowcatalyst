/**
 * Update Connection Use Case
 */

import type { UseCase } from "@flowcatalyst/application";
import { Result, UseCaseError } from "@flowcatalyst/application";
import type { ExecutionContext, UnitOfWork } from "@flowcatalyst/domain";

import type {
	ConnectionRepository,
} from "../../../infrastructure/persistence/index.js";
import {
	updateConnection,
	ConnectionUpdated,
} from "../../../domain/index.js";

import type { UpdateConnectionCommand } from "./command.js";

export interface UpdateConnectionUseCaseDeps {
	readonly connectionRepository: ConnectionRepository;
	readonly unitOfWork: UnitOfWork;
}

export function createUpdateConnectionUseCase(
	deps: UpdateConnectionUseCaseDeps,
): UseCase<UpdateConnectionCommand, ConnectionUpdated> {
	const { connectionRepository, unitOfWork } = deps;

	return {
		async execute(
			command: UpdateConnectionCommand,
			context: ExecutionContext,
		): Promise<Result<ConnectionUpdated>> {
			const connection = await connectionRepository.findById(
				command.connectionId,
			);
			if (!connection) {
				return Result.failure(
					UseCaseError.notFound(
						"CONNECTION_NOT_FOUND",
						"Connection not found",
						{ connectionId: command.connectionId },
					),
				);
			}

			const updated = updateConnection(connection, {
				...(command.name !== undefined ? { name: command.name } : {}),
				...(command.description !== undefined
					? { description: command.description }
					: {}),
				...(command.endpoint !== undefined
					? { endpoint: command.endpoint }
					: {}),
				...(command.externalId !== undefined
					? { externalId: command.externalId }
					: {}),
				...(command.status !== undefined ? { status: command.status } : {}),
				...(command.serviceAccountId !== undefined
					? { serviceAccountId: command.serviceAccountId }
					: {}),
			});

			const event = new ConnectionUpdated(context, {
				connectionId: updated.id,
				code: updated.code,
				name: updated.name,
				endpoint: updated.endpoint,
				externalId: updated.externalId,
				status: updated.status,
			});

			return unitOfWork.commit(updated, event, command);
		},
	};
}
