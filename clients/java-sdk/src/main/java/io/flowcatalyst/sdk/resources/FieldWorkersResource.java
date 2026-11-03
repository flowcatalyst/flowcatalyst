package io.flowcatalyst.sdk.resources;

import io.flowcatalyst.sdk.generated.model.EnsureFieldWorkerRequest;
import io.flowcatalyst.sdk.generated.model.FieldWorkerListResponse;
import io.flowcatalyst.sdk.generated.model.FieldWorkerResponse;
import io.flowcatalyst.sdk.generated.model.FieldWorkerSignInListResponse;
import io.flowcatalyst.sdk.generated.model.GrantFieldWorkerAppRequest;
import io.flowcatalyst.sdk.generated.model.ResetFieldWorkerPinRequest;
import io.flowcatalyst.sdk.generated.model.ResetFieldWorkerPinResponse;
import io.flowcatalyst.sdk.generated.model.UpdateFieldWorkerRequest;
import io.flowcatalyst.sdk.http.Transport;
import java.util.LinkedHashMap;
import java.util.Map;

/**
 * Field workers — the people who sign in on enrolled devices with a PIN
 * ({@code /api/field-workers}). The PIN hash is never exposed. Needs the
 * {@code platform:iam:field-worker:view|manage} permissions and access to the target client.
 */
public final class FieldWorkersResource {

    private final Transport transport;

    public FieldWorkersResource(Transport transport) {
        this.transport = transport;
    }

    /** Query parameters for {@link #list}; every member is optional (null takes the default). */
    public record Filters(String q, String appCode, String status, Integer page, Integer pageSize) {
        public static Filters none() {
            return new Filters(null, null, null, null, null);
        }
    }

    /**
     * Creates the worker for (clientId, workerCode) if absent and grants the listed applications;
     * the response's {@code created} says which happened.
     */
    public FieldWorkerResponse ensure(EnsureFieldWorkerRequest request) {
        return transport.post("/api/field-workers", request, FieldWorkerResponse.class);
    }

    /** Lists the workers of one client. {@code q} is a case-insensitive prefix of name or code. */
    public FieldWorkerListResponse list(String clientId, Filters filters) {
        Map<String, Object> query = new LinkedHashMap<>();
        query.put("clientId", clientId);
        if (filters != null) {
            query.put("q", filters.q());
            query.put("appCode", filters.appCode());
            query.put("status", filters.status());
            query.put("page", filters.page());
            query.put("pageSize", filters.pageSize());
        }
        return transport.get("/api/field-workers", query, FieldWorkerListResponse.class);
    }

    public FieldWorkerResponse get(String id) {
        return transport.get(path(id), null, FieldWorkerResponse.class);
    }

    /** Returns the refreshed worker. */
    public FieldWorkerResponse update(String id, UpdateFieldWorkerRequest request) {
        return transport.put(path(id), request, FieldWorkerResponse.class);
    }

    /** Revokes the worker's sessions, then removes the worker. */
    public void delete(String id) {
        transport.delete(path(id), Void.class);
    }

    /**
     * Resets the PIN. With no PIN in the request (or a null request) the platform generates a
     * temporary one, returned once in {@code temporaryPin}. Revokes every session of the worker.
     */
    public ResetFieldWorkerPinResponse resetPin(String id, ResetFieldWorkerPinRequest request) {
        return transport.post(
                path(id) + "/pin-reset",
                request != null ? request : new ResetFieldWorkerPinRequest(),
                ResetFieldWorkerPinResponse.class);
    }

    /** Clears the soft and hard locks and the failure counter. */
    public void unlock(String id) {
        transport.post(path(id) + "/unlock", null, Void.class);
    }

    /** Disables the worker and revokes their sessions. */
    public void suspend(String id) {
        transport.post(path(id) + "/suspend", null, Void.class);
    }

    public void activate(String id) {
        transport.post(path(id) + "/activate", null, Void.class);
    }

    /** Grants the worker an application (idempotent). */
    public void grantApp(String id, String appCode) {
        transport.post(path(id) + "/apps", new GrantFieldWorkerAppRequest().appCode(appCode), Void.class);
    }

    /** Revokes an application, and that application's sessions (idempotent). */
    public void revokeApp(String id, String appCode) {
        transport.delete(path(id) + "/apps/" + Transport.enc(appCode), Void.class);
    }

    /** Sign-in attempts, newest first. {@code page} is 0-based; either argument may be null. */
    public FieldWorkerSignInListResponse signIns(String id, Integer page, Integer pageSize) {
        Map<String, Object> query = new LinkedHashMap<>();
        query.put("page", page);
        query.put("pageSize", pageSize);
        return transport.get(path(id) + "/sign-ins", query, FieldWorkerSignInListResponse.class);
    }

    private static String path(String id) {
        return "/api/field-workers/" + Transport.enc(id);
    }
}
