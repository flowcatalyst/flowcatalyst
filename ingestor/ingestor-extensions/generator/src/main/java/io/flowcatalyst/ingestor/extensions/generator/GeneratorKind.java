package io.flowcatalyst.ingestor.extensions.generator;

import io.flowcatalyst.ingestor.contract.ExtensionKind;
import io.flowcatalyst.ingestor.contract.SourceKind;
import io.flowcatalyst.ingestor.contract.SourceKind.Type;
import java.util.List;

/**
 * Made-up records on a schedule, for samples, demos and load tests. Each poll is one payload: a JSON
 * array of {@code count} records from the template, so the ingestion's split is {@code json} with
 * the path {@code $}.
 */
public final class GeneratorKind implements ExtensionKind {

  /** The kind's type. */
  public static final String TYPE = "generator";

  static final String DEFAULT_TEMPLATE = "{\"n\": ${n}, \"at\": \"${now}\"}";

  private static final SourceKind KIND =
      new SourceKind(
          TYPE,
          "Generator",
          "Made-up records on a schedule, for trying an ingestion out. Split it as JSON with the path $.",
          List.of(
              SourceKind.field("every", "Every", Type.DURATION)
                  .description("How often a payload is made.")
                  .required()
                  .defaultValue("PT10S")
                  .between("PT1S", null)
                  .rule("must be a second or more")
                  .build(),
              SourceKind.field("count", "Records", Type.INTEGER)
                  .description("How many records each payload holds, 1 to 1000.")
                  .defaultValue("1")
                  .between("1", "1000")
                  .build(),
              SourceKind.field("template", "Template", Type.MULTILINE)
                  .description("One record as JSON. ${n} is the record's number, ${now} the time it was made.")
                  .defaultValue(DEFAULT_TEMPLATE)
                  .build()));

  @Override
  public SourceKind kind() {
    return KIND;
  }
}
