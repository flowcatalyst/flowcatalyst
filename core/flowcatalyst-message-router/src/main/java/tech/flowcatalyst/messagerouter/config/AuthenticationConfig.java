package tech.flowcatalyst.messagerouter.config;

import io.smallrye.config.ConfigMapping;
import io.smallrye.config.WithDefault;
import jakarta.enterprise.context.ApplicationScoped;

import java.util.Optional;

/**
 * Authentication configuration for the message router.
 * Supports opt-in BasicAuth and OIDC authentication modes.
 */
@ConfigMapping(prefix = "authentication")
@ApplicationScoped
public interface AuthenticationConfig {

    /**
     * Whether authentication is enabled globally.
     * If false, all endpoints are open.
     */
    @WithDefault("false")
    boolean enabled();

    /**
     * Authentication mode: NONE, BASIC, or OIDC.
     * Only used if enabled() is true.
     */
    @WithDefault("NONE")
    String mode();

    /**
     * Username for BasicAuth.
     * Should be set via environment variable AUTH_BASIC_USERNAME in deployment.
     */
    Optional<String> basicUsername();

    /**
     * Password for BasicAuth.
     * Should be set via environment variable AUTH_BASIC_PASSWORD in deployment.
     */
    Optional<String> basicPassword();

    enum AuthMode {
        NONE, BASIC, OIDC
    }

    default AuthMode getAuthMode() {
        if (!enabled()) {
            return AuthMode.NONE;
        }
        try {
            return AuthMode.valueOf(mode().toUpperCase());
        } catch (IllegalArgumentException e) {
            return AuthMode.NONE;
        }
    }

    default boolean isBasicAuthEnabled() {
        return getAuthMode() == AuthMode.BASIC &&
               basicUsername().isPresent() &&
               basicPassword().isPresent();
    }

    default boolean isOidcEnabled() {
        return getAuthMode() == AuthMode.OIDC;
    }
}
