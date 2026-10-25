package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.framework.config.Secret;
import io.flowcatalyst.framework.secrets.SecretStoreException;
import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.SecretRef;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.security.SecureRandom;
import java.time.Duration;
import java.util.Collection;
import java.util.HashSet;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Optional;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;
import java.util.function.Function;
import java.util.function.LongSupplier;
import java.util.regex.Pattern;
import org.jspecify.annotations.Nullable;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * Resolves the secrets ingestions name. A name is read from the platform, for the ingestion that
 * names it: the platform reveals only a secret the ingestion's live definition names, at its client,
 * and audits each reveal. The client is the ingestion's own and is never part of the name, so an
 * ingestion cannot name another client's secret.
 *
 * <p>{@code env:<name>} is the documented fallback for a secret not yet moved to the platform: it
 * reads the runtime's environment variable {@link #variable(String, String)}, {@code
 * INGEST_SECRET_V2_<client>__<name>}, each part written with its letters and digits as they are and
 * every other character as {@code _} and its two hex digits ({@code -} is {@code _2D}, {@code _} is
 * {@code _5F}, {@code .} is {@code _2E}, {@code :} is {@code _3A}), so no two clients and names share
 * a variable. The earlier {@code INGEST_SECRET_<CLIENT>__<NAME>} (upper-cased, every other character
 * {@code _}) is still read when the new one is not set and the old one cannot belong to another
 * client and name: neither part has {@code __} in it or starts or ends with {@code _}.
 *
 * <p>A revealed secret is kept for {@code ttl}, so that a source checking credentials on every
 * request does not make the platform audit a reveal for each one. What each running route was built
 * with is remembered as a salted hash, so {@link #rotated(Ingestion)} can tell when a value has
 * changed; entries of ingestions no longer running and kept values past their time are dropped
 * ({@link #retain(Collection)}), so a secret's plaintext stays in memory no longer than it is used.
 */
final class Secrets {

  private static final Logger LOG = LoggerFactory.getLogger(Secrets.class);

  /** The prefix of a name read from the environment. */
  static final String ENV_PREFIX = "env:";

  /** The prefix of the variables. */
  static final String ENV_VARIABLE = "INGEST_SECRET_V2_";

  /** The prefix of the variables in the earlier form. */
  static final String OLD_ENV_VARIABLE = "INGEST_SECRET_";

  private static final Pattern NOT_ALPHANUMERIC = Pattern.compile("[^A-Za-z0-9]");

  private record Key(String ingestionId, long version, String name) {}

  private record Kept(Secret secret, long expiresNanos) {}

  /** A route: an ingestion's id and version. */
  private record Route(String ingestionId, long version) {}

  private final SecretSource platform;
  private final Function<String, @Nullable String> env;
  private final Duration ttl;
  private final LongSupplier nanos;
  private final Map<Key, Kept> kept = new ConcurrentHashMap<>();
  /** What each route read, by name: the salted hash of the value it was given. */
  private final Map<Route, Map<String, String>> used = new ConcurrentHashMap<>();
  private final Set<String> warned = ConcurrentHashMap.newKeySet();
  private final byte[] salt = new byte[16];

  /**
   * Creates the resolver.
   *
   * @param platform where the platform's secrets are revealed
   * @param env the environment, by variable name; null for a variable that is not set
   * @param ttl how long a revealed secret is kept
   * @param nanos a monotonic clock
   */
  Secrets(SecretSource platform, Function<String, @Nullable String> env, Duration ttl, LongSupplier nanos) {
    this.platform = platform;
    this.env = env;
    this.ttl = ttl;
    this.nanos = nanos;
    new SecureRandom().nextBytes(salt);
  }

  /**
   * Secrets held in memory, by name, for the ingestions of client {@code acme}: for tests.
   *
   * @param values the secrets
   * @return the resolver
   */
  static Secrets of(Map<String, String> values) {
    SecretSource source =
        (ingestion, name) ->
            ingestion.clientIdentifier().equals("acme") ? Optional.ofNullable(values.get(name)) : Optional.empty();
    return new Secrets(source, name -> null, Duration.ZERO, System::nanoTime);
  }

  /**
   * The secret an ingestion names.
   *
   * @param ingestion the ingestion, whose client the secret belongs to
   * @param ref the reference: {@code name}, or {@code env:name}
   * @return the secret
   * @throws UnsupportedIngestionException when there is none, or the platform will not give it to
   *     this runtime for the ingestion; the message says why and holds no value
   * @throws SecretStoreException when the platform cannot be reached
   */
  Secret require(Ingestion ingestion, SecretRef ref) {
    String name = ref.name();
    if (name.startsWith(ENV_PREFIX)) {
      return fromEnv(ingestion, name);
    }
    Key key = new Key(ingestion.id(), ingestion.version(), name);
    Kept cached = kept.get(key);
    Secret secret = cached != null && nanos.getAsLong() - cached.expiresNanos() < 0 ? cached.secret() : reveal(ingestion, name);
    used.computeIfAbsent(new Route(ingestion.id(), ingestion.version()), r -> new ConcurrentHashMap<>())
        .computeIfAbsent(name, n -> fingerprint(secret));
    return secret;
  }

  /**
   * Reads every platform secret the ingestion's route was built with again, from the platform, and
   * says whether any has another value now: the route should be built again to use it. A secret
   * that cannot be read now is taken as unchanged (the route keeps what it has); {@code env:} names
   * are not read again, as the environment does not change while the runtime runs.
   *
   * @param ingestion a running ingestion
   * @return whether a value changed
   */
  boolean rotated(Ingestion ingestion) {
    Route route = new Route(ingestion.id(), ingestion.version());
    Map<String, String> names = used.get(route);
    if (names == null || names.isEmpty()) {
      return false;
    }
    boolean changed = false;
    for (Map.Entry<String, String> each : Map.copyOf(names).entrySet()) {
      Secret now;
      try {
        now = reveal(ingestion, each.getKey());
      } catch (RuntimeException e) {
        LOG.debug("ingestion {}: secret '{}' could not be read again: {}", ingestion.id(), each.getKey(), e.getMessage());
        continue;
      }
      if (!fingerprint(now).equals(each.getValue())) {
        LOG.info("ingestion {}: secret '{}' has changed", ingestion.id(), each.getKey());
        changed = true;
      }
    }
    if (changed) {
      // The route built again records what it reads then.
      used.remove(route);
    }
    return changed;
  }

  /**
   * Forgets what was kept for every route but these: their kept values, and what they were built
   * with; and every kept value past its time.
   *
   * @param running the ingestions whose routes run
   */
  void retain(Collection<Ingestion> running) {
    Set<Route> routes = new HashSet<>();
    for (Ingestion ingestion : running) {
      routes.add(new Route(ingestion.id(), ingestion.version()));
    }
    used.keySet().removeIf(route -> !routes.contains(route));
    long now = nanos.getAsLong();
    kept.entrySet().removeIf(e -> !routes.contains(new Route(e.getKey().ingestionId(), e.getKey().version())) || now - e.getValue().expiresNanos() >= 0);
  }

  /**
   * Forgets what was kept for an ingestion whose route is gone, every version of it.
   *
   * @param ingestionId the ingestion
   */
  void forget(String ingestionId) {
    used.keySet().removeIf(route -> route.ingestionId().equals(ingestionId));
    kept.keySet().removeIf(key -> key.ingestionId().equals(ingestionId));
  }

  /** How many values are kept in memory: for tests. */
  int keptCount() {
    return kept.size();
  }

  private Secret reveal(Ingestion ingestion, String name) {
    Optional<String> value;
    try {
      value = platform.reveal(ingestion, name);
    } catch (InterruptedException e) {
      Thread.currentThread().interrupt();
      throw new SecretStoreException("interrupted while reading secret '" + name + "'", e);
    } catch (RejectedException e) {
      // The platform answered and refused: not a store that is down. Its message holds no value.
      throw new UnsupportedIngestionException("the platform will not reveal secret '" + name + "' for this ingestion: " + e.getMessage());
    } catch (IOException e) {
      throw new SecretStoreException("secret '" + name + "' of client " + ingestion.clientIdentifier() + " cannot be read from the platform: " + e.getMessage(), e);
    }
    Secret secret =
        value
            .map(Secret::of)
            .orElseThrow(() -> new UnsupportedIngestionException("secret '" + name + "' is not set for client " + ingestion.clientIdentifier()));
    if (!ttl.isZero()) {
      kept.put(new Key(ingestion.id(), ingestion.version(), name), new Kept(secret, nanos.getAsLong() + ttl.toNanos()));
    }
    return secret;
  }

  private String fingerprint(Secret secret) {
    byte[] value = secret.reveal().getBytes(StandardCharsets.UTF_8);
    byte[] salted = new byte[salt.length + value.length];
    System.arraycopy(salt, 0, salted, 0, salt.length);
    System.arraycopy(value, 0, salted, salt.length, value.length);
    return Identities.sha256(salted);
  }

  private Secret fromEnv(Ingestion ingestion, String name) {
    String bare = name.substring(ENV_PREFIX.length());
    String variable = variable(ingestion.clientIdentifier(), bare);
    String value = env.apply(variable);
    if (value == null || value.isEmpty()) {
      String old = oldVariable(ingestion.clientIdentifier(), bare);
      if (old != null) {
        value = env.apply(old);
        if (value != null && !value.isEmpty() && warned.add(old)) {
          LOG.warn("secret '{}' of client {} is read from {}, the earlier form; set {} instead", name, ingestion.clientIdentifier(), old, variable);
        }
      }
    }
    if (value == null || value.isEmpty()) {
      throw new UnsupportedIngestionException("secret '" + name + "' is not set for client " + ingestion.clientIdentifier() + " (variable " + variable + ")");
    }
    return Secret.of(value);
  }

  /**
   * The variable an {@code env:} name of a client is read from.
   *
   * @param client the client's identifier
   * @param name the name after {@code env:}
   * @return {@code INGEST_SECRET_V2_<client>__<name>}, each part {@linkplain #encode encoded}
   */
  static String variable(String client, String name) {
    return ENV_VARIABLE + encode(client) + "__" + encode(name);
  }

  /**
   * A part of a variable: letters and digits as they are, any other character as {@code _} and its
   * two (or, past {@code U+00FF}, four) upper-case hex digits. An encoded part never holds {@code
   * __}, so the separator cannot be mistaken.
   *
   * @param part a client's identifier or a name
   * @return the encoded part
   */
  static String encode(String part) {
    StringBuilder out = new StringBuilder(part.length() + 8);
    for (int i = 0; i < part.length(); i++) {
      char c = part.charAt(i);
      if ((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
        out.append(c);
      } else if (c <= 0xff) {
        out.append('_').append(String.format(Locale.ROOT, "%02X", (int) c));
      } else {
        out.append("_U").append(String.format(Locale.ROOT, "%04X", (int) c));
      }
    }
    return out.toString();
  }

  /** The earlier variable, or null when it could belong to another client and name too. */
  static @Nullable String oldVariable(String client, String name) {
    String c = NOT_ALPHANUMERIC.matcher(client).replaceAll("_").toUpperCase(Locale.ROOT);
    String n = NOT_ALPHANUMERIC.matcher(name).replaceAll("_").toUpperCase(Locale.ROOT);
    for (String part : List.of(c, n)) {
      if (part.isEmpty() || part.contains("__") || part.startsWith("_") || part.endsWith("_")) {
        return null;
      }
    }
    return OLD_ENV_VARIABLE + c + "__" + n;
  }
}
