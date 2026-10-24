package io.flowcatalyst.ingestor.contract;

import java.util.LinkedHashMap;
import java.util.Map;
import org.jspecify.annotations.Nullable;
import tools.jackson.databind.JsonNode;

/**
 * A kind of source an extension adds, as the runtime needs it: its fields, and the rules its fields
 * cannot say. The runtime reports the kind to the platform with every poll (the platform draws the
 * form from its fields) and checks an ingestion's settings against it before it builds the
 * endpoint. It holds no Camel and no runtime code: the runtime's part is the module's {@code
 * IngestExtension}, whose {@code kind()} returns this.
 *
 * <p>An implementation has a public no-argument constructor and keeps no state.
 */
public interface ExtensionKind {

  /**
   * The kind: its type, name, description and fields. Its type is not a built-in kind's.
   *
   * @return the kind
   */
  SourceKind kind();

  /**
   * What is wrong with settings that each field on its own allows: a rule between two settings, an
   * address that must be https, a split the payload does not suit. A setting given is what its field
   * holds: one its field refused ({@link SourceKind#problems(Map)}) is left out, and one not given
   * has its default ({@link SourceKind#withDefaults(Map)}), so a setting may still be missing.
   *
   * @param settings the settings, by name: those its field allows, defaults included
   * @param split the ingestion's split, or null when it has none yet
   * @return the problems, by setting name, or {@code "split"} for the split; empty when there are
   *     none. A message holds no secret
   */
  default Map<String, String> problems(Map<String, JsonNode> settings, @Nullable Split split) {
    return Map.of();
  }

  /**
   * Every problem with settings: each field's, and the kind's own about the settings its fields
   * allow, so all are told at once; a field's problem is told before the kind's about the same
   * setting.
   *
   * @param settings the settings, by name
   * @param split the ingestion's split, or null
   * @return the problems, as {@link #problems(Map, Split)}
   */
  default Map<String, String> check(Map<String, JsonNode> settings, @Nullable Split split) {
    Map<String, String> all = new LinkedHashMap<>(kind().problems(settings));
    Map<String, JsonNode> allowed = new LinkedHashMap<>(settings);
    allowed.keySet().removeAll(all.keySet());
    problems(kind().withDefaults(allowed), split).forEach(all::putIfAbsent);
    return all;
  }
}
