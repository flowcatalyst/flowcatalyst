package io.flowcatalyst.function;

import java.time.Instant;
import java.util.Objects;

/// A subscription/dispatch-job delivery's envelope, parsed by
/// [Webhook#event(Request)] from a `webhook`-endpoint [Request]'s body.
/// Field-for-field the wire shape the platform sends
/// (`internal/platform/dispatchjob/processing`, `buildPayload`):
///
/// ```
/// {id, type, attemptNumber, time?, dataContentType?, source?, subject?,
///  correlationId?, messageGroup?, clientId?, clientCode?, data?}
/// ```
///
/// Only the shape of a **non-`dataOnly`** delivery. A `dataOnly` job sends its
/// payload verbatim as the whole body, which is not this envelope at all; a
/// function declaring such an endpoint reads it with [Webhook#dataOnly(Request)].
///
/// `dataJson` is the raw JSON text of the `data` member exactly as it
/// appeared on the wire (an object, array, string, number, boolean), never
/// parsed further here: this jar carries no JSON library, so a function
/// parses it with its own. A payload that was not JSON arrives as a JSON
/// string. `null` when `data` was absent.
///
/// @param id              the dispatch job's own id
/// @param type            the event type / dispatch job code
/// @param attemptNumber   this delivery attempt, 1-based
/// @param source          the originating event's `source`, `null` when absent
/// @param subject         the originating event's `subject`, `null` when absent
/// @param correlationId   links this delivery to the flow that caused it, `null` when absent
/// @param messageGroup    the ordering group this delivery belongs to, `null` when absent
/// @param clientId        the owning client's id, `null` for a platform-scoped job
/// @param clientCode      the owning client's code, `null` when the job has no client or it did not resolve
/// @param time            when the job was created, `null` when absent
/// @param dataContentType the media type of the job's payload, `null` when the job declared none
/// @param dataJson        the raw JSON text of `data`, `null` when absent
public record Event(
        String id,
        String type,
        int attemptNumber,
        String source,
        String subject,
        String correlationId,
        String messageGroup,
        String clientId,
        String clientCode,
        Instant time,
        String dataContentType,
        String dataJson) {

    public Event {
        Objects.requireNonNull(id, "id");
        Objects.requireNonNull(type, "type");
    }
}
