package io.flowcatalyst.ingestor.runtime;

import static io.flowcatalyst.ingestor.runtime.SourceHarness.await;
import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.RuntimeReport.Failure;
import io.flowcatalyst.ingestor.contract.RuntimeReport.RouteState;
import io.flowcatalyst.ingestor.contract.SecretRef;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.contract.Split;
import io.flowcatalyst.ingestor.extension.Backoff;
import io.flowcatalyst.ingestor.extension.FailureLog;
import java.lang.reflect.InvocationTargetException;
import java.lang.reflect.Proxy;
import java.net.URI;
import java.time.Duration;
import java.time.InstantSource;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.function.UnaryOperator;
import org.apache.camel.Endpoint;
import org.apache.camel.component.aws2.sqs.Sqs2Endpoint;
import org.jspecify.annotations.Nullable;
import org.junit.jupiter.api.AfterAll;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.condition.EnabledIf;
import org.testcontainers.containers.GenericContainer;
import org.testcontainers.containers.wait.strategy.Wait;
import software.amazon.awssdk.auth.credentials.AwsBasicCredentials;
import software.amazon.awssdk.auth.credentials.StaticCredentialsProvider;
import software.amazon.awssdk.core.sync.RequestBody;
import software.amazon.awssdk.regions.Region;
import software.amazon.awssdk.services.s3.S3Client;
import software.amazon.awssdk.services.s3.model.S3Object;
import software.amazon.awssdk.services.sqs.SqsClient;
import software.amazon.awssdk.services.sqs.model.Message;
import software.amazon.awssdk.services.sqs.model.QueueAttributeName;
import software.amazon.awssdk.services.sqs.model.SqsException;

/** The sqs and s3 sources against a local stand-in for AWS in a container. */
@EnabledIf("io.flowcatalyst.ingestor.runtime.Docker#available")
class AwsIngestTest {

  @SuppressWarnings("resource")
  static final GenericContainer<?> AWS =
      new GenericContainer<>("localstack/localstack:3.8.1")
          .withEnv("SERVICES", "sqs,s3")
          .withExposedPorts(4566)
          .waitingFor(Wait.forLogMessage(".*Ready\\..*", 1).withStartupTimeout(Duration.ofMinutes(3)));

  static URI endpoint;
  static SqsClient sqs;
  static S3Client s3;
  static int run;

  @BeforeAll
  static void startAws() {
    AWS.start();
    endpoint = URI.create("http://" + AWS.getHost() + ":" + AWS.getMappedPort(4566));
    StaticCredentialsProvider credentials = StaticCredentialsProvider.create(AwsBasicCredentials.create("test", "test"));
    sqs = SqsClient.builder().endpointOverride(endpoint).region(Region.EU_WEST_1).credentialsProvider(credentials).build();
    s3 =
        S3Client.builder()
            .endpointOverride(endpoint)
            .region(Region.EU_WEST_1)
            .credentialsProvider(credentials)
            .forcePathStyle(true)
            .build();
  }

  @AfterAll
  static void stopAws() {
    sqs.close();
    s3.close();
    AWS.stop();
  }

  SourceHarness harness;
  String queue;
  String queueUrl;
  String bucket;

  @BeforeEach
  void start() {
    run++;
    queue = "orders-" + run;
    bucket = "acme-orders-" + run;
    // A short visibility timeout, so that a message that was not deleted comes back within the test.
    queueUrl =
        sqs.createQueue(b -> b.queueName(queue).attributes(Map.of(QueueAttributeName.VISIBILITY_TIMEOUT, "2"))).queueUrl();
    s3.createBucket(b -> b.bucket(bucket));
    harness = new SourceHarness(SourceHarness.secrets(Map.of("aws", "test")), endpoint);
  }

  @AfterEach
  void stop() {
    harness.close();
  }

  private static final Source.AwsCredential KEY = new Source.AwsCredential("test", new SecretRef("aws"));

  private Ingestion fromQueue(Split split) {
    return SourceHarness.ingestion("orders", new Source.Sqs("eu-west-1", queue, KEY), split);
  }

  private Ingestion fromBucket(Split split) {
    return SourceHarness.ingestion("files", new Source.S3("eu-west-1", bucket, "in/", Duration.ofSeconds(1), KEY), split);
  }

  private void put(String key, String content) {
    s3.putObject(b -> b.bucket(bucket).key(key), RequestBody.fromString(content));
  }

  private List<String> keys() {
    return s3.listObjectsV2(b -> b.bucket(bucket)).contents().stream().map(S3Object::key).sorted().toList();
  }

  private int messagesLeft() {
    Map<QueueAttributeName, String> counts =
        sqs.getQueueAttributes(
                b ->
                    b.queueUrl(queueUrl)
                        .attributeNames(
                            QueueAttributeName.APPROXIMATE_NUMBER_OF_MESSAGES,
                            QueueAttributeName.APPROXIMATE_NUMBER_OF_MESSAGES_NOT_VISIBLE))
            .attributes();
    return counts.values().stream().mapToInt(Integer::parseInt).sum();
  }

  @Test
  void aQueueMessageBecomesJobsAndIsDeleted() throws Exception {
    sqs.sendMessage(b -> b.queueUrl(queueUrl).messageBody("[{\"id\":1},{\"id\":2},{\"id\":3}]"));
    harness.reconciler.reconcile(List.of(fromQueue(new Split.Json("$"))));
    assertThat(harness.reconciler.states()).extracting(RouteState::status).containsExactly(RouteState.Status.STARTED);

    await(() -> harness.payloads().size() == 3);
    await(() -> messagesLeft() == 0);

    assertThat(harness.payloads()).containsExactly("{\"id\":1}", "{\"id\":2}", "{\"id\":3}");
    Thread.sleep(3000);
    assertThat(harness.payloads()).hasSize(3);
    // Keyed by the queue and the message's id, so a message delivered twice makes its jobs once.
    assertThat(harness.batches.stream().flatMap(List::stream).map(io.flowcatalyst.ingestor.contract.JobRecord::externalId))
        .doesNotContainNull()
        .doesNotHaveDuplicates();
  }

  @Test
  void aQueueMessageThePlatformWouldNotTakeComesBack() throws Exception {
    harness.platformDown = true;
    sqs.sendMessage(b -> b.queueUrl(queueUrl).messageBody("{\"id\":1}"));
    harness.reconciler.reconcile(List.of(fromQueue(new Split.None())));

    await(() -> harness.failedSinceLastAsked() >= 1);
    assertThat(messagesLeft()).isEqualTo(1);

    harness.platformDown = false;
    await(() -> harness.payloads().size() == 1);
    await(() -> messagesLeft() == 0);
  }

  @Test
  void aQueueThatDoesNotExistIsReportedAsFailed() {
    harness.reconciler.reconcile(
        List.of(SourceHarness.ingestion("orders", new Source.Sqs("eu-west-1", "no-such-queue", KEY), new Split.None())));

    assertThat(harness.reconciler.states()).singleElement().satisfies(state -> assertThat(state.status()).isEqualTo(RouteState.Status.FAILED));
  }

  @Test
  void anObjectUnderThePrefixBecomesJobsAndIsMovedAsideAndDeeperOnesAreLeft() throws Exception {
    put("in/orders.csv", "id,sku\n1,A\n2,B\n3,C\n");
    put("in/archive/old.csv", "id,sku\n9,Z\n");
    put("elsewhere.csv", "id,sku\n8,Y\n");
    harness.reconciler.reconcile(List.of(fromBucket(new Split.Csv(true))));
    assertThat(harness.reconciler.states()).extracting(RouteState::status).containsExactly(RouteState.Status.STARTED);

    // A move is a copy then a delete: wait for both, not only the copy.
    await(() -> keys().contains(".done/in/orders.csv") && !keys().contains("in/orders.csv"));

    assertThat(harness.payloads()).containsExactly("1,A", "2,B", "3,C");
    assertThat(keys()).containsExactly(".done/in/orders.csv", "elsewhere.csv", "in/archive/old.csv");
    Thread.sleep(2500);
    assertThat(harness.payloads()).hasSize(3);
    // Keyed by the bucket, the key and the ETag.
    assertThat(harness.batches.stream().flatMap(List::stream).map(io.flowcatalyst.ingestor.contract.JobRecord::externalId))
        .doesNotContainNull()
        .doesNotHaveDuplicates();
  }

  @Test
  void anObjectThePlatformWouldNotTakeStaysAndIsReadAgain() throws Exception {
    harness.platformDown = true;
    put("in/orders.csv", "1,A\n2,B\n");
    harness.reconciler.reconcile(List.of(fromBucket(new Split.Csv(false))));

    await(() -> harness.failedSinceLastAsked() >= 1);
    await(() -> harness.failedSinceLastAsked() >= 1);
    assertThat(keys()).containsExactly("in/orders.csv");

    harness.platformDown = false;
    await(() -> keys().contains(".done/in/orders.csv"));
    assertThat(harness.payloads()).containsExactly("1,A", "2,B");
  }

  @Test
  void anObjectThatCannotBeReadIsMovedToRejected() throws Exception {
    put("in/orders.json", "{\"orders\": [{\"id\":1},{\"id\":");
    harness.reconciler.reconcile(List.of(fromBucket(new Split.Json("$.orders"))));

    await(() -> keys().contains(".rejected/in/orders.json"));

    assertThat(keys()).containsExactly(".rejected/in/orders.json");
    assertThat(harness.batches).isEmpty();
  }

  // B1: the route deletes a message itself, after its jobs are made, and a delete that fails is a paced
  // commit failure; a message received again and again is reported.

  private record Line(String message, @Nullable Throwable cause) {}

  /** A harness whose failures are logged into {@code lines}, paced by {@code backoff}, its sqs endpoints using {@code client}. */
  private static SourceHarness harness(List<Line> lines, Backoff backoff, @Nullable SqsClient client) {
    FailureLog failures = new FailureLog((m, c) -> lines.add(new Line(m, c)), Duration.ofMinutes(1), InstantSource.system());
    UnaryOperator<Endpoint> adjust =
        endpoint -> {
          if (client != null && endpoint instanceof Sqs2Endpoint queue) {
            queue.getConfiguration().setAmazonSQSClient(client);
          }
          return endpoint;
        };
    return new SourceHarness(SourceHarness.secrets(Map.of("aws", "test")), endpoint, new Pacing(InstantSource.system(), backoff, failures), adjust);
  }

  /** The test's own client, but every delete fails as SQS would refuse it; {@code deletes} counts them. */
  private static SqsClient failingDeletes(AtomicInteger deletes) {
    return (SqsClient)
        Proxy.newProxyInstance(
            AwsIngestTest.class.getClassLoader(),
            new Class<?>[] {SqsClient.class},
            (proxy, method, args) -> {
              if (method.getName().equals("deleteMessage")) {
                deletes.incrementAndGet();
                throw SqsException.builder().message("Access to the resource is denied").statusCode(403).build();
              }
              if (method.getName().equals("close")) {
                return null;
              }
              try {
                return method.invoke(sqs, args);
              } catch (InvocationTargetException e) {
                throw e.getCause();
              }
            });
  }

  private static List<Failure> failures(SourceHarness harness) {
    return harness.counters.drain(InstantSource.system().instant()).failures();
  }

  @Test
  void aMessageWhoseDeleteFailsIsReportedAsAFailedCommitKeptAndThePollAfterItWaits() throws Exception {
    AtomicInteger deletes = new AtomicInteger();
    List<Line> lines = new CopyOnWriteArrayList<>();
    // An hour after one failure: a poll that did not wait would receive the message again and try to delete it again.
    try (SourceHarness paced = harness(lines, new Backoff(Duration.ofHours(1), Duration.ofHours(1)), failingDeletes(deletes))) {
      sqs.sendMessage(b -> b.queueUrl(queueUrl).messageBody("{\"id\":1}"));
      paced.reconciler.reconcile(List.of(fromQueue(new Split.None())));

      List<Failure> reported = new ArrayList<>();
      await(() -> reported.addAll(failures(paced)) && !reported.isEmpty());
      assertThat(reported).singleElement().satisfies(f -> {
        assertThat(f.kind()).isEqualTo(Failure.Kind.SOURCE);
        assertThat(f.message())
            .startsWith("read and sent, but could not be deleted from the queue: Access to the resource is denied")
            .endsWith("it will be read again");
      });
      // Its jobs were made before the delete was tried.
      assertThat(paced.payloads()).containsExactly("{\"id\":1}");
      assertThat(paced.health.problem("orders", 1)).contains("could not be deleted from the queue");
      assertThat(lines).singleElement().satisfies(l -> assertThat(l.message()).contains("ingestion orders: read and sent, but could not be deleted"));

      // The message comes back after its 2 s; the poll that would take it waits.
      Thread.sleep(4500);
      assertThat(deletes).hasValue(1);
      assertThat(messagesLeft()).isEqualTo(1);
      assertThat(paced.payloads()).hasSize(1);
    }
  }

  @Test
  void aMessageThatCannotBeReadIsRejectedAndDeletedByTheRoute() throws Exception {
    sqs.sendMessage(b -> b.queueUrl(queueUrl).messageBody("[{\"id\":1},"));
    harness.reconciler.reconcile(List.of(fromQueue(new Split.Json("$"))));

    await(() -> messagesLeft() == 0);
    assertThat(harness.batches).isEmpty();
  }

  /** Receives every message on the queue {@code times} times, making each visible again at once. */
  private void receiveEach(int messages, int times) {
    Map<String, Integer> seen = new HashMap<>();
    while (seen.size() < messages || seen.values().stream().anyMatch(n -> n < times)) {
      for (Message m : sqs.receiveMessage(b -> b.queueUrl(queueUrl).maxNumberOfMessages(10).visibilityTimeout(0).waitTimeSeconds(1)).messages()) {
        seen.merge(m.messageId(), 1, Integer::sum);
      }
    }
  }

  @Test
  void messagesReceivedMoreThanFiveTimesAreReportedOnceForTheIngestionNamingTheQueueAndTheCount() throws Exception {
    for (int i = 1; i <= 3; i++) {
      int id = i;
      sqs.sendMessage(b -> b.queueUrl(queueUrl).messageBody("{\"id\":" + id + "}"));
    }
    receiveEach(3, CamelSourceEndpoints.SQS_RECEIVES_WARN);
    List<Line> lines = new CopyOnWriteArrayList<>();
    try (SourceHarness watched = harness(lines, Backoff.FAILURES, null)) {
      watched.reconciler.reconcile(List.of(fromQueue(new Split.None())));

      await(() -> watched.payloads().size() == 3);
      await(() -> messagesLeft() == 0);

      List<Line> often = lines.stream().filter(l -> l.message().contains("has been received")).toList();
      assertThat(often).as("one line for the ingestion, not one a message").singleElement().satisfies(l -> {
        assertThat(l.message()).contains("ingestion orders").contains("the sqs queue " + queue).containsPattern("has been received ([6-9]|\\d\\d+) times");
        assertThat(l.cause()).isNull();
      });
      assertThat(failures(watched))
          .filteredOn(f -> f.message().contains("has been received"))
          .singleElement()
          .satisfies(f -> assertThat(f.kind()).isEqualTo(Failure.Kind.SOURCE));
      // The messages were taken: nothing is failing.
      assertThat(watched.health.problem("orders", 1)).isNull();
    }
  }

  @Test
  void aQueueMessageIsProcessedOnAVirtualThread() throws Exception {
    sqs.sendMessage(b -> b.queueUrl(queueUrl).messageBody("{\"id\":1}"));
    harness.reconciler.reconcile(List.of(fromQueue(new Split.None())));

    await(() -> harness.payloads().size() == 1);
    assertThat(harness.virtual).isNotEmpty().containsOnly(true);
  }

  @Test
  void anObjectThePlatformDoesNotTakePollAfterPollWritesOneLineInAll() throws Exception {
    FailureLog failures = FailureLog.warn(org.slf4j.LoggerFactory.getLogger(SourceLogLinesTest.POLL_FAILURES));
    put("in/orders.csv", "1,A\n2,B\n");
    try (LogCapture log = new LogCapture()) {
      SourceHarness held =
          new SourceHarness(
              SourceHarness.secrets(Map.of("aws", "test")),
              endpoint,
              new Pacing(InstantSource.system(), new Backoff(Duration.ofMillis(1), Duration.ofMillis(1)), failures),
              UnaryOperator.identity());
      try {
        held.platformHeld = true;
        held.reconciler.reconcile(List.of(SourceHarness.ingestion("lines_s3", new Source.S3("eu-west-1", bucket, "in/", Duration.ofSeconds(1), KEY), new Split.Csv(false))));
        await(() -> failures.streak("lines_s3:dispatch") >= 4);
      } finally {
        held.close();
      }

      assertThat(keys()).containsExactly("in/orders.csv");
      // Without the runtime's logging settings the S3 component adds "Exchange failed, so rolling back", with a stack trace, for each.
      assertThat(SourceLogLinesTest.linesAbout(log, "lines_s3")).as("four failed polls").singleElement().satisfies(line -> {
        assertThat(line.logger()).isEqualTo(SourceLogLinesTest.POLL_FAILURES);
        assertThat(line.message()).contains("the platform did not take a payload's jobs");
      });
    }
  }
}
