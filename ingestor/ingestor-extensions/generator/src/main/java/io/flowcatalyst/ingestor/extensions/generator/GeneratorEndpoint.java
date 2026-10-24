package io.flowcatalyst.ingestor.extensions.generator;

import io.flowcatalyst.ingestor.extension.ExtensionContext;
import java.time.Duration;
import java.time.Instant;
import java.util.Optional;
import org.apache.camel.Consumer;
import org.apache.camel.Exchange;
import org.apache.camel.Processor;
import org.apache.camel.Producer;
import org.apache.camel.support.ScheduledPollConsumer;
import org.apache.camel.support.ScheduledPollEndpoint;

/** One generator ingestion's endpoint: a scheduled poll that makes one payload each time. */
final class GeneratorEndpoint extends ScheduledPollEndpoint {

  /** The key the next record's number is kept under. */
  static final String NEXT = "next";

  private final ExtensionContext context;
  private final int count;
  private final String template;

  GeneratorEndpoint(ExtensionContext context, Duration every, int count, String template) {
    this.context = context;
    this.count = count;
    this.template = template;
    setCamelContext(context.camel());
    setInitialDelay(1000);
    setDelay(every.toMillis());
  }

  @Override
  protected String createEndpointUri() {
    return "ingest-generator:" + context.ingestion().id();
  }

  @Override
  public Producer createProducer() {
    throw new UnsupportedOperationException("an ingestion's source is only consumed from");
  }

  @Override
  public Consumer createConsumer(Processor processor) throws Exception {
    Consumer consumer = new Generator(processor);
    configureConsumer(consumer);
    return consumer;
  }

  /** The records from {@code first} on, as a JSON array. */
  String payload(long first, Instant now) {
    StringBuilder body = new StringBuilder("[");
    for (int i = 0; i < count; i++) {
      if (i > 0) {
        body.append(',');
      }
      body.append(template.replace("${n}", Long.toString(first + i)).replace("${now}", now.toString()));
    }
    return body.append(']').toString();
  }

  private final class Generator extends ScheduledPollConsumer {

    Generator(Processor processor) {
      super(GeneratorEndpoint.this, processor);
    }

    @Override
    protected int poll() throws Exception {
      Optional<String> kept = context.state().get(NEXT);
      long first = kept.map(Long::parseLong).orElse(1L);
      Exchange exchange = createExchange(true);
      boolean taken;
      try {
        exchange.getMessage().setBody(payload(first, Instant.now()));
        getProcessor().process(exchange);
        taken = exchange.getException() == null;
      } finally {
        releaseExchange(exchange, false);
      }
      if (!taken) {
        // The route has reported it; the same numbers are made again next time.
        return 0;
      }
      context.state().put(NEXT, Long.toString(first + count));
      return count;
    }
  }
}
