package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.ingestor.contract.ApiAuth;
import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.contract.Source.ApiPoll.ResultCheck;
import io.flowcatalyst.framework.config.Secret;
import io.flowcatalyst.framework.http.Json;
import io.flowcatalyst.framework.httpclient.ClientResponse;
import io.flowcatalyst.framework.httpclient.HttpClientException;
import io.flowcatalyst.framework.httpclient.RequestBuilder;
import io.flowcatalyst.framework.httpclient.RequestMethod;
import io.flowcatalyst.framework.httpclient.ServiceClient;
import io.flowcatalyst.framework.httpclient.ServiceOptions;
import io.flowcatalyst.framework.httpclient.module.ServiceClients;
import java.net.URI;
import java.net.URLDecoder;
import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.util.ArrayList;
import java.util.Base64;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.function.LongSupplier;
import org.jspecify.annotations.Nullable;
import tools.jackson.core.JacksonException;
import tools.jackson.databind.JsonNode;

/**
 * One request of an API poll, with its login: what the poll's route runs each time, and what
 * "Test connection" runs once. A token (client credentials, or the API's own login) is kept until
 * shortly before it expires; when the API answers {@code 401} or {@code 403} it is dropped and the
 * request sent once more with a new one. Nothing returned or logged carries a secret or a token.
 *
 * <p>Its HTTP clients are opened when first used and given back on {@link #close()}; used again
 * after that, they are opened again.
 */
final class ApiPollClient implements AutoCloseable {

  /** How long a refused login waits before it is tried again. */
  static final Duration AUTH_RETRY = Duration.ofMinutes(5);

  private static final Duration REQUEST_TIMEOUT = Duration.ofSeconds(30);
  private static final long MAX_ANSWER = 50L * 1024 * 1024;
  private static final Duration DEFAULT_TOKEN_LIFETIME = Duration.ofMinutes(5);

  /** What one poll came to. */
  sealed interface Outcome {
    /** What happened, step by step, for a person; no secret or token in it. */
    List<String> steps();
  }

  /**
   * The API answered.
   *
   * @param status its status
   * @param body the answer
   * @param hasResults whether it has results by the poll's check; null when the split decides (a
   *     {@code records} check)
   * @param cursor what the answer moves the next request on to, or null
   * @param steps what happened
   */
  record Answer(int status, String body, @Nullable Boolean hasResults, @Nullable String cursor, List<String> steps) implements Outcome {}

  /**
   * The API, or its token endpoint, refused the credentials, also after a fresh token.
   *
   * @param message why
   * @param steps what happened
   */
  record Refused(String message, List<String> steps) implements Outcome {}

  /**
   * The API could not be asked, or answered with an error.
   *
   * @param message why
   * @param body the start of an error answer, or null
   * @param steps what happened
   */
  record Failed(String message, @Nullable String body, List<String> steps) implements Outcome {}

  private record Token(String headerValue, long expiresAtNanos) {}

  /** A token request the endpoint refused (400, 401, 403): the credentials are wrong. */
  private static final class TokenRefused extends Exception {
    TokenRefused(String message) {
      super(message, null, false, false);
    }
  }

  /** A token request that failed otherwise. */
  private static final class TokenFailed extends Exception {
    TokenFailed(String message) {
      super(message, null, false, false);
    }
  }

  private final Source.ApiPoll source;
  private final ServiceClients clients;
  private final Map<String, ServiceClients.Registration> open = new LinkedHashMap<>();
  private final LongSupplier nanos;
  private final URI apiUrl;
  private final String path;
  private final List<Map.Entry<String, String>> fixedQuery;
  private final @Nullable Secret secret;
  private final String name;
  private final OutboundGuard guard;
  private @Nullable Token token;

  /**
   * A client for one ingestion's API.
   *
   * @param ingestion the ingestion
   * @param source its source
   * @param secrets resolves the secrets the login names; an unset one fails here
   * @param name the start of its clients' names, unique to this client: see {@link #name}
   * @param clients the framework's registry its HTTP clients are registered in
   * @param nanos a monotonic clock, for token expiry
   * @throws UnsupportedIngestionException when the address cannot be used or a secret is not set
   */
  ApiPollClient(
      Ingestion ingestion, Source.ApiPoll source, Secrets secrets, String name, ServiceClients clients, LongSupplier nanos) {
    this(ingestion, source, secrets, name, clients, nanos, OutboundGuard.allowingLoopback());
  }

  /**
   * A client for one ingestion's API whose every request first checks the host it goes to.
   *
   * @param ingestion the ingestion
   * @param source its source
   * @param secrets resolves the secrets the login names; an unset one fails here
   * @param name the start of its clients' names, unique to this client: see {@link #name}
   * @param clients the framework's registry its HTTP clients are registered in
   * @param nanos a monotonic clock, for token expiry
   * @param guard what the API's and the login's hosts may resolve to
   * @throws UnsupportedIngestionException when the address cannot be used or a secret is not set
   */
  ApiPollClient(
      Ingestion ingestion,
      Source.ApiPoll source,
      Secrets secrets,
      String name,
      ServiceClients clients,
      LongSupplier nanos,
      OutboundGuard guard) {
    this.guard = guard;
    this.source = source;
    this.clients = clients;
    this.nanos = nanos;
    this.name = name;
    this.apiUrl = address(source.url(), "url");
    String rawPath = apiUrl.getRawPath();
    this.path = rawPath == null || rawPath.isEmpty() ? "/" : URLDecoder.decode(rawPath, StandardCharsets.UTF_8);
    this.fixedQuery = query(apiUrl.getRawQuery());
    this.secret =
        switch (source.auth()) {
          case ApiAuth.None _ -> null;
          case ApiAuth.Basic basic -> secrets.require(ingestion, basic.password());
          case ApiAuth.Bearer bearer -> secrets.require(ingestion, bearer.token());
          case ApiAuth.ApiKey key -> secrets.require(ingestion, key.value());
          case ApiAuth.ClientCredentials cc -> secrets.require(ingestion, cc.clientSecret());
          case ApiAuth.Login login -> secrets.require(ingestion, login.password());
        };
  }

  /**
   * Asks the API once.
   *
   * @param cursor the cursor to send, or null
   * @return what came of it
   */
  /**
   * A name for an ingestion's clients, as the framework allows them: {@code [a-z][a-z0-9-]*}.
   *
   * @param prefix such as {@code api-poll}
   * @param parts what makes it unique, such as the ingestion's id and version
   * @return the name
   */
  static String name(String prefix, Object... parts) {
    StringBuilder name = new StringBuilder(prefix);
    for (Object part : parts) {
      name.append('-').append(String.valueOf(part).toLowerCase(java.util.Locale.ROOT).replaceAll("[^a-z0-9]+", "-"));
    }
    return name.toString();
  }

  /**
   * Refuses the API's host, and the token or login endpoint's, when one resolves to an address a
   * source may not reach.
   *
   * @throws UnsupportedIngestionException when one does
   */
  void checkHosts() {
    guard.check(apiUrl.getHost(), "the API host");
    switch (source.auth()) {
      case ApiAuth.ClientCredentials cc -> guard.checkUrl(cc.tokenUrl(), "the token endpoint's host");
      case ApiAuth.Login login -> guard.checkUrl(login.url(), "the login endpoint's host");
      default -> {}
    }
  }

  /** Gives back the HTTP clients; the token held is kept. */
  @Override
  public synchronized void close() {
    for (ServiceClients.Registration registration : open.values()) {
      registration.close();
    }
    open.clear();
  }

  synchronized Outcome poll(@Nullable String cursor) {
    List<String> steps = new ArrayList<>();
    try {
      ClientResponse answer = send(cursor, steps);
      if (refusal(answer.status()) && renewable()) {
        steps.add("The API answered " + answer.status() + "; fetching a new token and asking again.");
        token = null;
        answer = send(cursor, steps);
      }
      if (refusal(answer.status())) {
        String message = "the API refused the login: " + answer.status() + " from " + source.method() + " " + path;
        steps.add("Refused: " + answer.status() + ".");
        return new Refused(message, List.copyOf(steps));
      }
      if (answer.status() < 200 || answer.status() > 299) {
        String message = "the API answered " + answer.status() + " to " + source.method() + " " + path;
        steps.add(message + ".");
        return new Failed(message, start(answer.bodyText()), List.copyOf(steps));
      }
      return answered(answer, steps);
    } catch (TokenRefused e) {
      steps.add("Refused: " + e.getMessage() + ".");
      return new Refused(String.valueOf(e.getMessage()), List.copyOf(steps));
    } catch (TokenFailed e) {
      steps.add(e.getMessage() + ".");
      return new Failed(String.valueOf(e.getMessage()), null, List.copyOf(steps));
    } catch (HttpClientException e) {
      String message = "could not ask the API (" + e.kind().name().toLowerCase(java.util.Locale.ROOT) + "): " + e.getMessage();
      steps.add(message + ".");
      return new Failed(message, null, List.copyOf(steps));
    } catch (UnsupportedIngestionException e) {
      String message = String.valueOf(e.getMessage());
      steps.add("Not asked: " + message + ".");
      return new Failed(message, null, List.copyOf(steps));
    }
  }

  private Answer answered(ClientResponse answer, List<String> steps) {
    String body = answer.bodyText();
    steps.add(source.method() + " " + path + " answered " + answer.status() + ", " + size(body) + ".");
    JsonNode json = null;
    if (needsJson()) {
      try {
        json = Json.mapper().readTree(body);
      } catch (JacksonException e) {
        steps.add("The answer is not JSON, so its paths cannot be read.");
      }
    }
    Boolean hasResults =
        switch (source.results()) {
          case ResultCheck.Records _ -> null;
          case ResultCheck.JsonPath check -> {
            JsonNode found = json == null ? null : JsonPaths.at(json, check.path());
            boolean present = JsonPaths.present(found);
            steps.add(present ? "Results: " + check.path() + " " + describe(found) + "." : "No results: " + check.path() + " is empty or absent.");
            yield present;
          }
          case ResultCheck.Header check -> {
            String value = answer.header(check.name());
            boolean present = value != null && (check.equals() == null || check.equals().equalsIgnoreCase(value.trim()));
            steps.add(present ? "Results: the answer carries " + check.name() + "." : "No results: the answer does not carry " + check.name() + (check.equals() == null ? "" : ": " + check.equals()) + ".");
            yield present;
          }
        };
    String next = null;
    Source.ApiPoll.Cursor cursor = source.cursor();
    if (cursor != null && json != null) {
      next = JsonPaths.text(JsonPaths.at(json, cursor.from()));
      steps.add(next == null ? "No cursor at " + cursor.from() + "; the next poll sends the one before." : "Next cursor from " + cursor.from() + ": " + clip(next) + ".");
    }
    return new Answer(answer.status(), body, hasResults, next, List.copyOf(steps));
  }

  private boolean needsJson() {
    return source.cursor() != null || source.results() instanceof ResultCheck.JsonPath;
  }

  private ClientResponse send(@Nullable String cursor, List<String> steps) throws TokenRefused, TokenFailed {
    // Again before each request: what the names resolve to now is what the client connects to.
    checkHosts();
    RequestBuilder request =
        client(name, apiUrl).request(source.method() == Source.ApiPoll.Method.POST ? RequestMethod.POST : RequestMethod.GET, path).timeout(REQUEST_TIMEOUT);
    for (Map.Entry<String, String> q : fixedQuery) {
      request.query(q.getKey(), q.getValue());
    }
    Source.ApiPoll.Cursor configured = source.cursor();
    if (configured != null) {
      String value = cursor != null ? cursor : configured.initial();
      if (value != null) {
        request.query(configured.param(), value);
      }
    }
    for (Source.ApiPoll.Header header : source.headerList()) {
      request.header(header.name(), header.value());
    }
    if (source.method() == Source.ApiPoll.Method.POST && source.body() != null) {
      request.body(source.body().getBytes(StandardCharsets.UTF_8), "application/json");
    }
    switch (source.auth()) {
      case ApiAuth.None _ -> {}
      case ApiAuth.Basic basic -> request.header("Authorization", basic(basic.username(), revealed()));
      case ApiAuth.Bearer _ -> request.header("Authorization", "Bearer " + revealed());
      case ApiAuth.ApiKey key -> request.header(key.header(), revealed());
      case ApiAuth.ClientCredentials cc -> request.header("Authorization", token(steps, () -> clientCredentials(cc, steps)).headerValue());
      case ApiAuth.Login login -> request.header(login.header(), token(steps, () -> login(login, steps)).headerValue());
    }
    return request.send();
  }

  private interface Fetch {
    Token get() throws TokenRefused, TokenFailed;
  }

  private Token token(List<String> steps, Fetch fetch) throws TokenRefused, TokenFailed {
    Token kept = token;
    if (kept != null && nanos.getAsLong() < kept.expiresAtNanos()) {
      steps.add("Using the token already held.");
      return kept;
    }
    Token fresh = fetch.get();
    token = fresh;
    return fresh;
  }

  private Token clientCredentials(ApiAuth.ClientCredentials cc, List<String> steps) throws TokenRefused, TokenFailed {
    URI url = address(cc.tokenUrl(), "tokenUrl");
    Map<String, String> form = new LinkedHashMap<>();
    form.put("grant_type", "client_credentials");
    if (cc.scope() != null) {
      form.put("scope", cc.scope());
    }
    if (cc.audience() != null) {
      form.put("audience", cc.audience());
    }
    RequestBuilder request = client(name + "-token", url).post(pathOf(url)).timeout(REQUEST_TIMEOUT);
    if (cc.credentialsIn() == ApiAuth.ClientCredentials.CredentialsIn.BODY) {
      form.put("client_id", cc.clientId());
      form.put("client_secret", revealed());
    } else {
      request.header(
          "Authorization",
          basic(URLEncoder.encode(cc.clientId(), StandardCharsets.UTF_8), URLEncoder.encode(revealed(), StandardCharsets.UTF_8)));
    }
    JsonNode answer = tokenAnswer(request.form(form), url, steps);
    String value = JsonPaths.text(answer.get("access_token"));
    if (value == null) {
      throw new TokenFailed("the token endpoint's answer has no access_token");
    }
    JsonNode expiresIn = answer.get("expires_in");
    Duration lasts = expiresIn != null && expiresIn.canConvertToLong() ? Duration.ofSeconds(expiresIn.asLong()) : DEFAULT_TOKEN_LIFETIME;
    steps.add("Fetched a token from " + url + ", lasting " + lasts.toSeconds() + " s.");
    return new Token("Bearer " + value, expiry(lasts));
  }

  private Token login(ApiAuth.Login login, List<String> steps) throws TokenRefused, TokenFailed {
    URI url = address(login.url(), "url");
    Map<String, String> fields = new LinkedHashMap<>(login.extras());
    fields.put(login.usernameField(), login.username());
    fields.put(login.passwordField(), revealed());
    RequestBuilder request = client(name + "-login", url).post(pathOf(url)).timeout(REQUEST_TIMEOUT);
    request = login.format() == ApiAuth.Login.Format.JSON ? request.json(fields) : request.form(fields);
    JsonNode answer = tokenAnswer(request, url, steps);
    String value = JsonPaths.text(JsonPaths.at(answer, login.tokenPath()));
    if (value == null) {
      throw new TokenFailed("the login's answer has nothing at " + login.tokenPath());
    }
    Duration lasts = login.lifetime();
    String expiresInPath = login.expiresInPath();
    if (expiresInPath != null) {
      JsonNode seconds = JsonPaths.at(answer, expiresInPath);
      if (seconds != null && seconds.canConvertToLong()) {
        lasts = Duration.ofSeconds(seconds.asLong());
      }
    }
    steps.add("Logged in at " + url + "; the token is kept for " + lasts.toSeconds() + " s.");
    return new Token(login.prefix() + value, expiry(lasts));
  }

  private JsonNode tokenAnswer(RequestBuilder request, URI url, List<String> steps) throws TokenRefused, TokenFailed {
    ClientResponse answer;
    try {
      answer = request.retryable(false).send();
    } catch (HttpClientException e) {
      throw new TokenFailed("could not reach " + url + " (" + e.kind().name().toLowerCase(java.util.Locale.ROOT) + ")");
    }
    if (answer.status() == 400 || refusal(answer.status())) {
      steps.add(url + " answered " + answer.status() + ".");
      throw new TokenRefused(url + " refused the credentials: " + answer.status());
    }
    if (answer.status() < 200 || answer.status() > 299) {
      throw new TokenFailed(url + " answered " + answer.status());
    }
    try {
      return Json.mapper().readTree(answer.bodyText());
    } catch (JacksonException e) {
      throw new TokenFailed(url + " did not answer JSON");
    }
  }

  /** Kept until shortly before it ends: a tenth of its life, at most 30 seconds, early. */
  private long expiry(Duration lasts) {
    long life = Math.max(lasts.toNanos(), 0);
    long margin = Math.min(life / 10, Duration.ofSeconds(30).toNanos());
    return nanos.getAsLong() + life - margin;
  }

  private boolean renewable() {
    return source.auth() instanceof ApiAuth.ClientCredentials || source.auth() instanceof ApiAuth.Login;
  }

  private String revealed() {
    Secret held = secret;
    if (held == null) {
      throw new IllegalStateException("this login has no secret");
    }
    return held.reveal();
  }

  private static boolean refusal(int status) {
    return status == 401 || status == 403;
  }

  private static String basic(String user, String password) {
    return "Basic " + Base64.getEncoder().encodeToString((user + ":" + password).getBytes(StandardCharsets.UTF_8));
  }

  /** The client of that name, opened on first use; each is for one address, so one server. */
  private ServiceClient client(String clientName, URI address) {
    ServiceClients.Registration registration = open.get(clientName);
    if (registration == null) {
      URI base = URI.create(address.getScheme() + "://" + address.getRawAuthority());
      // Registered, so its requests are measured; given back with its series on close.
      registration = clients.registerScoped(ServiceOptions.builder(clientName, base).requestTimeout(REQUEST_TIMEOUT).maxResponseBytes(MAX_ANSWER).build());
      open.put(clientName, registration);
    }
    return registration.client();
  }

  private static URI address(String text, String field) {
    URI url;
    try {
      url = new URI(text);
    } catch (java.net.URISyntaxException e) {
      throw new UnsupportedIngestionException("the " + field + " is not an address");
    }
    if (!("http".equals(url.getScheme()) || "https".equals(url.getScheme())) || url.getHost() == null || url.getRawUserInfo() != null) {
      throw new UnsupportedIngestionException("the " + field + " must be an http or https address without a login in it");
    }
    return url;
  }

  private static String pathOf(URI url) {
    String raw = url.getRawPath();
    return raw == null || raw.isEmpty() ? "/" : URLDecoder.decode(raw, StandardCharsets.UTF_8);
  }

  private static List<Map.Entry<String, String>> query(@Nullable String raw) {
    List<Map.Entry<String, String>> pairs = new ArrayList<>();
    if (raw == null || raw.isEmpty()) {
      return pairs;
    }
    for (String pair : raw.split("&")) {
      if (pair.isEmpty()) {
        continue;
      }
      int eq = pair.indexOf('=');
      String key = URLDecoder.decode(eq < 0 ? pair : pair.substring(0, eq), StandardCharsets.UTF_8);
      String value = eq < 0 ? "" : URLDecoder.decode(pair.substring(eq + 1), StandardCharsets.UTF_8);
      pairs.add(Map.entry(key, value));
    }
    return pairs;
  }

  private static String describe(@Nullable JsonNode node) {
    if (node == null) {
      return "is absent";
    }
    return node.isArray() ? "holds " + node.size() + " entr" + (node.size() == 1 ? "y" : "ies") : "is " + clip(String.valueOf(JsonPaths.text(node)));
  }

  private static String size(String body) {
    int bytes = body.getBytes(StandardCharsets.UTF_8).length;
    return bytes < 1024 ? bytes + " B" : String.format(java.util.Locale.ROOT, "%.1f KB", bytes / 1024.0);
  }

  private static String clip(String text) {
    return text.length() > 80 ? text.substring(0, 80) + "..." : text;
  }

  private static @Nullable String start(String body) {
    return body.isEmpty() ? null : body;
  }
}
