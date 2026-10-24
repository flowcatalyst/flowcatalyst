package io.flowcatalyst.ingestor.runtime;

import static io.flowcatalyst.ingestor.runtime.SourceHarness.await;
import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.RuntimeReport.RouteState;
import io.flowcatalyst.ingestor.contract.SecretRef;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.contract.Split;
import com.rabbitmq.client.Channel;
import com.rabbitmq.client.Connection;
import com.rabbitmq.client.ConnectionFactory;
import java.nio.charset.StandardCharsets;
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

/** The rabbitmq source against a real broker in a container. */
@EnabledIf("io.flowcatalyst.ingestor.runtime.Docker#available")
class RabbitMqIngestTest {

  @SuppressWarnings("resource")
  static final GenericContainer<?> BROKER =
      new GenericContainer<>("rabbitmq:4-alpine")
          .withExposedPorts(5672)
          .waitingFor(Wait.forLogMessage(".*Server startup complete.*", 1).withStartupTimeout(Duration.ofMinutes(2)));

  static Connection connection;
  static Channel channel;

  @BeforeAll
  static void startBroker() throws Exception {
    BROKER.start();
    ConnectionFactory factory = new ConnectionFactory();
    factory.setUri(uri());
    connection = factory.newConnection();
    channel = connection.createChannel();
  }

  @AfterAll
  static void stopBroker() throws Exception {
    connection.close();
    BROKER.stop();
  }

  private static String uri() {
    return "amqp://" + BROKER.getHost() + ":" + BROKER.getMappedPort(5672);
  }

  SourceHarness harness;

  @BeforeEach
  void start() throws Exception {
    channel.queueDelete("orders");
    channel.queueDelete("orders.dead");
    channel.queueDeclare("orders.dead", true, false, false, null);
    channel.queueDeclare(
        "orders", true, false, false, Map.of("x-dead-letter-exchange", "", "x-dead-letter-routing-key", "orders.dead"));
    harness = new SourceHarness(SourceHarness.secrets(Map.of("rabbit", "guest")), null);
  }

  @AfterEach
  void stop() {
    harness.close();
  }

  private static Ingestion queue(String queue, Split split) {
    return SourceHarness.ingestion("orders", new Source.RabbitMq(uri(), queue, "guest", new SecretRef("rabbit")), split);
  }

  private static void publish(String body) throws Exception {
    channel.basicPublish("", "orders", null, body.getBytes(StandardCharsets.UTF_8));
  }

  private static long waiting(String queue) {
    try {
      return channel.messageCount(queue);
    } catch (Exception e) {
      throw new IllegalStateException(e);
    }
  }

  @Test
  void aMessageBecomesJobsAndIsAcknowledged() throws Exception {
    publish("[{\"id\":1},{\"id\":2},{\"id\":3}]");
    publish("[{\"id\":4}]");
    harness.reconciler.reconcile(List.of(queue("orders", new Split.Json("$"))));
    assertThat(harness.reconciler.states()).extracting(RouteState::status).containsExactly(RouteState.Status.STARTED);

    await(() -> harness.payloads().size() == 4);

    assertThat(harness.payloads()).containsExactly("{\"id\":1}", "{\"id\":2}", "{\"id\":3}", "{\"id\":4}");
    // Stopping the route closes the connection; anything unacknowledged would be back on the queue.
    harness.reconciler.reconcile(List.of());
    assertThat(waiting("orders")).isZero();
    assertThat(waiting("orders.dead")).isZero();
  }

  @Test
  void aMessageThePlatformWouldNotTakeGoesBackOnTheQueueAndIsReadAgain() throws Exception {
    harness.platformDown = true;
    publish("{\"id\":1}");
    harness.reconciler.reconcile(List.of(queue("orders", new Split.None())));

    await(() -> harness.failedSinceLastAsked() >= 1);
    assertThat(harness.batches).isEmpty();

    harness.platformDown = false;
    await(() -> harness.payloads().size() == 1);
    assertThat(waiting("orders.dead")).isZero();
  }

  @Test
  void aMessageThatCannotBeReadIsDeadLetteredAndNotRedelivered() throws Exception {
    publish("[{\"id\":1},{\"id\":");
    publish("[{\"id\":2}]");
    harness.reconciler.reconcile(List.of(queue("orders", new Split.Json("$"))));

    await(() -> waiting("orders.dead") == 1);
    await(() -> harness.payloads().contains("{\"id\":2}"));
    assertThat(harness.payloads()).containsExactly("{\"id\":2}");
  }

  @Test
  void aQueueThatDoesNotExistIsReportedAsFailed() {
    harness.reconciler.reconcile(List.of(queue("no-such-queue", new Split.None())));

    assertThat(harness.reconciler.states()).singleElement().satisfies(state -> assertThat(state.status()).isEqualTo(RouteState.Status.FAILED));
    assertThat(harness.camel.getRoutes()).isEmpty();
  }
}
