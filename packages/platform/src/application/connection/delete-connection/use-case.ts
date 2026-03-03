/**
 * Delete Connection Use Case
 *
 * Hard deletes a connection. Fails if subscriptions reference it.
 */

import type { UseCase } from "@flowcatalyst/application";
import { Result, UseCaseError } from "@flowcatalyst/application";
import type { ExecutionContext, UnitOfWork } from "@flowcatalyst/domain";

import type {
	ConnectionRepository,
	SubscriptionRepository,
} from "../../../infrastructure/persistence/index.js";
import { ConnectionDeleted } from "../../../domain/index.js";

import type { DeleteConnectionCommand } from "./command.js";

export interface DeleteConnectionUseCaseDeps {
	readonly connectionRepository: ConnectionRepository;
	readonly subscriptionRepository: SubscriptionRepository;
	readonly unitOfWork: UnitOfWork;
}

export function createDeleteConnectionUseCase(
	deps: DeleteConnectionUseCaseDeps,
): UseCase<DeleteConnectionCommand, ConnectionDeleted> {
	const { connectionRepository, subscriptionRepository, unitOfWork } = deps;

	return {
		async execute(
			command: DeleteConnectionCommand,
			context: ExecutionContext,
		): Promise<Result<ConnectionDeleted>> {
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

			// Check if any subscriptions reference this connection
			const hasSubscriptions =
				await subscriptionRepository.existsByConnectionId(connection.id);
			if (hasSubscriptions) {
				return Result.failure(
					UseCaseError.businessRule(
						"CONNECTION_HAS_SUBSCRIPTIONS",
						"Cannot delete a connection that has subscriptions. Remove all subscriptions first.",
						{ connectionId: connection.id },
					),
				);
			}

			const event = new ConnectionDeleted(context, {
				connectionId: connection.id,
				code: connection.code,
				clientId: connection.clientId,
			});

			return unitOfWork.commitDelete(connection, event, command);
		},
	};
}
