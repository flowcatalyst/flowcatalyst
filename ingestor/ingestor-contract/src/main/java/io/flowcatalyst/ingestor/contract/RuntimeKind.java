package io.flowcatalyst.ingestor.contract;

import com.fasterxml.jackson.annotation.JsonInclude;
import java.util.List;
import org.jspecify.annotations.Nullable;

/**
 * A source kind the runtime reports in its poll: one an extension adds, or {@code camel-route}. The
 * platform stores and lists it, draws the form for it from {@code settings}, and enforces {@code
 * requiredPermission}; it knows nothing else about the kind.
 *
 * @param kind the kind's name, as a custom source names it in its {@code type}
 * @param description what it reads, for the form
 * @param requiredPermission the permission whoever creates or changes an ingestion of this kind must
 *     hold, or null for none
 * @param settings the kind's settings, in the order the form shows them
 */
@JsonInclude(JsonInclude.Include.NON_NULL)
public record RuntimeKind(String kind, String description, @Nullable String requiredPermission, List<SourceKind.Field> settings) {

  /** Copies the list. */
  public RuntimeKind {
    settings = List.copyOf(settings);
  }

  /**
   * The report of an extension's kind.
   *
   * @param kind the kind
   * @param requiredPermission the permission its ingestions need, or null
   * @return the report
   */
  public static RuntimeKind of(SourceKind kind, @Nullable String requiredPermission) {
    return new RuntimeKind(kind.type(), kind.description(), requiredPermission, kind.fields());
  }
}
