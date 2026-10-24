package io.flowcatalyst.ingestor.runtime;

import static io.flowcatalyst.ingestor.runtime.SourceHarness.await;
import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.RuntimeReport.RouteState;
import io.flowcatalyst.ingestor.contract.SecretRef;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.contract.Split;
import java.net.URI;
import java.time.Duration;
import java.util.List;
import java.util.Map;
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
import software.amazon.awssdk.services.sqs.model.QueueAttributeName;

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

    await(() -> keys().contains(".done/in/orders.csv"));

    assertThat(harness.payloads()).containsExactly("1,A", "2,B", "3,C");
    assertThat(keys()).containsExactly(".done/in/orders.csv", "elsewhere.csv", "in/archive/old.csv");
    Thread.sleep(2500);
    assertThat(harness.payloads()).hasSize(3);
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
}
