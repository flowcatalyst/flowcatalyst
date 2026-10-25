package io.flowcatalyst.ingestor.runtime;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatCode;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.RuntimeReport.RouteState;
import io.flowcatalyst.ingestor.contract.SecretRef;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.contract.Source.Track;
import io.flowcatalyst.ingestor.contract.Split;
import java.net.InetAddress;
import java.time.Duration;
import java.util.List;
import java.util.Map;
import org.apache.camel.CamelContext;
import org.apache.camel.impl.DefaultCamelContext;
import org.junit.jupiter.api.Test;

/** What a source's host may resolve to. */
class OutboundGuardTest {

  final OutboundGuard strict = OutboundGuard.strict();

  @Test
  void loopbackLinkLocalUnspecifiedAndMetadataAddressesAreRefusedAndPrivateOnesAllowed() throws Exception {
    for (String refused :
        List.of("127.0.0.1", "127.1.2.3", "::1", "169.254.169.254", "169.254.170.2", "fe80::1", "0.0.0.0", "::", "0.1.2.3", "100.100.100.200", "fd00:ec2::254", "::ffff:127.0.0.1", "64:ff9b::a9fe:a9fe", "::127.0.0.1")) {
      assertThat(strict.forbidden(InetAddress.getByName(refused))).as(refused).isNotNull();
    }
    for (String allowed : List.of("10.0.0.5", "172.16.3.4", "192.168.1.10", "fd12:3456::1", "100.64.0.1", "8.8.8.8")) {
      assertThat(strict.forbidden(InetAddress.getByName(allowed))).as(allowed).isNull();
    }
  }

  @Test
  void loopbackIsAllowedOnlyWhereTheRuntimeRunsBesideItsSources() throws Exception {
    assertThat(OutboundGuard.forEnvironment("local").forbidden(InetAddress.getByName("127.0.0.1"))).isNull();
    assertThat(OutboundGuard.forEnvironment("test").forbidden(InetAddress.getByName("::1"))).isNull();
    assertThat(OutboundGuard.forEnvironment("production").forbidden(InetAddress.getByName("127.0.0.1"))).isNotNull();
    // Metadata is refused everywhere.
    assertThat(OutboundGuard.forEnvironment("local").forbidden(InetAddress.getByName("169.254.169.254"))).isNotNull();
  }

  @Test
  void aNameIsCheckedByWhatItResolvesTo() {
    assertThatThrownBy(() -> strict.check("localhost", "the sftp host"))
        .isInstanceOf(UnsupportedIngestionException.class)
        .hasMessageContaining("the sftp host localhost resolves to")
        .hasMessageContaining("loopback");
    // One that does not resolve is left to fail on its own.
    assertThatCode(() -> strict.check("no-such-host.invalid", "the sftp host")).doesNotThrowAnyException();
  }

  @Test
  void theHostsOfAJdbcUrlAreRead() {
    assertThat(OutboundGuard.jdbcHosts("jdbc:postgresql://db.acme:5432/orders?ssl=true")).containsExactly("db.acme");
    assertThat(OutboundGuard.jdbcHosts("jdbc:postgresql://a:5432,b:5433/orders")).containsExactly("a", "b");
    assertThat(OutboundGuard.jdbcHosts("jdbc:sqlserver://sql.acme:1433;databaseName=x")).containsExactly("sql.acme");
    assertThat(OutboundGuard.jdbcHosts("jdbc:mysql://user@[::1]:3306/x")).containsExactly("[::1]");
    assertThat(OutboundGuard.jdbcHosts("jdbc:oracle:thin:@//ora.acme:1521/svc")).containsExactly("ora.acme");
    assertThat(OutboundGuard.jdbcHosts("jdbc:oracle:thin:@ora.acme:1521:sid")).containsExactly("ora.acme");
    assertThat(OutboundGuard.jdbcHosts("jdbc:h2:mem:x")).isEmpty();
    assertThatThrownBy(() -> strict.checkJdbc("jdbc:postgresql://169.254.169.254/x")).isInstanceOf(UnsupportedIngestionException.class);
  }

  @Test
  void aSourceWhoseHostIsRefusedIsReportedFailedWithWhy() {
    CamelContext camel = new DefaultCamelContext();
    camel.start();
    try {
      SourceEndpoints endpoints =
          new CamelSourceEndpoints(SourceHarness.secrets(Map.of("db", "pw", "sftp", "pw", "mq", "pw")), new MemoryState(), null, null, Extensions.none(), strict);
      Reconciler reconciler = new Reconciler(camel, i -> new IngestionRoute(i, endpoints, (ingestion, jobs) -> {}, new ExchangeCounters(java.time.InstantSource.system())));
      Ingestion db =
          SourceHarness.ingestion(
              "db",
              new Source.DatabasePoll("jdbc:postgresql://127.0.0.1:5432/x", "u", new SecretRef("db"), "t", new Track.Watermark("id", Track.Watermark.Type.NUMBER), 10, Duration.ofSeconds(5)),
              new Split.None());
      Ingestion sftp =
          SourceHarness.ingestion(
              "sftp", new Source.Sftp("169.254.169.254", 22, "/out", "u", new SecretRef("sftp"), "host ssh-ed25519 AAAA", null, Duration.ofMinutes(1)), new Split.None());
      Ingestion mq = SourceHarness.ingestion("mq", new Source.RabbitMq("amqp://localhost:5672", "q", "u", new SecretRef("mq")), new Split.None());

      reconciler.reconcile(List.of(db, sftp, mq));

      assertThat(reconciler.states()).extracting(RouteState::status).containsOnly(RouteState.Status.FAILED);
      assertThat(reconciler.states()).extracting(RouteState::error).allSatisfy(error -> assertThat(error).contains("which a source may not reach"));
    } finally {
      camel.stop();
    }
  }
}
