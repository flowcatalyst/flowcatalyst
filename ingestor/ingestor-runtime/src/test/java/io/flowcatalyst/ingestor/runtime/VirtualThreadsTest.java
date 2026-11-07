package io.flowcatalyst.ingestor.runtime;

import static io.flowcatalyst.ingestor.runtime.SourceHarness.await;
import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.SecretRef;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.contract.Split;
import java.nio.file.Files;
import java.nio.file.Path;
import java.time.Duration;
import java.time.InstantSource;
import java.util.List;
import java.util.concurrent.CopyOnWriteArrayList;
import org.apache.camel.CamelContext;
import org.apache.camel.impl.DefaultCamelContext;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;
import org.junit.jupiter.api.io.TempDir;

/**
 * An exchange runs on a virtual thread where it is processed, not only in Camel's thread manager: a polled
 * source's poll (here the file component, which the sftp source is built on) consumes and dispatches on one.
 * The database-poll and sqs sources assert the same against real servers (DatabasePollTest, AwsIngestTest).
 */
@Timeout(value = 60, threadMode = Timeout.ThreadMode.SEPARATE_THREAD)
class VirtualThreadsTest {

  @Test
  void aPolledFileIsProcessedOnAVirtualThread(@TempDir Path dir) throws Exception {
    Files.writeString(dir.resolve("orders.csv"), "1,A\n2,B\n");
    Ingestion ingestion =
        SourceHarness.ingestion(
            "ing_virtual",
            new Source.Sftp("files.example", 22, "in", "u", new SecretRef("p"), null, null, Duration.ofSeconds(1), true),
            new Split.Csv(false));
    SourceEndpoints endpoints =
        (camel, i) -> camel.getEndpoint("file:" + dir + "?delay=1000&move=${exchangeProperty." + IngestionRoute.FOLDER + "}/${file:name}");
    List<Boolean> virtual = new CopyOnWriteArrayList<>();
    List<String> threads = new CopyOnWriteArrayList<>();
    JobSink sink =
        (i, jobs) -> {
          virtual.add(Thread.currentThread().isVirtual());
          threads.add(Thread.currentThread().getName());
        };

    CamelContext camel = new DefaultCamelContext();
    camel.start();
    try {
      camel.addRoutes(new IngestionRoute(ingestion, endpoints, sink, new ExchangeCounters(InstantSource.system()), new SourceHealth()));
      await(() -> Files.exists(dir.resolve(".done").resolve("orders.csv")));
    } finally {
      camel.stop();
    }

    assertThat(virtual).as("on %s", threads).isNotEmpty().containsOnly(true);
  }
}
