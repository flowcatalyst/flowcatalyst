package tech.flowcatalyst.sdk.client.auth;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.sdk.config.FlowCatalystConfig;
import tech.flowcatalyst.sdk.exception.AuthenticationException;

import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.time.Instant;
import java.util.Map;
import java.util.concurrent.locks.ReentrantLock;

import com.fasterxml.jackson.databind.ObjectMapper;

/**
 * Manages OAuth2 client credentials tokens for FlowCatalyst API authentication.
 */
@ApplicationScoped
public class OidcTokenManager {

    private final FlowCatalystConfig config;
    private final HttpClient httpClient;
    private final ObjectMapper objectMapper;
    private final ReentrantLock lock = new ReentrantLock();

    private String accessToken;
    private Instant expiresAt;

    @Inject
    public OidcTokenManager(FlowCatalystConfig config) {
        this.config = config;
        this.httpClient = HttpClient.newBuilder()
            .connectTimeout(Duration.ofSeconds(10))
            .build();
        this.objectMapper = new ObjectMapper();
    }

    /**
     * Get a valid access token, fetching a new one if necessary.
     */
    public String getAccessToken() {
        lock.lock();
        try {
            if (isTokenValid()) {
                return accessToken;
            }
            return fetchNewToken();
        } finally {
            lock.unlock();
        }
    }

    /**
     * Force refresh the access token.
     */
    public String refreshToken() {
        lock.lock();
        try {
            return fetchNewToken();
        } finally {
            lock.unlock();
        }
    }

    private boolean isTokenValid() {
        return accessToken != null
            && expiresAt != null
            && Instant.now().plusSeconds(30).isBefore(expiresAt);
    }

    private String fetchNewToken() {
        String clientId = config.clientId()
            .orElseThrow(AuthenticationException::missingCredentials);
        String clientSecret = config.clientSecret()
            .orElseThrow(AuthenticationException::missingCredentials);

        String tokenUrl = config.tokenUrl()
            .orElseGet(() -> config.baseUrl() + "/oauth/token");

        String body = String.format(
            "grant_type=client_credentials&client_id=%s&client_secret=%s",
            clientId, clientSecret
        );

        try {
            HttpRequest request = HttpRequest.newBuilder()
                .uri(URI.create(tokenUrl))
                .header("Content-Type", "application/x-www-form-urlencoded")
                .POST(HttpRequest.BodyPublishers.ofString(body))
                .timeout(Duration.ofSeconds(config.http().timeout()))
                .build();

            HttpResponse<String> response = httpClient.send(
                request, HttpResponse.BodyHandlers.ofString()
            );

            if (response.statusCode() != 200) {
                throw AuthenticationException.invalidCredentials();
            }

            @SuppressWarnings("unchecked")
            Map<String, Object> json = objectMapper.readValue(
                response.body(), Map.class
            );

            this.accessToken = (String) json.get("access_token");
            int expiresIn = ((Number) json.getOrDefault("expires_in", 3600)).intValue();
            this.expiresAt = Instant.now().plusSeconds(expiresIn);

            return this.accessToken;
        } catch (AuthenticationException e) {
            throw e;
        } catch (Exception e) {
            throw new AuthenticationException("Failed to fetch access token", e);
        }
    }
}
