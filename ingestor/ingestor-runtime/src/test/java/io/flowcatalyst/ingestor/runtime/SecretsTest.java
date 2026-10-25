package io.flowcatalyst.ingestor.runtime;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import io.flowcatalyst.framework.secrets.SecretStoreException;
import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.SecretRef;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.contract.Split;
import java.io.IOException;
import java.time.Duration;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.concurrent.atomic.AtomicLong;
import org.jspecify.annotations.Nullable;
import org.junit.jupiter.api.Test;

/** Secrets: revealed by the platform for the ingestion that names them, kept briefly, with env: as the fallback. */
class SecretsTest {

  final List<String> asked = new ArrayList<>();
  final AtomicLong nanos = new AtomicLong();
  // The earlier form, still read for a client and a name it cannot be mistaken for.
  final Map<String, String> env = Map.of("INGEST_SECRET_ACME__SFTP_PASSWORD", "from-env");
  Optional<String> value = Optional.of("s3cret");
  @Nullable IOException failure;
  final Ingestion ingestion = SourceHarness.ingestion("ing_1", new Source.Sqs("eu-west-1", "q", null), new Split.None());

  Secrets secrets(Duration ttl) {
    SecretSource platform =
        (i, name) -> {
          asked.add(i.id() + "/" + name);
          IOException failed = failure;
          if (failed != null) {
            throw failed;
          }
          return value;
        };
    return new Secrets(platform, env::get, ttl, nanos::get);
  }

  @Test
  void aNameIsRevealedByThePlatformForTheIngestionThatNamesIt() {
    assertThat(secrets(Duration.ZERO).require(ingestion, new SecretRef("sftp-password")).matches("s3cret")).isTrue();
    assertThat(asked).containsExactly("ing_1/sftp-password");
  }

  @Test
  void aRevealedSecretIsKeptForTheTtlSoAPerRequestCheckIsNotAnAuditRowPerRequest() {
    Secrets secrets = secrets(Duration.ofMinutes(1));

    secrets.require(ingestion, new SecretRef("token"));
    secrets.require(ingestion, new SecretRef("token"));
    assertThat(asked).hasSize(1);

    nanos.addAndGet(Duration.ofSeconds(61).toNanos());
    value = Optional.of("rotated");
    assertThat(secrets.require(ingestion, new SecretRef("token")).matches("rotated")).isTrue();
    assertThat(asked).hasSize(2);
  }

  @Test
  void aSecretThePlatformHasNoValueForNamesTheClientAndNeverTheValue() {
    value = Optional.empty();

    assertThatThrownBy(() -> secrets(Duration.ZERO).require(ingestion, new SecretRef("missing")))
        .isInstanceOf(UnsupportedIngestionException.class)
        .hasMessage("secret 'missing' is not set for client acme");
  }

  @Test
  void aPlatformThatCannotBeReachedIsAStoreFailureAndOneThatRefusesIsNot() {
    failure = new IOException("connection refused");
    assertThatThrownBy(() -> secrets(Duration.ZERO).require(ingestion, new SecretRef("token"))).isInstanceOf(SecretStoreException.class);

    failure = new RejectedException("GET /secrets answered 409: LEASE_NOT_HELD, another runtime holds it", 409);
    assertThatThrownBy(() -> secrets(Duration.ZERO).require(ingestion, new SecretRef("token")))
        .isInstanceOf(UnsupportedIngestionException.class)
        .hasMessageContaining("LEASE_NOT_HELD");
  }

  @Test
  void envNamesReadTheRuntimesEnvironmentForTheIngestionsClientAndNeverAskThePlatform() {
    Secrets secrets = secrets(Duration.ZERO);

    assertThat(secrets.require(ingestion, new SecretRef("env:sftp-password")).matches("from-env")).isTrue();
    assertThatThrownBy(() -> secrets.require(ingestion, new SecretRef("env:other")))
        .isInstanceOf(UnsupportedIngestionException.class)
        .hasMessageContaining("INGEST_SECRET_V2_acme__other");
    assertThat(asked).isEmpty();
  }

  @Test
  void theVariableOfAClientAndANameIsNeverAnotherPairsAndTheNewFormIsReadFirst() {
    assertThat(Secrets.variable("acme--eu", "sftp-password")).isEqualTo("INGEST_SECRET_V2_acme_2D_2Deu__sftp_2Dpassword");
    // Under the earlier form these two were the same variable, INGEST_SECRET_A__B__C.
    assertThat(Secrets.variable("a--b", "c")).isNotEqualTo(Secrets.variable("a", "b--c"));
    assertThat(Secrets.oldVariable("a--b", "c")).isNull();
    assertThat(Secrets.oldVariable("a", "b--c")).isNull();
    assertThat(Secrets.oldVariable("acme", "sftp-password")).isEqualTo("INGEST_SECRET_ACME__SFTP_PASSWORD");
    assertThat(Secrets.variable("a_b", "c")).isNotEqualTo(Secrets.variable("a-b", "c"));
    assertThat(Secrets.variable("a", "B")).isNotEqualTo(Secrets.variable("a", "b"));

    Ingestion eu = SourceHarness.ingestion("ing_eu", new Source.Sqs("eu-west-1", "q", null), new Split.None());
    Ingestion euOf = new Ingestion(eu.id(), 1, "clt_eu", "acme--eu", "logistics", "c", "c", eu.source(), eu.split(), eu.target(), false);
    Map<String, String> both = Map.of("INGEST_SECRET_V2_acme__token", "new", "INGEST_SECRET_ACME__TOKEN", "old", "INGEST_SECRET_ACME__EU__TOKEN", "ambiguous");
    Secrets secrets = new Secrets((i, n) -> Optional.empty(), both::get, Duration.ZERO, nanos::get);
    assertThat(secrets.require(ingestion, new SecretRef("env:token")).matches("new")).isTrue();
    // acme--eu's earlier variable could be acme's "eu--token": it is not read.
    assertThatThrownBy(() -> secrets.require(euOf, new SecretRef("env:token"))).isInstanceOf(UnsupportedIngestionException.class);
  }

  @Test
  void aRotatedSecretIsSeenWhenTheRouteIsCheckedAndWhatGoneRoutesKeptIsDropped() {
    Secrets secrets = secrets(Duration.ofMinutes(1));
    secrets.require(ingestion, new SecretRef("token"));
    assertThat(secrets.rotated(ingestion)).isFalse();

    value = Optional.of("rotated");
    assertThat(secrets.rotated(ingestion)).isTrue();
    // Read again for the rebuilt route, from what the check fetched.
    assertThat(secrets.require(ingestion, new SecretRef("token")).matches("rotated")).isTrue();
    assertThat(secrets.rotated(ingestion)).isFalse();

    // A platform that cannot be reached is not a rotation.
    failure = new IOException("connection refused");
    assertThat(secrets.rotated(ingestion)).isFalse();

    assertThat(secrets.keptCount()).isEqualTo(1);
    secrets.retain(List.of());
    assertThat(secrets.keptCount()).isZero();
    assertThat(secrets.rotated(ingestion)).isFalse();
  }

  @Test
  void aKeptValuePastItsTimeIsDroppedFromMemory() {
    Secrets secrets = secrets(Duration.ofMinutes(1));
    secrets.require(ingestion, new SecretRef("token"));
    secrets.retain(List.of(ingestion));
    assertThat(secrets.keptCount()).isEqualTo(1);

    nanos.addAndGet(Duration.ofSeconds(61).toNanos());
    secrets.retain(List.of(ingestion));
    assertThat(secrets.keptCount()).isZero();

    secrets.require(ingestion, new SecretRef("token"));
    secrets.forget(ingestion.id());
    assertThat(secrets.keptCount()).isZero();
  }
}
