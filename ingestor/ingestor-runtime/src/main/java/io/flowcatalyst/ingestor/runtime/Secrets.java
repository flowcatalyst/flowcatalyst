package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.framework.config.Secret;
import io.flowcatalyst.framework.secrets.SecretStoreException;
import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.SecretRef;
import java.io.IOException;
import java.time.Duration;
import java.util.Locale;
import java.util.Map;
import java.util.Optional;
import java.util.concurrent.ConcurrentHashMap;
import java.util.function.Function;
import java.util.function.LongSupplier;
import java.util.regex.Pattern;
import org.jspecify.annotations.Nullable;

/**
 * Resolves the secrets ingestions name. A name is read from the platform, for the ingestion that
 * names it: the platform reveals only a secret the ingestion's live definition names, at its client,
 * and audits each reveal. The client is the ingestion's own and is never part of the name, so an
 * ingestion cannot name another client's secret.
 *
 * <p>{@code env:<name>} is the documented fallback for a secret not yet moved to the platform: it
 * reads the runtime's environment variable {@code INGEST_SECRET_<CLIENT>__<NAME>}, the client's
 * identifier and the name upper-cased with every character other than a letter or digit as {@code _}.
 *
 * <p>A revealed secret is kept for {@code ttl}, so that a source checking credentials on every
 * request does not make the platform audit a reveal for each one.
 */
final class Secrets {

  /** The prefix of a name read from the environment. */
  static final String ENV_PREFIX = "env:";

  /** The prefix of the variables. */
  static final String ENV_VARIABLE = "INGEST_SECRET_";

  private static final Pattern NOT_ALPHANUMERIC = Pattern.compile("[^A-Za-z0-9]");

  private record Key(String ingestionId, long version, String name) {}

  private record Kept(Secret secret, long expiresNanos) {}

  private final SecretSource platform;
  private final Function<String, @Nullable String> env;
  private final Duration ttl;
  private final LongSupplier nanos;
  private final Map<Key, Kept> kept = new ConcurrentHashMap<>();

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
    if (cached != null && nanos.getAsLong() - cached.expiresNanos() < 0) {
      return cached.secret();
    }
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
      kept.put(key, new Kept(secret, nanos.getAsLong() + ttl.toNanos()));
    }
    return secret;
  }

  private Secret fromEnv(Ingestion ingestion, String name) {
    String variable = ENV_VARIABLE + encoded(ingestion.clientIdentifier()) + "__" + encoded(name.substring(ENV_PREFIX.length()));
    String value = env.apply(variable);
    if (value == null || value.isEmpty()) {
      throw new UnsupportedIngestionException("secret '" + name + "' is not set for client " + ingestion.clientIdentifier());
    }
    return Secret.of(value);
  }

  private static String encoded(String part) {
    return NOT_ALPHANUMERIC.matcher(part).replaceAll("_").toUpperCase(Locale.ROOT);
  }
}
