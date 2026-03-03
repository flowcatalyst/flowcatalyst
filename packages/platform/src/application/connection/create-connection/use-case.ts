/**
 * Create Connection Use Case
 */

import type { UseCase } from "@flowcatalyst/application";
import {
	validateRequired,
	Result,
	UseCaseError,
} from "@flowcatalyst/application";
import type { ExecutionContext, UnitOfWork } from "@flowcatalyst/domain";

import type {
	ConnectionRepository,
} from "../../../infrastructure/persistence/index.js";
import type { ServiceAccountRepository } from "../../../infrastructure/persistence/repositories/service-account-repository.js";
import type { ClientRepository } from "../../../infrastructure/persistence/repositories/client-repository.js";
import {
	createConnection,
	ConnectionCreated,
} from "../../../domain/index.js";

import type { CreateConnectionCommand } from "./command.js";

export interface CreateConnectionUseCaseDeps {
	readonly connectionRepository: ConnectionRepository;
	readonly serviceAccountRepository: ServiceAccountRepository;
	readonly clientRepository: ClientRepository;
	readonly unitOfWork: UnitOfWork;
}

const CODE_PATTERN = /^[a-z][a-z0-9-]*$/;

export function createCreateConnectionUseCase(
	deps: CreateConnectionUseCaseDeps,
): UseCase<CreateConnectionCommand, ConnectionCreated> {
	const {
		connectionRepository,
		serviceAccountRepository,
		clientRepository,
		unitOfWork,
	} = deps;

	return {
		async execute(
			command: CreateConnectionCommand,
			context: ExecutionContext,
		): Promise<Result<ConnectionCreated>> {
			// Validate required fields
			const codeResult = validateRequired(
				command.code,
				"code",
				"CODE_REQUIRED",
			);
			if (Result.isFailure(codeResult)) return codeResult;

			const nameResult = validateRequired(
				command.name,
				"name",
				"NAME_REQUIRED",
			);
			if (Result.isFailure(nameResult)) return nameResult;

			const endpointResult = validateRequired(
				command.endpoint,
				"endpoint",
				"ENDPOINT_REQUIRED",
			);
			if (Result.isFailure(endpointResult)) return endpointResult;

			const saResult = validateRequired(
				command.serviceAccountId,
				"serviceAccountId",
				"SERVICE_ACCOUNT_ID_REQUIRED",
			);
			if (Result.isFailure(saResult)) return saResult;

			// Validate code format
			if (!CODE_PATTERN.test(command.code)) {
				return Result.failure(
					UseCaseError.validation(
						"INVALID_CODE_FORMAT",
						"Code must start with a lowercase letter and contain only lowercase letters, numbers, and hyphens",
					),
				);
			}

			// Validate service account exists
			const sa = await serviceAccountRepository.findById(
				command.serviceAccountId,
			);
			if (!sa) {
				return Result.failure(
					UseCaseError.notFound(
						"SERVICE_ACCOUNT_NOT_FOUND",
						"Service account not found",
						{ serviceAccountId: command.serviceAccountId },
					),
				);
			}

			// Validate client exists if provided
			const clientId = command.clientId ?? null;
			if (clientId) {
				const clientExists = await clientRepository.exists(clientId);
				if (!clientExists) {
					return Result.failure(
						UseCaseError.notFound(
							"CLIENT_NOT_FOUND",
							"Client not found",
							{ clientId },
						),
					);
				}
			}

			// Check code uniqueness per client scope
			const codeExists = await connectionRepository.existsByCodeAndClientId(
				command.code,
				clientId,
			);
			if (codeExists) {
				return Result.failure(
					UseCaseError.businessRule(
						"CODE_EXISTS",
						"Connection code already exists in this scope",
						{ code: command.code },
					),
				);
			}

			const connection = createConnection({
				code: command.code,
				name: command.name,
				description: command.description ?? null,
				endpoint: command.endpoint,
				externalId: command.externalId ?? null,
				serviceAccountId: command.serviceAccountId,
				clientId,
			});

			const event = new ConnectionCreated(context, {
				connectionId: connection.id,
				code: connection.code,
				name: connection.name,
				endpoint: connection.endpoint,
				externalId: connection.externalId,
				serviceAccountId: connection.serviceAccountId,
				clientId: connection.clientId,
			});

			return unitOfWork.commit(connection, event, command);
		},
	};
}
