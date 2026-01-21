package tech.flowcatalyst.messagerouter.security;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.messagerouter.config.AuthenticationConfig;

import java.util.Base64;
import java.util.logging.Logger;

/**
 * Helper class for BasicAuth credential validation.
 * Validates BasicAuth credentials against configured username/password.
 */
@ApplicationScoped
public class BasicAuthIdentityProvider {

    private static final Logger LOG = Logger.getLogger(BasicAuthIdentityProvider.class.getName());

    @Inject
    AuthenticationConfig authConfig;

    /**
     * Validate BasicAuth credentials
     */
    public boolean validateCredentials(String username, String password) {
        if (!authConfig.isBasicAuthEnabled()) {
            return false;
        }

        String configUsername = authConfig.basicUsername().orElse("");
        String configPassword = authConfig.basicPassword().orElse("");

        boolean valid = username != null && password != null &&
                       username.equals(configUsername) &&
                       password.equals(configPassword);

        if (valid) {
            LOG.fine("BasicAuth authentication successful for user: " + username);
        } else {
            LOG.warning("BasicAuth authentication failed for user: " + username);
        }

        return valid;
    }

    /**
     * Extract BasicAuth credentials from Authorization header
     */
    public static BasicAuthRequest extractBasicAuth(String authHeader) {
        if (authHeader == null || !authHeader.startsWith("Basic ")) {
            return null;
        }

        try {
            String encoded = authHeader.substring(6);
            String decoded = new String(Base64.getDecoder().decode(encoded));
            String[] parts = decoded.split(":", 2);

            if (parts.length == 2) {
                return new BasicAuthRequest(parts[0], parts[1]);
            }
        } catch (IllegalArgumentException e) {
            LOG.warning("Invalid BasicAuth header format");
        }

        return null;
    }
}
