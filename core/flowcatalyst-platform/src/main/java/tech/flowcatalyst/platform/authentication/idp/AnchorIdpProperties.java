package tech.flowcatalyst.platform.authentication.idp;

import io.smallrye.config.ConfigMapping;
import io.smallrye.config.WithName;

import java.util.Optional;

/**
 * Configuration properties for Anchor Tenant IDP.
 *
 * This is the anchor tenant's corporate IDP (e.g., their Microsoft Entra).
 * Anchor domain users authenticate via this IDP.
 *
 * Configuration in application.properties:
 * <pre>
 * flowcatalyst.idp.anchor.enabled=true
 * flowcatalyst.idp.anchor.type=ENTRA
 * flowcatalyst.idp.anchor.name=Anchor Corp Entra
 * flowcatalyst.idp.anchor.entra.tenant-id=xxx-xxx-xxx
 * flowcatalyst.idp.anchor.entra.client-id=yyy-yyy-yyy
 * flowcatalyst.idp.anchor.entra.client-secret=${ANCHOR_ENTRA_SECRET}
 * flowcatalyst.idp.anchor.entra.application-object-id=zzz-zzz-zzz
 * </pre>
 */
@ConfigMapping(prefix = "flowcatalyst.idp.anchor")
public interface AnchorIdpProperties {

    /**
     * Whether anchor tenant IDP sync is enabled.
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
