package tech.flowcatalyst.platform.authentication.idp;

import io.smallrye.config.ConfigMapping;
import io.smallrye.config.WithName;

import java.util.Optional;

/**
 * Configuration properties for Platform IDP (internal Keycloak by default).
 *
 * This is the internal IDP that FlowCatalyst controls for basic platform authentication.
 *
 * Configuration in application.properties:
 * <pre>
 * flowcatalyst.idp.platform.enabled=true
 * flowcatalyst.idp.platform.type=KEYCLOAK
 * flowcatalyst.idp.platform.name=FlowCatalyst Keycloak
 * flowcatalyst.idp.platform.keycloak.admin-url=http://localhost:8080
 * flowcatalyst.idp.platform.keycloak.realm=flowcatalyst
 * flowcatalyst.idp.platform.keycloak.client-id=admin-cli
 * flowcatalyst.idp.platform.keycloak.client-secret=${KEYCLOAK_ADMIN_SECRET}
 * </pre>
 */
@ConfigMapping(prefix = "flowcatalyst.idp.platform")
public interface PlatformIdpProperties {

    /**
     * Whether platform IDP sync is enabled.
     */
    @WithName("enabled")
    boolean enabled();

    /**
     * IDP type (KEYCLOAK, ENTRA, OIDC_GENERIC).
     */
    @WithName("type")
    String type();

    /**
     * Human-readable name for this IDP.
     */
    @WithName("name")
    Optional<String> name();

    /**
     * Keycloak-specific configuration.
     */
    @WithName("keycloak")
    Optional<KeycloakConfig> keycloak();

    /**
     * Entra-specific configuration.
     */
    @WithName("entra")
    Optional<EntraConfig> entra();

    interface KeycloakConfig {
        @WithName("admin-url")
        String adminUrl();

        @WithName("realm")
        String realm();

        @WithName("client-id")
        String clientId();

        @WithName("client-secret")
        String clientSecret();
    }

    interface EntraConfig {
        @WithName("tenant-id")
        String tenantId();

        @WithName("client-id")
        String clientId();

        @WithName("client-secret")
        String clientSecret();

        @WithName("application-object-id")
        String applicationObjectId();
    }
}
