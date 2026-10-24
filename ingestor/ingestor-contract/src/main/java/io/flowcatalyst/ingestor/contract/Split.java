package io.flowcatalyst.ingestor.contract;

import com.fasterxml.jackson.annotation.JsonSubTypes;
import com.fasterxml.jackson.annotation.JsonTypeInfo;

/** How one inbound payload becomes records. Each record becomes one dispatch job. */
@JsonTypeInfo(use = JsonTypeInfo.Id.NAME, property = "kind")
@JsonSubTypes({
  @JsonSubTypes.Type(value = Split.None.class, name = "none"),
  @JsonSubTypes.Type(value = Split.Csv.class, name = "csv"),
  @JsonSubTypes.Type(value = Split.Json.class, name = "json"),
  @JsonSubTypes.Type(value = Split.Xml.class, name = "xml"),
})
public sealed interface Split {

  /** The payload is one record. */
  record None() implements Split {}

  /**
   * One record per line.
   *
   * @param header whether the first line is a header, which is not a record
   */
  record Csv(boolean header) implements Split {}

  /**
   * One record per element of an array in the document. The document is read as a stream, so the
   * path is only a walk down object fields to the array, not a full JSONPath.
   *
   * @param arrayPath {@code $} for a top-level array, or {@code $.field.field} for a nested one
   */
  record Json(String arrayPath) implements Split {}

  /**
   * One record per occurrence of an element.
   *
   * @param element the element's name
   */
  record Xml(String element) implements Split {}
}
