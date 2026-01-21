package tech.flowcatalyst.dispatchjob.model;

/**
 * The kind (category) of a dispatch job.
 *
 * <p>This determines what type of work the dispatch job represents and how
 * the {@code code} field should be interpreted:
 *
 * <table>
 *   <tr><th>Kind</th><th>Code Contains</th><th>Example Code</th></tr>
 *   <tr><td>EVENT</td><td>Event type identifier</td><td>{@code order.created}</td></tr>
 *   <tr><td>TASK</td><td>Task code identifier</td><td>{@code send-welcome-email}</td></tr>
 * </table>
 *
 * <h3>EVENT</h3>
 * <p>Dispatch jobs of kind {@code EVENT} are created when an event occurs in the system
 * and needs to be delivered to subscribers. The {@code code} field contains the event
 * type identifier (e.g., {@code order.created}, {@code user.registered}).
 *
 * <h3>TASK</h3>
 * <p>Dispatch jobs of kind {@code TASK} represent asynchronous work items that need
 * to be executed. The {@code code} field contains the task type identifier
 * (e.g., {@code send-welcome-email}, {@code generate-report}).
 *
 * @see tech.flowcatalyst.dispatchjob.entity.DispatchJob#kind
 * @see tech.flowcatalyst.dispatchjob.entity.DispatchJob#code
 */
public enum DispatchKind {

    /**
     * An event dispatch - delivers an event to subscribers.
     *
     * <p>When {@code kind = EVENT}:
     * <ul>
     *   <li>{@code code} = the event type (e.g., {@code order.created})</li>
     *   <li>{@code eventId} = the source event's ID</li>
     *   <li>{@code subject} = the aggregate reference from the event</li>
     * </ul>
     */
    EVENT,

    /**
     * A task dispatch - executes an asynchronous task.
     *
     * <p>When {@code kind = TASK}:
     * <ul>
     *   <li>{@code code} = the task code (e.g., {@code send-welcome-email})</li>
     *   <li>{@code eventId} = optionally, the triggering event's ID</li>
     *   <li>{@code subject} = the entity/resource the task operates on</li>
     * </ul>
     */
    TASK
}
