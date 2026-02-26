/**
 * Resource-Level Authorization Guards
 *
 * Provides use case wrappers that enforce resource-level authorization.
 * While action-level auth (requirePermission) checks "can this principal
 * perform this action?", resource guards check "can this principal perform
 * this action on THIS specific resource?"
 *
 * Guards use AuditContext (AsyncLocalStorage) to access the current principal,
 * so they work transparently within use case execution.
 *
 * When authorization fails, guards return NOT_FOUND (not FORBIDDEN) to avoid
 * leaking the existence of resources the principal cannot access.
 */

import type { UseCase, Command } from "@flowcatalyst/application";
import { Result, UseCaseError } from "@flowcatalyst/application";
import {
	AuditContext,
	type PrincipalInfo,
	type ExecutionContext,
} from "@flowcatalyst/domain";
import type { DomainEvent } from "@flowcatalyst/domain";

import { canAccessClient } from "./authorization-service.js";

/**
 * A resource guard function that determines whether the current principal
 * can access the resource referenced by the command.
 *
 * @param command - The command being executed
 * @param principal - The authenticated principal
 * @returns true if authorized, false if not
 */
export type ResourceGuard<TCommand> = (
	command: TCommand,
	principal: PrincipalInfo,
) => boolean | Promise<boolean>;

/**
 * Wrap a use case with a resource-level authorization guard.
 *
 * The guard runs before the use case. If it returns false, the use case
 * is not executed and a NOT_FOUND error is returned (to avoid leaking
 * resource existence).
 *
 * @param useCase - The use case to wrap
 * @param guard - The resource guard function
 * @returns A guarded use case
 */
export function createGuardedUseCase<
	TCommand extends Command,
	TEvent extends DomainEvent,
>(
	useCase: UseCase<TCommand, TEvent>,
	guard: ResourceGuard<TCommand>,
): UseCase<TCommand, TEvent> {
	return {
		async execute(
			command: TCommand,
			context: ExecutionContext,
		): Promise<Result<TEvent>> {
			const principal = AuditContext.getPrincipal();
			if (!principal) {
				return Result.failure(
					UseCaseError.notFound("RESOURCE_NOT_FOUND", "Resource not found"),
				);
			}

			const authorized = await guard(command, principal);
			if (!authorized) {
				return Result.failure(
					UseCaseError.notFound("RESOURCE_NOT_FOUND", "Resource not found"),
				);
			}

			return useCase.execute(command, context);
		},
	};
}

// ─── Common Guard Predicates ────────────────────────────────────────────────

/**
 * Guard that always allows access.
 * Use for use cases that don't need resource-level restrictions
 * (e.g., platform-wide operations that are already protected by action-level auth).
 */
export function noResourceRestriction<TCommand>(): ResourceGuard<TCommand> {
	return () => true;
}

/**
 * Guard that checks client access for commands with a clientId field.
 * ANCHOR scope users can access all clients.
 * CLIENT scope users can only access their home client.
 * PARTNER scope users can access explicitly granted clients.
 *
 * If the command has no clientId (null/undefined), access is allowed
 * (the operation is not client-scoped).
 */
export function clientScopedGuard<
	TCommand extends { clientId?: string | null | undefined },
>(): ResourceGuard<TCommand> {
	return (command, principal) => {
		if (!command.clientId) {
			// Not client-scoped, allow
			return true;
		}
		return canAccessClient(principal, command.clientId);
	};
}

/**
 * Guard that checks client access using a custom field extractor.
 * Use when the client ID is not in a standard `clientId` field.
 *
 * @param getClientId - Function to extract client ID from the command
 */
export function clientAccessGuard<TCommand>(
	getClientId: (command: TCommand) => string | null | undefined,
): ResourceGuard<TCommand> {
	return (command, principal) => {
		const clientId = getClientId(command);
		if (!clientId) {
			return true;
		}
		return canAccessClient(principal, clientId);
	};
}

/**
 * Guard that checks if the principal has ANCHOR scope (platform admin).
 * Use for operations that should only be performed by platform administrators.
 */
export function anchorOnlyGuard<TCommand>(): ResourceGuard<TCommand> {
	return (_command, principal) => {
		return principal.scope === "ANCHOR";
	};
}

/**
 * Compose multiple guards with AND semantics.
 * All guards must pass for the operation to proceed.
 */
export function allGuards<TCommand>(
	...guards: ResourceGuard<TCommand>[]
): ResourceGuard<TCommand> {
	return async (command, principal) => {
		for (const guard of guards) {
			const result = await guard(command, principal);
			if (!result) return false;
		}
		return true;
	};
}
