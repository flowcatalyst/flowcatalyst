package io.flowcatalyst.sdk.resources;

import io.flowcatalyst.sdk.generated.model.AttemptDTO;
import io.flowcatalyst.sdk.generated.model.DispatchJobRead;
import io.flowcatalyst.sdk.generated.model.DispatchJobResponse;
import io.flowcatalyst.sdk.http.Transport;
import io.flowcatalyst.sdk.outbox.CreateDispatchJobDto;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

/**
 * Dispatch jobs: ingest straight from the client, and reads of them.
 *
 * <p>Ingest: {@code POST /api/dispatch-jobs/batch}, the
 * endpoint the outbox processor delivers {@code DISPATCH_JOB} rows to. Use it when a job does
 * not have to be atomic with a database write; otherwise write it through
 * {@link io.flowcatalyst.sdk.outbox.OutboxManager#createDispatchJob}. The items are the same
 * {@link CreateDispatchJobDto} batch items the outbox writes.
 *
 * <p>The credential needs {@code platform:messaging:batch:dispatch-jobs-write}. The platform
 * stores a batch whole or not at all: any refusal (an unknown {@code clientCode}, a client the
 * caller does not reach, a code outside the caller's application, an invalid member) rejects
 * every item and arrives as a {@link io.flowcatalyst.sdk.error.FlowCatalystException}.
 *
 * <p>Reads ({@link #list}, {@link #get}, {@link #attempts}, {@link #byEvent}): the credential
 * needs {@code platform:application-service:dispatch-job:view} (an application's service
 * account) or the console's {@code platform:messaging:dispatch-job:view}. An application
 * service account sees only its own applications' jobs (the first segment of the job code, the
 * code of the event that raised it); a job of another application answers {@code 404}, a
 * filter can only narrow, and a credential bound to no application is refused with {@code 403}.
 */
public final class DispatchJobsResource {

    /** At most this many items per call (the platform's limit). */
    public static final int MAX_BATCH = 1000;

    private final Transport transport;

    public DispatchJobsResource(Transport transport) {
        this.transport = transport;
    }

    /** One per-item outcome, positional with the items sent. */
    public record BatchResult(String id, String status, String error) {
        /** {@code SUCCESS} (or {@code SKIPPED}, a duplicate the platform already holds). */
        public boolean succeeded() {
            return "SUCCESS".equals(status) || "SKIPPED".equals(status);
        }
    }

    private record BatchResponse(List<BatchResult> results) {}

    /**
     * Creates the jobs in one call. Every job is validated first ({@link
     * CreateDispatchJobDto#validate()}), so nothing is sent when one is invalid.
     *
     * @return the platform's results, one per job, in order
     * @throws IllegalArgumentException when a job is invalid or there are more than {@link #MAX_BATCH}
     */
    public List<BatchResult> createBatch(List<CreateDispatchJobDto> jobs) {
        if (jobs.isEmpty()) {
            return List.of();
        }
        if (jobs.size() > MAX_BATCH) {
            throw new IllegalArgumentException("at most " + MAX_BATCH + " dispatch jobs per batch");
        }
        List<Map<String, Object>> items = new ArrayList<>(jobs.size());
        for (CreateDispatchJobDto job : jobs) {
            items.add(job.toBatchItem());
        }
        BatchResponse response =
                transport.post("/api/dispatch-jobs/batch", Map.of("items", items), BatchResponse.class);
        return response == null || response.results() == null ? List.of() : response.results();
    }

    // ── reads ────────────────────────────────────────────────────────

    /**
     * Optional filters for {@link #list}. {@code since}/{@code until} are RFC 3339 timestamps;
     * {@code statuses}, {@code codes} and {@code applications} narrow to any of the listed values;
     * {@code limit} (default 100, at most 1000) and {@code offset} page the result, newest first
     * unless {@code sort} is {@code createdAt.asc}.
     */
    public record Filters(
            String status,
            String clientId,
            String subscriptionId,
            String code,
            String messageGroup,
            String since,
            String until,
            List<String> statuses,
            List<String> codes,
            List<String> applications,
            String sort,
            Integer limit,
            Integer offset) {
        public static Filters none() {
            return new Filters(null, null, null, null, null, null, null, null, null, null, null, null, null);
        }

        public static Filters page(int limit, int offset) {
            return new Filters(null, null, null, null, null, null, null, null, null, null, null, limit, offset);
        }
    }

    /** Lists dispatch jobs (the slim list shape). */
    public List<DispatchJobRead> list(Filters filters) {
        Map<String, Object> query = new LinkedHashMap<>();
        if (filters != null) {
            query.put("status", filters.status());
            query.put("clientId", filters.clientId());
            query.put("subscriptionId", filters.subscriptionId());
            query.put("code", filters.code());
            query.put("messageGroup", filters.messageGroup());
            query.put("since", filters.since());
            query.put("until", filters.until());
            query.put("statuses", EventsResource.csv(filters.statuses()));
            query.put("codes", EventsResource.csv(filters.codes()));
            query.put("applications", EventsResource.csv(filters.applications()));
            query.put("sort", filters.sort());
            query.put("limit", filters.limit());
            query.put("offset", filters.offset());
        }
        List<DispatchJobRead> jobs =
                transport.get("/api/dispatch-jobs", query, transport.listOf(DispatchJobRead.class));
        return jobs == null ? List.of() : jobs;
    }

    /** One dispatch job in full. */
    public DispatchJobResponse get(String id) {
        return transport.get("/api/dispatch-jobs/" + Transport.enc(id), null, DispatchJobResponse.class);
    }

    /** A job's delivery attempts, oldest first. */
    public List<AttemptDTO> attempts(String id) {
        List<AttemptDTO> attempts = transport.get(
                "/api/dispatch-jobs/" + Transport.enc(id) + "/attempts", null,
                transport.listOf(AttemptDTO.class));
        return attempts == null ? List.of() : attempts;
    }

    /** The jobs one event raised (only the caller's own applications' jobs, for a scoped account). */
    public List<DispatchJobRead> byEvent(String eventId) {
        List<DispatchJobRead> jobs = transport.get(
                "/api/dispatch-jobs/event/" + Transport.enc(eventId), null,
                transport.listOf(DispatchJobRead.class));
        return jobs == null ? List.of() : jobs;
    }
}
