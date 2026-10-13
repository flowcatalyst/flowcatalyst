package io.flowcatalyst.sdk.resources;

import io.flowcatalyst.sdk.http.Transport;
import io.flowcatalyst.sdk.outbox.CreateDispatchJobDto;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;

/**
 * Dispatch-job ingest straight from the client: {@code POST /api/dispatch-jobs/batch}, the
 * endpoint the outbox processor delivers {@code DISPATCH_JOB} rows to. Use it when a job does
 * not have to be atomic with a database write; otherwise write it through
 * {@link io.flowcatalyst.sdk.outbox.OutboxManager#createDispatchJob}. The items are the same
 * {@link CreateDispatchJobDto} batch items the outbox writes.
 *
 * <p>The credential needs {@code platform:messaging:batch:dispatch-jobs-write}. The platform
 * stores a batch whole or not at all: any refusal (an unknown {@code clientCode}, a client the
 * caller does not reach, a code outside the caller's application, an invalid member) rejects
 * every item and arrives as a {@link io.flowcatalyst.sdk.error.FlowCatalystException}.
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
}
