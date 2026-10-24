package io.flowcatalyst.sdk.resources;

import io.flowcatalyst.sdk.generated.model.CreateIngestionRequest;
import io.flowcatalyst.sdk.generated.model.IngestionFailureListResponse;
import io.flowcatalyst.sdk.generated.model.IngestionHistoryResponse;
import io.flowcatalyst.sdk.generated.model.IngestionListResponse;
import io.flowcatalyst.sdk.generated.model.IngestionResponse;
import io.flowcatalyst.sdk.generated.model.IngestionSourceKindListResponse;
import io.flowcatalyst.sdk.generated.model.IngestionTestResponse;
import io.flowcatalyst.sdk.generated.model.IngestionVersionListResponse;
import io.flowcatalyst.sdk.generated.model.IngestionVersionResponse;
import io.flowcatalyst.sdk.generated.model.SaveIngestionVersionRequest;
import io.flowcatalyst.sdk.generated.model.TestIngestionRequest;
import io.flowcatalyst.sdk.generated.model.UpdateIngestionRequest;
import io.flowcatalyst.sdk.http.Transport;
import java.util.LinkedHashMap;
import java.util.Map;

/**
 * Ingestions resource: an ingestion takes data from a source, splits it into records and turns
 * each record into a dispatch job. An ingestion belongs to an INTEGRATION application and a client.
 *
 * <p>Definitions hold secret <em>names</em> only; nothing here handles a secret value. Build the
 * {@code source} and {@code split} of a request with {@link IngestionDefinitions}. The runtime
 * operations ({@code /api/ingestion-runtime/v1/*}) are the runtime's own client and are not here.
 *
 * <p>Errors: 400 arrives as {@code HttpStatus} with status 400, 403 {@code Forbidden}, 404
 * {@code NotFound}, 409 {@code Conflict} (code {@code INGESTION_CODE_EXISTS}, a stale
 * {@code expectedVersion}, or {@code INGESTION_SET_OWNED} for an ingestion a set owns).
 */
public final class IngestionsResource {

    private static final String PATH = "/api/ingestions";

    private final Transport transport;

    public IngestionsResource(Transport transport) {
        this.transport = transport;
    }

    /** Optional filters for {@link #list}; {@code clientId} is an id or an identifier. */
    public record ListFilters(
            String applicationCode, String clientId, String q, String status, String kind, String after, Integer limit) {
        public static ListFilters none() {
            return new ListFilters(null, null, null, null, null, null, null);
        }

        /** The page that follows the one whose {@code nextCursor} is given. */
        public static ListFilters page(String after, Integer limit) {
            return new ListFilters(null, null, null, null, null, after, limit);
        }
    }

    /** One page, newest cursor in {@code nextCursor}; pass it as {@code after} for the next page. */
    public IngestionListResponse list(ListFilters filters) {
        Map<String, Object> query = new LinkedHashMap<>();
        if (filters != null) {
            query.put("applicationCode", filters.applicationCode());
            query.put("clientId", filters.clientId());
            query.put("q", filters.q());
            query.put("status", filters.status());
            query.put("kind", filters.kind());
            query.put("after", filters.after());
            query.put("limit", filters.limit());
        }
        return transport.get(PATH, query, IngestionListResponse.class);
    }

    public IngestionResponse create(CreateIngestionRequest data) {
        return transport.post(PATH, data, IngestionResponse.class);
    }

    public IngestionResponse get(String id) {
        return transport.get(path(id), null, IngestionResponse.class);
    }

    /** Change the live definition; {@code expectedVersion} is the live version read (0 when none). */
    public IngestionResponse update(String id, UpdateIngestionRequest data) {
        return transport.put(path(id), data, IngestionResponse.class);
    }

    public void delete(String id) {
        transport.delete(path(id), Void.class);
    }

    public IngestionResponse pause(String id) {
        return transport.post(path(id) + "/pause", null, IngestionResponse.class);
    }

    public IngestionResponse resume(String id) {
        return transport.post(path(id) + "/resume", null, IngestionResponse.class);
    }

    public IngestionVersionListResponse listVersions(String id) {
        return transport.get(path(id) + "/versions", null, IngestionVersionListResponse.class);
    }

    /** Save a new version; it runs only once it is live ({@code live} on the request, or {@link #makeVersionLive}). */
    public IngestionVersionResponse saveVersion(String id, SaveIngestionVersionRequest data) {
        return transport.post(path(id) + "/versions", data, IngestionVersionResponse.class);
    }

    public IngestionResponse makeVersionLive(String id, long version) {
        return transport.post(path(id) + "/versions/" + version + "/live", null, IngestionResponse.class);
    }

    /** The ingestion as a definition file: the YAML text {@code fcdev ingest apply} reads back. */
    public String getDefinition(String id) {
        return transport.get(path(id) + "/definition", null, String.class);
    }

    /** Change history, offset paged. */
    public IngestionHistoryResponse listHistory(String id, Integer limit, Integer offset) {
        Map<String, Object> query = new LinkedHashMap<>();
        query.put("limit", limit);
        query.put("offset", offset);
        return transport.get(path(id) + "/history", query, IngestionHistoryResponse.class);
    }

    /** Failures, cursor paged: pass the previous page's {@code nextCursor} as {@code after}. */
    public IngestionFailureListResponse listFailures(String id, String after, Integer limit) {
        Map<String, Object> query = new LinkedHashMap<>();
        query.put("after", after);
        query.put("limit", limit);
        return transport.get(path(id) + "/failures", query, IngestionFailureListResponse.class);
    }

    /** The source kinds the platform knows plus those the live runtimes report. */
    public IngestionSourceKindListResponse listSourceKinds() {
        return transport.get("/api/ingestion-source-kinds", null, IngestionSourceKindListResponse.class);
    }

    /**
     * Queue a connection test, of a saved ingestion ({@code ingestionId}) or of a definition not
     * saved. The answer is {@code PENDING}; poll {@link #getTest} for the result.
     */
    public IngestionTestResponse testConnection(TestIngestionRequest data) {
        return transport.post("/api/ingestion-tests", data, IngestionTestResponse.class);
    }

    public IngestionTestResponse getTest(String id) {
        return transport.get("/api/ingestion-tests/" + Transport.enc(id), null, IngestionTestResponse.class);
    }

    private static String path(String id) {
        return PATH + "/" + Transport.enc(id);
    }
}
