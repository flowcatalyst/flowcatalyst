package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.ingestor.contract.HttpAuth;
import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.RateLimit;
import io.flowcatalyst.ingestor.contract.SecretRef;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.extension.FailureLog;
import io.flowcatalyst.ingestor.contract.Split;
import io.flowcatalyst.framework.auth.PlatformTokens;
import io.flowcatalyst.framework.auth.Principal;
import io.flowcatalyst.framework.auth.TokenResult;
import io.flowcatalyst.framework.config.Secret;
import io.flowcatalyst.framework.core.MonotonicClock;
import io.flowcatalyst.framework.http.HttpErrors;
import io.flowcatalyst.framework.http.Json;
import io.flowcatalyst.framework.http.Request;
import io.flowcatalyst.framework.http.Response;
import io.flowcatalyst.framework.http.ratelimit.Decision;
import io.flowcatalyst.framework.http.ratelimit.MemoryRateLimiter;
import io.flowcatalyst.framework.http.ratelimit.RateLimitPolicy;
import io.flowcatalyst.framework.http.ratelimit.RateLimiter;
import io.flowcatalyst.framework.secrets.SecretStoreException;
import java.io.ByteArrayInputStream;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.util.Base64;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;
import java.util.regex.Pattern;
import javax.xml.XMLConstants;
import javax.xml.stream.XMLInputFactory;
import javax.xml.stream.XMLStreamConstants;
import javax.xml.stream.XMLStreamException;
import javax.xml.stream.XMLStreamReader;
import org.apache.camel.Exchange;
import org.apache.camel.ProducerTemplate;
import org.jspecify.annotations.Nullable;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import tools.jackson.core.JacksonException;
import tools.jackson.core.JsonParser;
import tools.jackson.core.JsonToken;

/**
 * The listener for every HTTP ingestion: {@code POST /ingest/{client}/{code}}, or {@code POST
 * /ingest/{application}/{client}/{code}} when two applications have an ingestion of that code for the
 * client (the client is its identifier or its platform id). One fixed route serves them all; which
 * ingestions exist behind it changes as the platform's list does.
 *
 * <p>The dispatch jobs are created before the answer is sent: {@code 202} means the platform has
 * them, {@code 503} means it has none or only some and the caller should send the payload again.
 * A caller that sends an {@code Idempotency-Key} header has each record keyed by it and its position,
 * so sending the same request again (after a {@code 503}, say) makes no new jobs; without one a
 * request sent again makes them again. An address that fails to log in to an ingestion more than
 * {@link #AUTH_FAILURES} allows is answered {@code 429} without its credentials being checked, until
 * its wait is over.
 *
 * <p>After the login and the rate limit, and before any job is made, the request is checked against
 * the ingestion's {@code accept} ({@code 415} for a media type it does not take, {@code 400} for a
 * body that is empty, not the JSON or XML it must be), and its {@code X-Message-Group} and {@code
 * X-Correlation-Id} headers, which become every record's group and correlation id. The {@code 202}
 * names what was made: {@code jobId} or, for an ingestion that splits, {@code jobIds} in record order;
 * {@code eventId} or {@code eventIds} when the ingestion's target is an event.
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

  /** The request header a caller names a request with, so that sending it again makes no new jobs. */
  static final String IDEMPOTENCY_KEY = "Idempotency-Key";

  /** The request header a caller names the records' message group with. */
  static final String MESSAGE_GROUP = "X-Message-Group";

  /** The request header a caller names the records' correlation id with. */
  static final String CORRELATION_ID = "X-Correlation-Id";

  private static final Pattern GROUP = Pattern.compile("^[A-Za-z0-9_.:-]{1,128}$");

  /** The longest correlation id a job takes. */
  private static final int MAX_CORRELATION_ID = 100;

  /** Failed logins one address may make to one ingestion before it waits. */
  static final RateLimitPolicy AUTH_FAILURES = RateLimitPolicy.of(20, Duration.ofMinutes(1));

  /** The most callers kept waiting at once; beyond it, new ones are only counted. */
  private static final int MAX_BLOCKED = 10_000;

  private final Reconciler reconciler;
  private final Secrets secrets;
  private final RateLimiter limiter;
  private final ProducerTemplate producer;
  private final PlatformTokens tokens;
  private final MonotonicClock clock;
  private final Map<String, Long> blockedUntil = new ConcurrentHashMap<>();
  private final FailureLog failures;

  HttpIngest(Reconciler reconciler, Secrets secrets, RateLimiter limiter, ProducerTemplate producer, PlatformTokens tokens) {
    this(reconciler, secrets, limiter, producer, tokens, System::nanoTime);
  }

  HttpIngest(
      Reconciler reconciler, Secrets secrets, RateLimiter limiter, ProducerTemplate producer, PlatformTokens tokens, MonotonicClock clock) {
    this(reconciler, secrets, limiter, producer, tokens, clock, Pacing.system().failures());
  }

  HttpIngest(
      Reconciler reconciler,
      Secrets secrets,
      RateLimiter limiter,
      ProducerTemplate producer,
      PlatformTokens tokens,
      MonotonicClock clock,
      FailureLog failures) {
    this.failures = failures;
    this.tokens = tokens;
    this.reconciler = reconciler;
    this.secrets = secrets;
    this.limiter = limiter;
    this.producer = producer;
    this.clock = clock;
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
    String caller = ingestion.id() + "|" + (request.remoteAddress() == null ? "?" : request.remoteAddress());
    Response blocked = blocked(caller);
    if (blocked != null) {
      return blocked;
    }
    Response refusal;
    try {
      refusal = refusal(ingestion, http.auth(), request.header("Authorization"));
    } catch (SecretStoreException e) {
      failures.failed(ingestion.id() + ":credentials", "ingestion " + ingestion.id() + " cannot check credentials: " + e.getMessage(), null);
      return HttpErrors.error(503, "UNAVAILABLE", "credentials cannot be checked right now; send it again");
    }
    if (refusal != null) {
      // Only a refusal the caller caused counts towards the lockout: one the platform caused (a 503 when its
      // keys cannot be read) must not lock callers out during an outage.
      if (refusal.status() >= 500) {
        return refusal;
      }
      Response limited = refused(caller);
      return limited != null ? limited : refusal;
    }
    if (http.rateLimit() != null) {
      Response limited = limited(ingestion, http.rateLimit());
      if (limited != null) {
        return limited;
      }
    }

    Source.Http.Accept accept = http.accept();
    if (accept != null && !accept.types().isEmpty() && !accepted(accept.types(), request.header("Content-Type"))) {
      return bad(415, "UNSUPPORTED_MEDIA_TYPE", "this ingestion takes " + String.join(", ", accept.types()));
    }
    String group = request.header(MESSAGE_GROUP);
    if (group != null && !GROUP.matcher(group).matches()) {
      return bad(400, "INVALID_MESSAGE_GROUP", MESSAGE_GROUP + " must be 1 to 128 letters, digits, and _ . : -");
    }
    String correlation = request.header(CORRELATION_ID);
    if (correlation != null && correlation.isBlank()) {
      correlation = null;
    }
    if (correlation != null && correlation.length() > MAX_CORRELATION_ID) {
      return bad(400, "INVALID_CORRELATION_ID", CORRELATION_ID + " must be at most " + MAX_CORRELATION_ID + " characters");
    }
    byte[] body = request.bodyBytes();
    if (accept != null) {
      Response invalid = invalid(accept.check(), body);
      if (invalid != null) {
        return invalid;
      }
    }
    String key = request.header(IDEMPOTENCY_KEY);
    String sentGroup = group;
    String sentCorrelation = correlation;
    String sentType = request.header("Content-Type");
    Exchange exchange =
        producer.send(
            endpoint(ingestion),
            e -> {
              e.getMessage().setBody(body);
              if (sentType != null) {
                e.setProperty(IngestionRoute.CONTENT_TYPE, sentType);
              }
              if (sentGroup != null) {
                e.setProperty(IngestionRoute.MESSAGE_GROUP, sentGroup);
              }
              if (sentCorrelation != null) {
                e.setProperty(IngestionRoute.CORRELATION_ID, sentCorrelation);
              }
              if (key != null && !key.isBlank()) {
                // The caller's own key: the same request sent again makes no new jobs.
                e.setProperty(IngestionRoute.IDENTITY, key.strip());
              }
            });
    if (exchange.isFailed()) {
      // A platform that did not take the jobs is the same for every request: no stack trace (R2, R7).
      Throwable why = exchange.getException();
      boolean dispatch = false;
      for (Throwable t = why; t != null; t = t.getCause()) {
        dispatch |= t instanceof DispatchFailedException;
      }
      failures.failed(
          ingestion.id() + ":payload",
          "ingestion " + ingestion.id() + " could not take a payload" + (why == null || why.getMessage() == null ? "" : ": " + ExchangeCounters.clean(why.getMessage())),
          dispatch ? null : why);
      return HttpErrors.error(503, "UNAVAILABLE", "the payload was not accepted; send it again");
    }
    if (Boolean.TRUE.equals(exchange.getProperty(IngestionRoute.REFUSED, Boolean.class))) {
      return HttpErrors.error(422, "REFUSED", "the platform refused the payload's jobs; do not send it again unchanged");
    }
    if (exchange.getProperty(IngestionRoute.REJECTED) != null) {
      return HttpErrors.error(422, "UNPROCESSABLE", "the payload could not be read; do not send it again unchanged");
    }
    failures.recovered(ingestion.id() + ":payload", "ingestion " + ingestion.id() + " takes payloads again");
    return Response.json(202, accepted(ingestion, exchange.getProperty(IngestionRoute.CREATED, IngestionRoute.Created.class)));
  }

  /** The {@code 202} body: {@code accepted}, and the ids the platform made when it said (R1). */
  private static Map<String, Object> accepted(Ingestion ingestion, IngestionRoute.@Nullable Created created) {
    Map<String, Object> answer = new LinkedHashMap<>();
    answer.put("accepted", true);
    if (created == null || created.ids() == null) {
      return answer;
    }
    String noun = created.events() ? "event" : "job";
    if (ingestion.split() instanceof Split.None) {
      // One payload, one record: its id, or none when the payload was blank.
      if (created.ids().size() == 1) {
        answer.put(noun + "Id", created.ids().getFirst());
      }
    } else {
      answer.put(noun + "Ids", created.ids());
    }
    return answer;
  }

  private static Response bad(int status, String code, String message) {
    return Response.json(status, Map.of("error", code, "message", message));
  }

  private static Response bad(int status, String code) {
    return Response.json(status, Map.of("error", code));
  }

  /** Whether the request's media type, its parameters ignored, is one of those allowed. */
  static boolean accepted(List<String> allowed, @Nullable String contentType) {
    if (contentType == null) {
      return false;
    }
    int semicolon = contentType.indexOf(';');
    String type = (semicolon < 0 ? contentType : contentType.substring(0, semicolon)).strip().toLowerCase(Locale.ROOT);
    for (String candidate : allowed) {
      if (candidate.strip().toLowerCase(Locale.ROOT).equals(type)) {
        return true;
      }
    }
    return false;
  }

  /** The refusal for a body that is empty or not what the ingestion takes, or null when it is. Reads the body in place. */
  private static @Nullable Response invalid(Source.Http.Body check, byte[] body) {
    boolean blank = true;
    for (byte b : body) {
      if (b != ' ' && b != '\t' && b != '\n' && b != '\r') {
        blank = false;
        break;
      }
    }
    if (blank) {
      return bad(400, "EMPTY_BODY");
    }
    return switch (check) {
      case NONE -> null;
      case JSON -> json(body, false);
      case JSON_OBJECT -> json(body, true);
      case XML -> xml(body);
    };
  }

  /** Reads the whole body as one JSON value without building it, and checks it is an object with a property when asked. */
  private static @Nullable Response json(byte[] body, boolean object) {
    try (JsonParser parser = Json.mapper().createParser(body)) {
      JsonToken first = parser.nextToken();
      if (first == null) {
        return bad(400, "INVALID_JSON");
      }
      if (object && (first != JsonToken.START_OBJECT || parser.nextToken() != JsonToken.PROPERTY_NAME)) {
        return bad(400, "PAYLOAD_MUST_BE_A_NON_EMPTY_JSON_OBJECT");
      }
      if (object) {
        // Inside the first property; skip on to the end of the object.
        parser.nextToken();
        parser.skipChildren();
        while (parser.nextToken() == JsonToken.PROPERTY_NAME) {
          parser.nextToken();
          parser.skipChildren();
        }
      } else {
        parser.skipChildren();
      }
      // One value only: anything after it is not JSON.
      return parser.nextToken() == null ? null : bad(400, "INVALID_JSON");
    } catch (JacksonException e) {
      return bad(400, "INVALID_JSON");
    }
  }

  /**
   * Reads the whole body as XML with DTDs and external entities off; says where it is not well-formed,
   * never what it says. A DOCTYPE is refused outright: no integration payload needs one, and the body is
   * passed on as it is to a processor whose own XML parser this check cannot vouch for.
   */
  private static @Nullable Response xml(byte[] body) {
    XMLInputFactory factory = XMLInputFactory.newFactory();
    factory.setProperty(XMLInputFactory.SUPPORT_DTD, false);
    factory.setProperty(XMLInputFactory.IS_SUPPORTING_EXTERNAL_ENTITIES, false);
    for (String property : new String[] {XMLConstants.ACCESS_EXTERNAL_DTD, XMLConstants.ACCESS_EXTERNAL_SCHEMA}) {
      try {
        factory.setProperty(property, "");
      } catch (IllegalArgumentException e) {
        // A parser without that property has no such access to close.
      }
    }
    XMLStreamReader reader = null;
    try {
      reader = factory.createXMLStreamReader(new ByteArrayInputStream(body));
      boolean root = false;
      while (reader.hasNext()) {
        int event = reader.next();
        if (event == XMLStreamConstants.DTD) {
          return bad(400, "INVALID_XML", "a DOCTYPE is not accepted");
        }
        root |= event == XMLStreamConstants.START_ELEMENT;
      }
      return root ? null : bad(400, "INVALID_XML", "no root element");
    } catch (XMLStreamException e) {
      var at = e.getLocation();
      String where = at == null || at.getLineNumber() < 0 ? "" : " at line " + at.getLineNumber() + ", column " + at.getColumnNumber();
      return bad(400, "INVALID_XML", "the body is not well-formed XML" + where);
    } finally {
      if (reader != null) {
        try {
          reader.close();
        } catch (XMLStreamException e) {
          // Nothing left to read.
        }
      }
    }
  }

  /**
   * A caller (an ingestion and an address) that has failed to authenticate too often is refused
   * without its credentials being checked, until its wait is over; the others are not held up.
   */
  private @Nullable Response blocked(String caller) {
    Long until = blockedUntil.get(caller);
    if (until == null) {
      return null;
    }
    long left = until - clock.nanos();
    if (left <= 0) {
      blockedUntil.remove(caller, until);
      return null;
    }
    return tooManyFailures(Duration.ofNanos(left));
  }

  /** Counts a failed login; past the limit the caller waits, and is told so instead of 401. */
  private @Nullable Response refused(String caller) {
    if (limiter.acquire("auth|" + caller, 1, AUTH_FAILURES) instanceof Decision.Limited over) {
      if (blockedUntil.size() >= MAX_BLOCKED) {
        long now = clock.nanos();
        blockedUntil.values().removeIf(until -> until - now <= 0);
      }
      if (blockedUntil.size() < MAX_BLOCKED) {
        blockedUntil.put(caller, clock.nanos() + over.retryAfter().toNanos());
      }
      LOG.warn("too many failed logins to an HTTP ingestion from {}; refusing it for {}", caller, over.retryAfter());
      return tooManyFailures(over.retryAfter());
    }
    return null;
  }

  private static Response tooManyFailures(Duration wait) {
    long seconds = Math.max(1, wait.plusMillis(999).toSeconds());
    return HttpErrors.error(429, "RATE_LIMITED", "too many failed logins; wait before trying again").header("Retry-After", Long.toString(seconds));
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
        failures.failed(ingestion.id() + ":signing-keys", "the platform's signing keys could not be read (ingestion " + ingestion.id() + "): " + unavailable.reason(), null);
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
      failures.failed(ingestion.id() + ":secret", "ingestion " + ingestion.id() + " cannot check credentials; refusing the request: " + e.getMessage(), null);
      return false;
    }
    return expected.matches(presented);
  }
}
