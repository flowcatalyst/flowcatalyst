package io.flowcatalyst.sdk.resources;

import io.flowcatalyst.sdk.generated.model.CreateEnrolmentCodeRequest;
import io.flowcatalyst.sdk.generated.model.FieldDeviceListResponse;
import io.flowcatalyst.sdk.generated.model.FieldDeviceResponse;
import io.flowcatalyst.sdk.generated.model.FieldEnrolmentCodeResponse;
import io.flowcatalyst.sdk.http.Transport;
import java.util.LinkedHashMap;
import java.util.Map;

/**
 * Field devices — enrolled phones and tablets, and the single-use enrolment codes that enrol them
 * ({@code /api/field-devices}). Needs {@code platform:iam:field-device:view|manage}.
 */
public final class FieldDevicesResource {

    private final Transport transport;

    public FieldDevicesResource(Transport transport) {
        this.transport = transport;
    }

    /** Query parameters for {@link #list}; every member is optional. */
    public record Filters(String status, String mode, Integer page, Integer pageSize) {
        public static Filters none() {
            return new Filters(null, null, null, null);
        }
    }

    /**
     * Mints a single-use code (valid 10 minutes, shown once). {@code qrPayload} is the string to
     * render as a QR code. A SHARED code cannot name an assigned worker.
     */
    public FieldEnrolmentCodeResponse createEnrolmentCode(CreateEnrolmentCodeRequest request) {
        return transport.post(
                "/api/field-devices/enrolment-codes", request, FieldEnrolmentCodeResponse.class);
    }

    public FieldDeviceListResponse list(String clientId, Filters filters) {
        Map<String, Object> query = new LinkedHashMap<>();
        query.put("clientId", clientId);
        if (filters != null) {
            query.put("status", filters.status());
            query.put("mode", filters.mode());
            query.put("page", filters.page());
            query.put("pageSize", filters.pageSize());
        }
        return transport.get("/api/field-devices", query, FieldDeviceListResponse.class);
    }

    public FieldDeviceResponse get(String id) {
        return transport.get(path(id), null, FieldDeviceResponse.class);
    }

    /** Suspends the device and revokes its sessions; resumable with {@link #activate}. */
    public void suspend(String id) {
        transport.post(path(id) + "/suspend", null, Void.class);
    }

    public void activate(String id) {
        transport.post(path(id) + "/activate", null, Void.class);
    }

    /** Permanent; revokes the device's sessions. */
    public void revoke(String id) {
        transport.post(path(id) + "/revoke", null, Void.class);
    }

    private static String path(String id) {
        return "/api/field-devices/" + Transport.enc(id);
    }
}
