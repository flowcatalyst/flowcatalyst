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

    /**
     * The header that says who a calling service account acts for (for example the application's
     * signed-in user) on set, delete and reveal; the platform records it in the audit row beside
     * the service account. Honoured only from a service account, ignored from anyone else. At most
     * 256 printable characters (no control characters), else {@code 400 INVALID_ON_BEHALF_OF}.
     */
    public static final String ON_BEHALF_OF = "X-On-Behalf-Of";

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

    /**
     * Set a property (create or update). The response masks a SECRET value as {@code "***"}.
     *
     * <p>A SECRET's value is required and the mask is not a value: {@code "***"} is refused ({@code
     * 400 SECRET_VALUE_MASKED}), as is an empty secret ({@code 400 SECRET_VALUE_REQUIRED}). To leave
     * a secret unchanged, do not set it.
     */
    public ConfigResponse set(String app, String section, String property, SetPropertyRequest data) {
        return set(app, section, property, data, null);
    }

    /**
     * {@link #set(String, String, String, SetPropertyRequest)} on behalf of a person.
     *
     * @param onBehalfOf who the calling service account acts for, sent as the {@link #ON_BEHALF_OF}
     *     header; null or blank sends nothing
     */
    public ConfigResponse set(
            String app, String section, String property, SetPropertyRequest data, String onBehalfOf) {
        return transport.exchange(
                "PUT", propertyPath(app, section, property), null, data, actor(onBehalfOf), ConfigResponse.class);
    }

    /** Delete a property (audited; deleting one that is not there is not an error). */
    public void delete(String app, String section, String property, String clientId) {
        delete(app, section, property, clientId, null);
    }

    /**
     * {@link #delete(String, String, String, String)} on behalf of a person.
     *
     * @param onBehalfOf who the calling service account acts for (see {@link #ON_BEHALF_OF}); null
     *     or blank sends nothing
     */
    public void delete(String app, String section, String property, String clientId, String onBehalfOf) {
        transport.exchange(
                "DELETE", propertyPath(app, section, property), clientQuery(clientId), null, actor(onBehalfOf), Void.class);
    }

    /**
     * Read a SECRET property's plaintext ({@code GET /api/config/{app}/{section}/{property}/secret}).
     *
     * <p>Requires a reveal permission: {@code platform:admin:config:reveal} (a platform
     * administrator), or {@code platform:application-service:config:reveal} (an application's
     * service account, which the platform confines to its own applications and to the clients it
     * is linked to: its explicit clients, else the clients its application is enabled for, plus the
     * application-wide value). This supersedes the wording in CHANGES-0.0.13.md. Every call is
     * audited server-side (with the on-behalf-of principal when one is given). Errors: 403
     * {@code Forbidden}; 404 {@code NotFound}; 400 code {@code NOT_A_SECRET} (a PLAIN property, use
     * {@link #get}) arrives as {@code HttpStatus} with status 400.
     *
     * <p>The call is never cached by the SDK and the value is never logged or put in an error
     * message; cache it yourself only with a short TTL.
     *
     * @param clientId optional; selects the client-scoped value
     */
    public RevealedSecret revealSecret(String app, String section, String property, String clientId) {
        return revealSecret(app, section, property, clientId, null);
    }

    /**
     * {@link #revealSecret(String, String, String, String)} on behalf of a person: the reveal's
     * audit row records who the service account acted for.
     *
     * @param onBehalfOf who the calling service account acts for (see {@link #ON_BEHALF_OF}); null
     *     or blank sends nothing
     */
    public RevealedSecret revealSecret(
            String app, String section, String property, String clientId, String onBehalfOf) {
        SecretResponse r = transport.exchange(
                "GET",
                propertyPath(app, section, property) + "/secret",
                clientQuery(clientId),
                null,
                actor(onBehalfOf),
                SecretResponse.class);
        return new RevealedSecret(
                r.getApplicationCode(), r.getSection(), r.getProperty(), r.getClientId(), r.getValue(), r.getUpdatedAt());
    }

    private static Map<String, String> actor(String onBehalfOf) {
        if (onBehalfOf == null || onBehalfOf.isBlank()) return null;
        return Map.of(ON_BEHALF_OF, onBehalfOf.strip());
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
