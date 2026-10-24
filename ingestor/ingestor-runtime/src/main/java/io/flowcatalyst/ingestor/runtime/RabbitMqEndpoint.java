package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.ingestor.contract.Source;
import com.rabbitmq.client.Channel;
import com.rabbitmq.client.Connection;
import com.rabbitmq.client.ConnectionFactory;
import com.rabbitmq.client.Delivery;
import io.flowcatalyst.framework.config.Secret;
import java.io.IOException;
import java.net.URI;
import java.time.Duration;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import javax.net.ssl.SSLContext;
import org.apache.camel.CamelContext;
import org.apache.camel.Consumer;
import org.apache.camel.Exchange;
import org.apache.camel.Processor;
import org.apache.camel.Producer;
import org.apache.camel.support.DefaultConsumer;
import org.apache.camel.support.DefaultEndpoint;
import org.jspecify.annotations.Nullable;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * A RabbitMQ queue consumed with the plain RabbitMQ client. The queue must exist; nothing is
 * declared. A message is acknowledged once its records are dispatch jobs. A message the platform
 * would not take jobs for goes back on the queue after a pause; one that cannot be read is
 * rejected without requeueing, so it goes to the queue's dead-letter exchange when it has one and
 * is dropped when it has not.
 */
final class RabbitMqEndpoint extends DefaultEndpoint {

  private static final Logger LOG = LoggerFactory.getLogger(RabbitMqEndpoint.class);

  /** Unacknowledged messages the broker hands one runtime at a time. */
  private static final int PREFETCH = 20;

  /** How long a message the platform would not take is held before it goes back on the queue. */
  private static final Duration REDELIVERY_PAUSE = Duration.ofSeconds(5);

  private static final Duration CONNECT_TIMEOUT = Duration.ofSeconds(10);

  private final String ingestionId;
  private final Source.RabbitMq source;
  private final Secret password;

  RabbitMqEndpoint(CamelContext camel, String ingestionId, Source.RabbitMq source, Secret password) {
    this.ingestionId = ingestionId;
    this.source = source;
    this.password = password;
    setCamelContext(camel);
  }

  @Override
  protected String createEndpointUri() {
    return "ingest-rabbitmq:" + ingestionId;
  }

  @Override
  public Producer createProducer() {
    throw new UnsupportedOperationException("an ingestion's source is only consumed from");
  }

  @Override
  public Consumer createConsumer(Processor processor) throws Exception {
    Consumer consumer = new QueueConsumer(processor);
    configureConsumer(consumer);
    return consumer;
  }

  private ConnectionFactory factory() throws Exception {
    URI uri = URI.create(source.uri());
    ConnectionFactory factory = new ConnectionFactory();
    factory.setUri(uri);
    if ("amqps".equalsIgnoreCase(uri.getScheme())) {
      // setUri alone would accept any certificate.
      factory.useSslProtocol(SSLContext.getDefault());
      factory.enableHostnameVerification();
    }
    factory.setUsername(source.username());
    factory.setPassword(password.reveal());
    factory.setConnectionTimeout((int) CONNECT_TIMEOUT.toMillis());
    // The connection's own reading and heartbeat threads, as the deliveries' (doStart).
    factory.setThreadFactory(Thread.ofVirtual().name("rabbitmq-" + ingestionId + "-", 0).factory());
    factory.setAutomaticRecoveryEnabled(true);
    factory.setTopologyRecoveryEnabled(true);
    return factory;
  }

  private final class QueueConsumer extends DefaultConsumer {

    private final CountDownLatch stopping = new CountDownLatch(1);
    private @Nullable ExecutorService executor;
    private @Nullable Connection connection;

    QueueConsumer(Processor processor) {
      super(RabbitMqEndpoint.this, processor);
    }

    @Override
    protected void doStart() throws Exception {
      super.doStart();
      ExecutorService deliveries = Executors.newVirtualThreadPerTaskExecutor();
      executor = deliveries;
      try {
        Connection opened = factory().newConnection(deliveries, "ingest-" + ingestionId);
        connection = opened;
        Channel channel = opened.createChannel();
        channel.basicQos(PREFETCH);
        channel.basicConsume(
            source.queue(), false, (tag, delivery) -> deliver(channel, delivery), tag -> LOG.warn("the broker cancelled the consumer of ingestion {}", ingestionId));
      } catch (Exception e) {
        disconnect();
        throw e;
      }
    }

    @Override
    protected void doStop() throws Exception {
      stopping.countDown();
      disconnect();
      super.doStop();
    }

    private void disconnect() {
      Connection opened = connection;
      connection = null;
      if (opened != null) {
        try {
          // Messages not yet acknowledged go back on the queue.
          opened.close((int) CONNECT_TIMEOUT.toMillis());
        } catch (IOException | RuntimeException e) {
          LOG.debug("closing the connection of ingestion {} failed", ingestionId, e);
        }
      }
      ExecutorService deliveries = executor;
      executor = null;
      if (deliveries != null) {
        deliveries.shutdownNow();
      }
    }

    /** One message at a time per channel, in the order the broker delivered them. */
    private void deliver(Channel channel, Delivery delivery) throws IOException {
      long tag = delivery.getEnvelope().getDeliveryTag();
      Exchange exchange = createExchange(false);
      try {
        exchange.getMessage().setBody(delivery.getBody());
        try {
          getProcessor().process(exchange);
        } catch (Exception e) {
          exchange.setException(e);
        }
        if (exchange.getException() != null) {
          LOG.warn("ingestion {} could not take a message; it goes back on the queue: {}", ingestionId, exchange.getException().getMessage());
          pause();
          channel.basicNack(tag, false, true);
        } else if (exchange.getProperty(IngestionRoute.REJECTED) != null) {
          channel.basicReject(tag, false);
        } else {
          channel.basicAck(tag, false);
        }
      } finally {
        releaseExchange(exchange, false);
      }
    }

    /** Keeps a failing message from coming straight back, without holding up a stop. */
    private void pause() {
      try {
        if (stopping.await(REDELIVERY_PAUSE.toMillis(), TimeUnit.MILLISECONDS)) {
          return;
        }
      } catch (InterruptedException e) {
        Thread.currentThread().interrupt();
      }
    }
  }
}
