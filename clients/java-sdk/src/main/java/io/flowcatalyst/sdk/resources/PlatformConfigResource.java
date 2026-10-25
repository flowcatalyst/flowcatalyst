package io.flowcatalyst.sdk.resources;

import io.flowcatalyst.sdk.generated.model.ConfigListResponse;
import io.flowcatalyst.sdk.generated.model.ConfigResponse;
import io.flowcatalyst.sdk.generated.model.SecretResponse;
import io.flowcatalyst.sdk.generated.model.SetPropertyRequest;
import io.flowcatalyst.sdk.http.Transport;
import java.time.OffsetDateTime;
import java.util.LinkedHashMap;
import java.util.Map;

/**
 * Platform-config resource: the application-scoped configuration properties the platform stores.
 *
 * <p><b>SECRET properties.</b> {@link #get} and {@link #list} always return the literal {@code
 * "***"} as the value of a property whose {@code valueType} is {@code SECRET}, for every caller.
 * The only way to read a secret's plaintext is {@link #revealSecret}.
 */
public final class PlatformConfigResource {

    private final Transport transport;

    public PlatformConfigResource(Transport transport) {
        this.transport = transport;
    }

    /**
     * The plaintext of a SECRET property. {@link #toString()} masks the value; read it only through
     * {@link #value()} and never log it.
     */
    public record RevealedSecret(
            String applicationCode,
            String section,
            String property,
            String clientId,
            String value,
            OffsetDateTime updatedAt) {

        @Override
        public String toString() {
            return "RevealedSecret[applicationCode=" + applicationCode + ", section=" + section
                    + ", property=" + property + ", clientId=" + clientId
                    + ", value=***, updatedAt=" + updatedAt + "]";
        }
    }

    /** List an application's properties. SECRET values are returned as {@code "***"}. */
    public ConfigListResponse list(String app) {
        return transport.get("/api/platform-config/" + Transport.enc(app), null, ConfigListResponse.class);
    }

    /**
     * Get one property. A SECRET property's value is {@code "***"}; use {@link #revealSecret} for
     * the plaintext.
     *
     * @param clientId optional; selects the client-scoped value
     */
    public ConfigResponse get(String app, String section, String property, String clientId) {
        return transport.get(propertyPath(app, section, property), clientQuery(clientId), ConfigResponse.class);
    }

    /** Set a property (create or update). The response masks a SECRET value as {@code "***"}. */
    public ConfigResponse set(String app, String section, String property, SetPropertyRequest data) {
        return transport.put(propertyPath(app, section, property), data, ConfigResponse.class);
    }

    /** Delete a property. */
    public void delete(String app, String section, String property, String clientId) {
        String path = propertyPath(app, section, property);
        String q = clientId == null ? "" : "?clientId=" + Transport.enc(clientId);
        transport.delete(path + q, Void.class);
    }

    /**
     * Read a SECRET property's plaintext ({@code GET /api/config/{app}/{section}/{property}/secret}).
     *
     * <p>Requires a reveal permission: {@code platform:admin:config:reveal} (a platform
     * administrator), or {@code platform:application-service:config:reveal} (an application's
     * service account, which the platform confines to its own applications and to the clients it
     * is linked to: its explicit clients, else the clients its application is enabled for, plus the
     * application-wide value). This supersedes the wording in CHANGES-0.0.13.md. Every call is
     * audited server-side. Errors: 403
     * {@code Forbidden}; 404 {@code NotFound}; 400 code {@code NOT_A_SECRET} (a PLAIN property, use
     * {@link #get}) arrives as {@code HttpStatus} with status 400.
     *
     * <p>The call is never cached by the SDK and the value is never logged or put in an error
     * message; cache it yourself only with a short TTL.
     *
     * @param clientId optional; selects the client-scoped value
     */
    public RevealedSecret revealSecret(String app, String section, String property, String clientId) {
        SecretResponse r = transport.get(
                propertyPath(app, section, property) + "/secret", clientQuery(clientId), SecretResponse.class);
        return new RevealedSecret(
                r.getApplicationCode(), r.getSection(), r.getProperty(), r.getClientId(), r.getValue(), r.getUpdatedAt());
    }

    private static String propertyPath(String app, String section, String property) {
        return "/api/config/" + Transport.enc(app) + "/" + Transport.enc(section) + "/" + Transport.enc(property);
    }

    private static Map<String, Object> clientQuery(String clientId) {
        if (clientId == null) return null;
        Map<String, Object> q = new LinkedHashMap<>();
        q.put("clientId", clientId);
        return q;
    }
}
