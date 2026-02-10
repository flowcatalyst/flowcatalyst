package tech.flowcatalyst.platform.common;

import tech.flowcatalyst.platform.common.errors.UseCaseError;

import java.util.Map;

/**
 * Base interface for all use cases.
 *
 * <p>Provides a template method via {@link #execute(Object, ExecutionContext)} that
 * enforces resource-level authorization before delegating to business logic.
 *
 * <p>The two-level authorization model:
 * <ol>
 *   <li><strong>Action-level</strong> (API layer): "Can this principal perform this action?"
 *       — handled by {@code @RolesAllowed} or permission annotations on controllers</li>
 *   <li><strong>Resource-level</strong> (use case layer): "Can this principal perform this
 *       action on THIS specific resource?" — handled by {@link #authorizeResource}</li>
 * </ol>
 *
 * <p>Implementation example:
 * <pre>{@code
 * @ApplicationScoped
 * public class DeactivateUserUseCase implements UseCase<DeactivateUserCommand, UserDeactivated> {
 *
 *     @Override
 *     public boolean authorizeResource(DeactivateUserCommand command, ExecutionContext context) {
 *         return true; // No resource-level restriction
 *     }
 *
 *     @Override
 *     public Result<UserDeactivated> doExecute(DeactivateUserCommand command, ExecutionContext context) {
 *         // Business logic here
 *     }
 * }
 * }</pre>
 *
 * @param <C> The command type
 * @param <E> The domain event type (must extend {@link DomainEvent})
 */
public interface UseCase<C, E extends DomainEvent> {

    /**
     * Execute the use case with resource-level authorization.
     *
     * <p>This default method checks {@link #authorizeResource} before delegating
     * to {@link #doExecute}. Operations classes call this method.
     *
     * @param command The command to execute
     * @param context The execution context with tracing and principal info
     * @return Success with the domain event, or Failure with error
     */
    default Result<E> execute(C command, ExecutionContext context) {
        if (!authorizeResource(command, context)) {
            return Result.failure(new UseCaseError.AuthorizationError(
                "RESOURCE_ACCESS_DENIED",
                "Not authorized to access this resource",
                Map.of()
            ));
        }
        return doExecute(command, context);
    }

    /**
     * Resource-level authorization guard.
     *
     * <p>Return {@code true} if the principal in the context is authorized to
     * perform this operation on the specific resource identified by the command.
     * Return {@code true} if no resource-level restriction is needed.
     *
     * @param command The command identifying the target resource
     * @param context The execution context with principal and authorization info
     * @return true if authorized, false to reject with 403
     */
    boolean authorizeResource(C command, ExecutionContext context);

    /**
     * Business logic implementation.
     *
     * <p>This method contains the actual use case logic: validation, business
     * rule checks, aggregate creation, event emission, and UnitOfWork commit.
     *
     * @param command The command to execute
     * @param context The execution context with tracing and principal info
     * @return Success with the domain event, or Failure with error
     */
    Result<E> doExecute(C command, ExecutionContext context);
}
