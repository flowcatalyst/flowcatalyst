package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.ingestor.contract.HttpAuth;
import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.RateLimit;
import io.flowcatalyst.ingestor.contract.SecretRef;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.framework.auth.PlatformTokens;
import io.flowcatalyst.framework.auth.Principal;
import io.flowcatalyst.framework.auth.TokenResult;
import io.flowcatalyst.framework.config.Secret;
import io.flowcatalyst.framework.core.MonotonicClock;
import io.flowcatalyst.framework.http.HttpErrors;
import io.flowcatalyst.framework.http.Request;
import io.flowcatalyst.framework.http.Response;
import io.flowcatalyst.framework.http.ratelimit.Decision;
import io.flowcatalyst.framework.http.ratelimit.MemoryRateLimiter;
import io.flowcatalyst.framework.http.ratelimit.RateLimitPolicy;
import io.flowcatalyst.framework.http.ratelimit.RateLimiter;
import io.flowcatalyst.framework.secrets.SecretStoreException;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.util.Base64;
import java.util.List;
import java.util.Map;
import org.apache.camel.Exchange;
import org.apache.camel.ProducerTemplate;
import org.jspecify.annotations.Nullable;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * The listener for every HTTP ingestion: {@code POST /ingest/{client}/{code}}, or {@code POST
 * /ingest/{application}/{client}/{code}} when two applications have an ingestion of that code for the
 * client (the client is its identifier or its platform id). One fixed route serves them all; which
 * ingestions exist behind it changes as the platform's list does.
 *
 * <p>The dispatch jobs are created before the answer is sent: {@code 202} means the platform has
 * them, {@code 503} means it has none or only some and the caller should send the payload again.
 */
final class HttpIngest {

  /** The route pattern. */
  static final String PATTERN = "/ingest/{client}/{code}";

  /** The route pattern that names the application too. */
  static final String PATTERN_WITH_APPLICATION = "/ingest/{application}/{client}/{code}";

  private static final Logger LOG = LoggerFactory.getLogger(HttpIngest.class);

  private static final String BEARER = "Bearer ";

  /** The most ingestions the limiter keeps a bucket for; beyond it, new ones share a bucket. */
  private static final int MAX_INGESTIONS = 100_000;

  private final Reconciler reconciler;
  private final Secrets secrets;
  private final RateLimiter limiter;
  private final ProducerTemplate producer;
  private final PlatformTokens tokens;

  HttpIngest(Reconciler reconciler, Secrets secrets, RateLimiter limiter, ProducerTemplate producer, PlatformTokens tokens) {
    this.tokens = tokens;
    this.reconciler = reconciler;
    this.secrets = secrets;
    this.limiter = limiter;
    this.producer = producer;
  }

  /**
   * The limiter for every HTTP ingestion, keyed by ingestion id. The count is this runtime's own:
   * with several runtimes behind a load balancer each allows the full limit.
   *
   * <p>It is asked here, after the caller has proved who it is, and not as route middleware, which
   * runs first: a caller without the credentials must not be able to use up an ingestion's budget.
   *
   * @param clock the clock the buckets refill by
   * @return the limiter
   */
  static RateLimiter limiter(MonotonicClock clock) {
    // Every request brings its ingestion's own policy; this default is never the one used.
    return MemoryRateLimiter.create("ingest-http", RateLimitPolicy.of(60, Duration.ofMinutes(1)), MAX_INGESTIONS, clock);
  }

  /** The Camel endpoint an HTTP ingestion's route consumes from. */
  static String endpoint(Ingestion ingestion) {
    return "direct:http-" + ingestion.id();
  }

  Response handle(Request request) {
    return handle(request, null);
  }

  Response handleWithApplication(Request request) {
    return handle(request, request.pathParam("application"));
  }

  private Response handle(Request request, @Nullable String application) {
    List<Ingestion> found = reconciler.find(application, request.pathParam("client"), request.pathParam("code"));
    if (found.size() > 1) {
      return HttpErrors.error(409, "AMBIGUOUS", "more than one application has this ingestion; post to " + PATTERN_WITH_APPLICATION);
    }
    if (found.isEmpty() || !(found.getFirst().source() instanceof Source.Http http)) {
      return HttpErrors.notFound("no such ingestion");
    }
    Ingestion ingestion = found.getFirst();
    Response refusal;
    try {
      refusal = refusal(ingestion, http.auth(), request.header("Authorization"));
    } catch (SecretStoreException e) {
      LOG.warn("ingestion {} cannot check credentials: {}", ingestion.id(), e.getMessage());
      return HttpErrors.error(503, "UNAVAILABLE", "credentials cannot be checked right now; send it again");
    }
    if (refusal != null) {
      return refusal;
    }
    if (http.rateLimit() != null) {
      Response limited = limited(ingestion, http.rateLimit());
      if (limited != null) {
        return limited;
      }
    }

    byte[] body = request.bodyBytes();
    Exchange exchange = producer.send(endpoint(ingestion), e -> e.getMessage().setBody(body));
    if (exchange.isFailed()) {
      LOG.warn("ingestion {} could not take a payload", ingestion.id(), exchange.getException());
      return HttpErrors.error(503, "UNAVAILABLE", "the payload was not accepted; send it again");
    }
    if (exchange.getProperty(IngestionRoute.REJECTED) != null) {
      return HttpErrors.error(422, "UNPROCESSABLE", "the payload could not be read; do not send it again unchanged");
    }
    return Response.json(202, Map.of("accepted", true));
  }

  /** The answer for a request over the ingestion's limit, or null when it is within it. */
  private @Nullable Response limited(Ingestion ingestion, RateLimit limit) {
    RateLimitPolicy policy;
    try {
      policy = RateLimitPolicy.of(limit.requests(), limit.per());
    } catch (IllegalArgumentException e) {
      LOG.warn("ingestion {} has a rate limit the runtime cannot apply: {}", ingestion.id(), e.getMessage());
      return HttpErrors.error(503, "UNAVAILABLE", "the ingestion's rate limit cannot be applied");
    }
    // A changed limit applies from the next request; what was used carries over in proportion.
    if (limiter.acquire(ingestion.id(), 1, policy) instanceof Decision.Limited over) {
      long seconds = Math.max(1, over.retryAfter().plusMillis(999).toSeconds());
      return HttpErrors.error(429, "RATE_LIMITED", "too many requests; slow down")
          .header("Retry-After", Long.toString(seconds));
    }
    return null;
  }

  /** Why the caller may not post to the ingestion, or null when it may. */
  private @Nullable Response refusal(Ingestion ingestion, HttpAuth auth, @Nullable String header) {
    if (header == null) {
      return unauthenticated();
    }
    return switch (auth) {
      case HttpAuth.Bearer bearer ->
          header.startsWith(BEARER) && matches(ingestion, bearer.token(), header.substring(BEARER.length())) ? null : unauthenticated();
      case HttpAuth.Basic basic -> {
        if (!header.startsWith("Basic ")) {
          yield unauthenticated();
        }
        String decoded;
        try {
          decoded = new String(Base64.getDecoder().decode(header.substring("Basic ".length())), StandardCharsets.UTF_8);
        } catch (IllegalArgumentException e) {
          yield unauthenticated();
        }
        int colon = decoded.indexOf(':');
        yield colon >= 0
                && decoded.substring(0, colon).equals(basic.username())
                && matches(ingestion, basic.password(), decoded.substring(colon + 1))
            ? null
            : unauthenticated();
      }
      case HttpAuth.FlowCatalystToken platform ->
          header.startsWith(BEARER) ? platformToken(ingestion, platform, header.substring(BEARER.length())) : unauthenticated();
    };
  }

  /**
   * An access token of the platform, for a principal with access to the ingestion's client and to
   * the application it names, and, when it names one, that principal.
   */
  private @Nullable Response platformToken(Ingestion ingestion, HttpAuth.FlowCatalystToken auth, String token) {
    return switch (tokens.verify(token)) {
      case TokenResult.Valid valid -> {
        Principal principal = valid.token().principal();
        String principalId = auth.principalId();
        if (!principal.mayActFor(ingestion.clientId())) {
          yield HttpErrors.forbidden("FORBIDDEN", "the token's principal may not act for this client");
        }
        String application = auth.application();
        if (application == null || !principal.mayUse(application)) {
          yield HttpErrors.forbidden("FORBIDDEN", "the token's principal has no access to this ingestion's application");
        }
        if (principalId != null && !principalId.equals(principal.id().value())) {
          yield HttpErrors.forbidden("FORBIDDEN", "this ingestion accepts another principal");
        }
        yield null;
      }
      case TokenResult.Invalid invalid -> {
        LOG.debug("ingestion {} refused a token: {}", ingestion.id(), invalid.reason());
        yield unauthenticated();
      }
      case TokenResult.Unavailable unavailable -> {
        LOG.warn("the platform's signing keys could not be read: {}", unavailable.reason());
        yield HttpErrors.error(503, "UNAVAILABLE", "tokens cannot be checked right now; send it again");
      }
    };
  }

  private static Response unauthenticated() {
    return HttpErrors.unauthenticated("credentials are required");
  }

  private boolean matches(Ingestion ingestion, SecretRef ref, String presented) {
    Secret expected;
    try {
      expected = secrets.require(ingestion, ref);
    } catch (UnsupportedIngestionException e) {
      LOG.warn("ingestion {} cannot check credentials; refusing the request: {}", ingestion.id(), e.getMessage());
      return false;
    }
    return expected.matches(presented);
  }
}
