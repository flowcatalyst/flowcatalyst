/**
 * Delete Email Domain Mapping Use Case
 */

import type { UseCase } from "@flowcatalyst/application";
import { Result, UseCaseError } from "@flowcatalyst/application";
import type { ExecutionContext, UnitOfWork } from "@flowcatalyst/domain";

import type { EmailDomainMappingRepository } from "../../../infrastructure/persistence/index.js";
import { EmailDomainMappingDeleted } from "../../../domain/index.js";

import type { DeleteEmailDomainMappingCommand } from "./command.js";

export interface DeleteEmailDomainMappingUseCaseDeps {
	readonly emailDomainMappingRepository: EmailDomainMappingRepository;
	readonly unitOfWork: UnitOfWork;
}

export function createDeleteEmailDomainMappingUseCase(
	deps: DeleteEmailDomainMappingUseCaseDeps,
): UseCase<DeleteEmailDomainMappingCommand, EmailDomainMappingDeleted> {
	const { emailDomainMappingRepository, unitOfWork } = deps;

	return {
		async execute(
			command: DeleteEmailDomainMappingCommand,
			context: ExecutionContext,
		): Promise<Result<EmailDomainMappingDeleted>> {
			const mapping = await emailDomainMappingRepository.findById(
				command.emailDomainMappingId,
			);
			if (!mapping) {
				return Result.failure(
					UseCaseError.notFound(
						"MAPPING_NOT_FOUND",
						"Email domain mapping not found",
						{
							emailDomainMappingId: command.emailDomainMappingId,
						},
					),
				);
			}

			const event = new EmailDomainMappingDeleted(context, {
				emailDomainMappingId: mapping.id,
				emailDomain: mapping.emailDomain,
				identityProviderId: mapping.identityProviderId,
			});

			return unitOfWork.commitDelete(mapping, event, command);
		},
	};
}
