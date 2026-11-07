package io.flowcatalyst.ingestor.runtime;

import static io.flowcatalyst.ingestor.runtime.SourceHarness.await;
import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.SecretRef;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.contract.Split;
import io.flowcatalyst.ingestor.extension.Backoff;
import io.flowcatalyst.ingestor.extension.FailureLog;
import java.io.IOException;
import java.net.ServerSocket;
import java.net.URI;
import java.nio.file.Files;
import java.nio.file.Path;
import java.time.Duration;
import java.time.InstantSource;
import java.util.List;
import java.util.Map;
import org.apache.camel.CamelContext;
import org.apache.camel.component.aws2.sqs.Sqs2Endpoint;
import org.apache.camel.component.file.remote.SftpEndpoint;
import org.apache.camel.impl.DefaultCamelContext;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;
import org.junit.jupiter.api.io.TempDir;
import org.slf4j.LoggerFactory;

/**
 * A source that fails poll after poll writes the paced lines and no more (docs/ingestor-failure-pacing.md R3, F12):
 * Camel's own line for each failed poll or each failed exchange is either handed to the runtime's handler or
 * turned down in {@code simplelogger.properties}, where the runtime reports the same failure itself.
 */
@Timeout(value = 60, threadMode = Timeout.ThreadMode.SEPARATE_THREAD)
class SourceLogLinesTest {

  /** No wait beyond the source's own interval, so that many polls fail within the test. */
  private static final Backoff NO_WAIT = new Backoff(Duration.ofMillis(1), Duration.ofMillis(1));

  private static final Source.AwsCredential KEY = new Source.AwsCredential("test", new SecretRef("aws"));

  /** The failure log the runtime writes to, new for each test, so each streak starts here. */
  private final FailureLog failures = FailureLog.warn(LoggerFactory.getLogger(POLL_FAILURES));

  private final Pacing pacing = new Pacing(InstantSource.system(), NO_WAIT, failures);

  static final String POLL_FAILURES = "io.flowcatalyst.ingestor.runtime.PollFailures";

  /** The WARN and ERROR lines that count: every one of Camel's, and the runtime's failure log's about this ingestion. */
  static List<LogCapture.Record> linesAbout(LogCapture log, String ingestionId) {
    return log.records().stream()
        .filter(LogCapture.Record::warnOrError)
        .filter(r -> r.logger().startsWith("org.apache.camel") || (r.logger().equals(POLL_FAILURES) && r.message().contains("ingestion " + ingestionId)))
        .toList();
  }

  private static int closedPort() throws IOException {
    try (ServerSocket socket = new ServerSocket(0)) {
      return socket.getLocalPort();
    }
  }

  @Test
  void anSqsQueueThatCannotBeReachedPollAfterPollWritesOneLineInAll() throws Exception {
    URI aws = URI.create("http://127.0.0.1:" + closedPort());
    Ingestion ingestion = SourceHarness.ingestion("ing_lines_sqs", new Source.Sqs("eu-west-1", "lines", KEY), new Split.None());
    CamelSourceEndpoints real = new CamelSourceEndpoints(SourceHarness.secrets(Map.of("aws", "test")), new MemoryState(), aws);
    // The queue's address is known, so the route starts; every receive is refused.
    SourceEndpoints endpoints =
        (camel, i) -> {
          Sqs2Endpoint endpoint = real.sqsEndpoint(camel, i, (Source.Sqs) i.source());
          endpoint.getConfiguration().setQueueUrl(aws + "/000000000000/lines");
          return endpoint;
        };

    try (LogCapture log = new LogCapture()) {
      CamelContext camel = new DefaultCamelContext();
      camel.start();
      try {
        camel.addRoutes(
            new IngestionRoute(
                ingestion, endpoints, (i, jobs) -> {}, new ExchangeCounters(InstantSource.system()), new SourceHealth(), TraceWindows.disabled(), pacing));
        await(() -> failures.streak("ing_lines_sqs:poll") >= 10);
      } finally {
        camel.stop();
      }

      // Without the runtime's logging settings Sqs2Consumer adds an ERROR with a stack trace for every one of them.
      assertThat(linesAbout(log, "ing_lines_sqs")).as("ten failed polls").singleElement().satisfies(line -> {
        assertThat(line.logger()).isEqualTo(POLL_FAILURES);
        assertThat(line.message()).contains("could not poll its source");
      });
    }
  }

  @Test
  void anSftpHostThatIsDownPollAfterPollWritesOneLineInAll() throws Exception {
    Ingestion ingestion =
        SourceHarness.ingestion(
            "ing_lines_sftp",
            new Source.Sftp("127.0.0.1", closedPort(), "in", "u", new SecretRef("p"), null, null, Duration.ofSeconds(1), true),
            new Split.None());
    CamelSourceEndpoints real = new CamelSourceEndpoints(SourceHarness.secrets(Map.of("p", "pw")), new MemoryState(), null);
    SourceEndpoints endpoints =
        (camel, i) -> {
          SftpEndpoint endpoint = (SftpEndpoint) real.endpoint(camel, i);
          // Its three connect attempts a poll, quicker than a second apart.
          endpoint.setReconnectDelay(10);
          return endpoint;
        };

    try (LogCapture log = new LogCapture()) {
      CamelContext camel = new DefaultCamelContext();
      camel.start();
      try {
        camel.addRoutes(
            new IngestionRoute(
                ingestion, endpoints, (i, jobs) -> {}, new ExchangeCounters(InstantSource.system()), new SourceHealth(), TraceWindows.disabled(), pacing));
        await(() -> failures.streak("ing_lines_sftp:poll") >= 4);
      } finally {
        camel.stop();
      }

      // Without the runtime's logging settings Camel's BackgroundTask adds "The task SftpReconnect does not have
      // more budget to continue running" for every one of them.
      assertThat(linesAbout(log, "ing_lines_sftp")).as("four failed polls").singleElement().satisfies(line -> {
        assertThat(line.logger()).isEqualTo(POLL_FAILURES);
        assertThat(line.message()).contains("could not poll its source: Cannot connect");
      });
    }
  }

  @Test
  void aFileThePlatformDoesNotTakePollAfterPollWritesOneLineInAll(@TempDir Path dir) throws Exception {
    Files.writeString(dir.resolve("orders.csv"), "1,A\n2,B\n");
    // The file component's on-completion is the one the sftp component uses (GenericFileOnCompletion).
    Ingestion ingestion =
        SourceHarness.ingestion(
            "ing_lines_file",
            new Source.Sftp("files.example", 22, "in", "u", new SecretRef("p"), null, null, Duration.ofSeconds(1), true),
            new Split.Csv(false));
    SourceEndpoints endpoints =
        (camel, i) -> camel.getEndpoint("file:" + dir + "?delay=1000&move=${exchangeProperty." + IngestionRoute.FOLDER + "}/${file:name}");
    JobSink held =
        (i, jobs) -> {
          throw new PlatformHeldException("the platform is not taking jobs");
        };

    try (LogCapture log = new LogCapture()) {
      CamelContext camel = new DefaultCamelContext();
      camel.start();
      try {
        camel.addRoutes(
            new IngestionRoute(ingestion, endpoints, held, new ExchangeCounters(InstantSource.system()), new SourceHealth(), TraceWindows.disabled(), pacing));
        await(() -> failures.streak("ing_lines_file:dispatch") >= 4);
      } finally {
        camel.stop();
      }

      assertThat(Files.exists(dir.resolve("orders.csv"))).as("the file stays").isTrue();
      // Without the runtime's logging settings the file component adds "Rollback file strategy" for every one of them.
      assertThat(linesAbout(log, "ing_lines_file")).as("four failed polls").singleElement().satisfies(line -> {
        assertThat(line.logger()).isEqualTo(POLL_FAILURES);
        assertThat(line.message()).contains("the platform did not take a payload's jobs");
      });
    }
  }
}
