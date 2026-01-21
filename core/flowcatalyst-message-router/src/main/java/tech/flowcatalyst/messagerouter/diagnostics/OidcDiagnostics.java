package tech.flowcatalyst.messagerouter.diagnostics;

import io.quarkus.runtime.StartupEvent;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.event.Observes;
import org.eclipse.microprofile.config.ConfigProvider;

import java.util.logging.Logger;

/**
 * Diagnostic logging to debug OIDC configuration issues.
 * This will show exactly what Quarkus sees for OIDC config at startup.
 */
@ApplicationScoped
public class OidcDiagnostics {

    private static final Logger LOG = Logger.getLogger(OidcDiagnostics.class.getName());

    void logOidcConfig(@Observes StartupEvent event) {
        var config = ConfigProvider.getConfig();

        LOG.info("========== OIDC CONFIGURATION DIAGNOSTICS ==========");

        // Check all OIDC-related properties
        logProperty(config, "quarkus.oidc.enabled");
        logProperty(config, "quarkus.oidc.auth-server-url");
        logProperty(config, "quarkus.oidc.client-id");
        logProperty(config, "quarkus.oidc.credentials.secret");
        logProperty(config, "quarkus.oidc.application-type");
        logProperty(config, "authentication.enabled");
        logProperty(config, "authentication.mode");

        // Check environment variables directly
        LOG.info("Environment variable QUARKUS_OIDC_ENABLED: " + System.getenv("QUARKUS_OIDC_ENABLED"));
        LOG.info("Environment variable QUARKUS_OIDC_AUTH_SERVER_URL: " + System.getenv("QUARKUS_OIDC_AUTH_SERVER_URL"));
        LOG.info("Environment variable QUARKUS_OIDC_CLIENT_ID: " + System.getenv("QUARKUS_OIDC_CLIENT_ID"));
        LOG.info("Environment variable AUTHENTICATION_ENABLED: " + System.getenv("AUTHENTICATION_ENABLED"));
        LOG.info("Environment variable AUTHENTICATION_MODE: " + System.getenv("AUTHENTICATION_MODE"));

        LOG.info("===================================================");
    }

    private void logProperty(org.eclipse.microprofile.config.Config config, String propertyName) {
        var value = config.getOptionalValue(propertyName, String.class);
        if (value.isPresent()) {
            // Mask secrets
            if (propertyName.contains("secret") || propertyName.contains("password")) {
                LOG.info(propertyName + " = ***MASKED*** (length: " + value.get().length() + ")");
            } else {
                LOG.info(propertyName + " = " + value.get());
            }
        } else {
            LOG.info(propertyName + " = <NOT SET>");
        }
    }
}
