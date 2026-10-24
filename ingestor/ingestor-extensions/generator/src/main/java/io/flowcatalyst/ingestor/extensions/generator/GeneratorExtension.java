package io.flowcatalyst.ingestor.extensions.generator;

import io.flowcatalyst.ingestor.contract.ExtensionKind;
import io.flowcatalyst.ingestor.extension.ExtensionContext;
import io.flowcatalyst.ingestor.extension.IngestExtension;
import java.time.Duration;
import org.apache.camel.Endpoint;

/**
 * The runtime's part of {@link GeneratorKind}. Records are numbered on from the last poll; the
 * number is kept by the platform, so it carries on when another runtime takes the ingestion
 * over.
 */
public final class GeneratorExtension implements IngestExtension {

  @Override
  public ExtensionKind kind() {
    return new GeneratorKind();
  }

  @Override
  public Endpoint endpoint(ExtensionContext context) {
    // The kind's bounds have been checked: a second or more, 1 to 1000 records.
    Duration every = context.settings().duration("every").orElseThrow();
    long count = context.settings().integer("count").orElseThrow();
    String template = context.settings().requireText("template");
    return new GeneratorEndpoint(context, every, (int) count, template);
  }
}
