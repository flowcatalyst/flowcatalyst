package io.flowcatalyst.sdk.resources;

import io.flowcatalyst.sdk.generated.model.ApplyIngestionSetRequest;
import io.flowcatalyst.sdk.generated.model.IngestionSetApplyListResponse;
import io.flowcatalyst.sdk.generated.model.IngestionSetArchiveResponse;
import io.flowcatalyst.sdk.generated.model.IngestionSetListResponse;
import io.flowcatalyst.sdk.generated.model.IngestionSetPlanResponse;
import io.flowcatalyst.sdk.generated.model.IngestionSetReleaseResponse;
import io.flowcatalyst.sdk.http.Transport;
import java.util.LinkedHashMap;
import java.util.Map;

/**
 * Ingestion sets: a named group of ingestions applied together from definition files. A set owns
 * its ingestions (read-only until released); an ingestion it no longer lists is deleted.
 *
 * <p>Listing and applying need access to every client and application. An ingestion owned by
 * another set answers 409 {@code INGESTION_SET_OWNED}.
 */
public final class IngestionSetsResource {

    private static final String PATH = "/api/ingestion-sets";

    private final Transport transport;

    public IngestionSetsResource(Transport transport) {
        this.transport = transport;
    }

    public IngestionSetListResponse list() {
        return transport.get(PATH, null, IngestionSetListResponse.class);
    }

    /**
     * Apply a set. {@code dryRun} true checks and reports the changes and writes nothing; null
     * leaves the parameter off.
     */
    public IngestionSetPlanResponse apply(String name, ApplyIngestionSetRequest data, Boolean dryRun) {
        return transport.put(path(name), dryRunQuery(dryRun), data, IngestionSetPlanResponse.class);
    }

    public IngestionSetApplyListResponse listApplies(String name) {
        return transport.get(path(name) + "/applies", null, IngestionSetApplyListResponse.class);
    }

    public IngestionSetArchiveResponse getApply(String name, String applyId) {
        return transport.get(applyPath(name, applyId), null, IngestionSetArchiveResponse.class);
    }

    /** Apply an archived apply's items again, as a new apply. */
    public IngestionSetPlanResponse reapply(String name, String applyId, Boolean dryRun) {
        return transport.post(
                applyPath(name, applyId) + "/reapply", dryRunQuery(dryRun), null, IngestionSetPlanResponse.class);
    }

    /** End the set's ownership; nothing else changes. */
    public IngestionSetReleaseResponse release(String name) {
        return transport.post(path(name) + "/release", null, IngestionSetReleaseResponse.class);
    }

    private static String path(String name) {
        return PATH + "/" + Transport.enc(name);
    }

    private static String applyPath(String name, String applyId) {
        return path(name) + "/applies/" + Transport.enc(applyId);
    }

    private static Map<String, Object> dryRunQuery(Boolean dryRun) {
        if (dryRun == null) return null;
        Map<String, Object> q = new LinkedHashMap<>();
        q.put("dryRun", dryRun);
        return q;
    }
}
