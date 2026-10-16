package io.flowcatalyst.sdk.resources;

import io.flowcatalyst.sdk.generated.model.EventRead;
import io.flowcatalyst.sdk.generated.model.EventResponse;
import io.flowcatalyst.sdk.http.Transport;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

/**
 * Reads of events: {@code GET /api/events} and {@code GET /api/events/{id}}.
 *
 * <p>The credential needs {@code platform:application-service:event:view} (an application's
 * service account) or the console's {@code platform:messaging:event:view}. An application
 * service account sees only its own applications' events (the first segment of the event
 * type); anything else answers {@code 404} on a single read, and a filter can only narrow
 * within that. A credential bound to no application is refused with {@code 403}.
 */
public final class EventsResource {

    private final Transport transport;

    public EventsResource(Transport transport) {
        this.transport = transport;
    }

    /**
     * Optional filters for {@link #list}. {@code since}/{@code until} are RFC 3339 timestamps;
     * {@code types} and {@code applications} narrow to any of the listed values; {@code limit}
     * (default 100, at most 1000) and {@code offset} page the newest-first result.
     */
    public record Filters(
            String type,
            String source,
            String subject,
            String clientId,
            String correlationId,
            String since,
            String until,
            List<String> types,
            List<String> applications,
            Integer limit,
            Integer offset) {
        public static Filters none() {
            return new Filters(null, null, null, null, null, null, null, null, null, null, null);
        }

        public static Filters page(int limit, int offset) {
            return new Filters(null, null, null, null, null, null, null, null, null, limit, offset);
        }
    }

    /** Lists events, newest first. */
    public List<EventRead> list(Filters filters) {
        Map<String, Object> query = new LinkedHashMap<>();
        if (filters != null) {
            query.put("type", filters.type());
            query.put("source", filters.source());
            query.put("subject", filters.subject());
            query.put("clientId", filters.clientId());
            query.put("correlationId", filters.correlationId());
            query.put("since", filters.since());
            query.put("until", filters.until());
            query.put("types", csv(filters.types()));
            query.put("applications", csv(filters.applications()));
            query.put("limit", filters.limit());
            query.put("offset", filters.offset());
        }
        List<EventRead> events = transport.get("/api/events", query, transport.listOf(EventRead.class));
        return events == null ? List.of() : events;
    }

    /** One event, with its data and context. */
    public EventResponse get(String id) {
        return transport.get("/api/events/" + Transport.enc(id), null, EventResponse.class);
    }

    /** The platform takes plural filters as one comma-separated value. */
    static String csv(List<String> values) {
        return values == null || values.isEmpty() ? null : String.join(",", values);
    }
}
