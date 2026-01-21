package tech.flowcatalyst.messagerouter.vertx.verticle;

import io.github.resilience4j.circuitbreaker.CircuitBreaker;
import io.github.resilience4j.circuitbreaker.CircuitBreakerConfig;
import io.vertx.core.AbstractVerticle;
import io.vertx.core.eventbus.Message;
import io.vertx.core.json.JsonObject;
import org.jboss.logging.Logger;
import tech.flowcatalyst.messagerouter.vertx.channel.MediatorChannels;
import tech.flowcatalyst.messagerouter.vertx.message.RouterMessages.MediationRequest;
import tech.flowcatalyst.messagerouter.vertx.message.RouterMessages.MediationResult;

import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;

/**
 * Mediator verticle for HTTP delivery with circuit breaker.
 * <p>
 * Uses JDK HttpClient (blocking) - fine on virtual threads.
 * <p>
 * Threading: Virtual Thread (blocking OK)
 */
public class MediatorVerticle extends AbstractVerticle {

    private static final Logger LOG = Logger.getLogger(MediatorVerticle.class);

    private String poolCode;
    private HttpClient httpClient;
    private CircuitBreaker circuitBreaker;

    @Override
    public void start() {
        this.poolCode = config().getString("poolCode");

        // Standard JDK HttpClient - blocking is fine on virtual threads
        this.httpClient = HttpClient.newBuilder()
                .connectTimeout(Duration.ofSeconds(5))
                .build();

        // Resilience4j circuit breaker
        CircuitBreakerConfig cbConfig = CircuitBreakerConfig.custom()
                .failureRateThreshold(50)
                .waitDurationInOpenState(Duration.ofSeconds(30))
                .slidingWindowSize(10)
                .minimumNumberOfCalls(5)
                .permittedNumberOfCallsInHalfOpenState(3)
                .build();

        this.circuitBreaker = CircuitBreaker.of("mediator-" + poolCode, cbConfig);

        // Listen for mediation requests via typed channel
        MediatorChannels.address(poolCode).mediate(vertx).consumer(this::handleRequest);

        LOG.infof("MediatorVerticle [%s] started", poolCode);
    }

    @Override
    public void stop() {
        LOG.infof("MediatorVerticle [%s] stopped", poolCode);
    }

    private void handleRequest(Message<MediationRequest> msg) {
        MediationRequest request = msg.body();

        String endpoint = request.mediationTarget();
        String authToken = request.authToken();
        String messageId = request.id();

        LOG.debugf("Mediating message [%s] to endpoint [%s]", request.sqsMessageId(), endpoint);

        try {
            // Blocking HTTP call wrapped in circuit breaker
            MediationResult result = circuitBreaker.executeSupplier(() -> {
                try {
                    // Build request body
                    JsonObject body = new JsonObject()
                            .put("messageId", messageId)
                            .put("sqsMessageId", request.sqsMessageId())
                            .put("mediationType", request.mediationType() != null ? request.mediationType().name() : "HTTP");

                    HttpRequest httpRequest = HttpRequest.newBuilder()
                            .uri(URI.create(endpoint))
                            .header("Authorization", "Bearer " + authToken)
                            .header("Content-Type", "application/json")
                            .POST(HttpRequest.BodyPublishers.ofString(body.encode()))
                            .timeout(Duration.ofSeconds(30))
                            .build();

                    HttpResponse<String> response = httpClient.send(httpRequest,
                            HttpResponse.BodyHandlers.ofString());

                    return interpretResponse(response);
                } catch (Exception e) {
                    throw new RuntimeException("HTTP call failed: " + e.getMessage(), e);
                }
            });

            msg.reply(result);

        } catch (io.github.resilience4j.circuitbreaker.CallNotPermittedException e) {
            // Circuit breaker is open
            LOG.warnf("Circuit breaker OPEN for pool [%s], message [%s]", poolCode, request.sqsMessageId());
            msg.reply(MediationResult.nack(60, "Circuit breaker open"));

        } catch (Exception e) {
            // Other failure
            LOG.warnf("Mediation failed for message [%s]: %s", request.sqsMessageId(), e.getMessage());
            msg.reply(MediationResult.nack(30, e.getMessage()));
        }
    }

    private MediationResult interpretResponse(HttpResponse<String> response) {
        int status = response.statusCode();
        String body = response.body();

        LOG.debugf("Mediation response: status=%d", status);

        if (status == 200) {
            try {
                JsonObject responseBody = new JsonObject(body);
                boolean ack = responseBody.getBoolean("ack", true);

                if (ack) {
                    return MediationResult.success();
                } else {
                    int delaySeconds = responseBody.getInteger("delaySeconds", 0);
                    return MediationResult.nack(delaySeconds, "Target returned ack=false");
                }
            } catch (Exception e) {
                // Response not JSON or missing fields - treat as success
                return MediationResult.success();
            }
        } else if (status == 429) {
            // Rate limited by target
            String retryAfter = response.headers().firstValue("Retry-After").orElse("60");
            int delay;
            try {
                delay = Integer.parseInt(retryAfter);
            } catch (NumberFormatException e) {
                delay = 60;
            }
            return MediationResult.nack(delay, "Rate limited (429)");
        } else if (status >= 400 && status < 500) {
            // Client error - likely configuration issue, ACK to remove from queue
            LOG.warnf("Client error %d - treating as config error", status);
            return MediationResult.configError("HTTP " + status);
        } else {
            // Server error - retry
            return MediationResult.nack(10, "Server error: " + status);
        }
    }

    // === ACCESSORS FOR MONITORING ===

    public String getPoolCode() {
        return poolCode;
    }

    public CircuitBreaker.State getCircuitBreakerState() {
        return circuitBreaker.getState();
    }

    public void resetCircuitBreaker() {
        circuitBreaker.reset();
    }
}
