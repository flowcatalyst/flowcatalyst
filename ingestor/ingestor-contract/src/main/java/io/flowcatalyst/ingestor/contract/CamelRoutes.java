package io.flowcatalyst.ingestor.contract;

import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Set;
import org.jspecify.annotations.Nullable;
import org.snakeyaml.engine.v2.api.Dump;
import org.snakeyaml.engine.v2.api.DumpSettings;
import org.snakeyaml.engine.v2.api.Load;
import org.snakeyaml.engine.v2.api.LoadSettings;
import org.snakeyaml.engine.v2.common.FlowStyle;
import org.snakeyaml.engine.v2.exceptions.YamlEngineException;
import tools.jackson.databind.JsonNode;

/**
 * What a {@code camel-route} source's YAML may hold, checked by the runtime when it takes the
 * ingestion and at "test connection" (the platform stores the YAML and knows nothing of Camel):
 * exactly one route, written {@code from:} or {@code route:}
 * holding {@code from:}, with the {@code from}'s {@code uri}, optional {@code parameters} and
 * {@code steps}. Nothing else at the top: no {@code beans}, {@code routeConfiguration}, {@code rest}
 * or templates. The route's id is the runtime's to give.
 *
 * <p>The runtime runs the steps, then hands the body to the ingestion's split and jobs. An exception
 * in a step is reported as a source failure and fails the exchange, so a source that can offer the
 * message again does.
 */
public final class CamelRoutes {

  /** The kind's type: a custom source of this type holds a route in its {@code yaml} setting. */
  public static final String TYPE = "camel-route";

  /**
   * The permission whoever creates or changes a camel-route ingestion must hold: a route runs with
   * the runtime's own network, identity and environment. The runtime reports it with the kind.
   */
  public static final String PERMISSION = "platform:ingestion:ingestion:route";

  private static final SourceKind KIND =
      new SourceKind(
          TYPE,
          "Camel route",
          "A route in Camel's YAML DSL: its from and steps make the payload, which the ingestion's split then reads.",
          List.of(
              SourceKind.field("yaml", "Route (YAML)", SourceKind.Type.MULTILINE)
                  .description("One from: with its uri and steps.")
                  .required()
                  .build()));

  private CamelRoutes() {}

  /**
   * The kind a runtime reports when routes are enabled.
   *
   * @return the kind, whose permission is {@link #PERMISSION}
   */
  public static RuntimeKind kind() {
    return RuntimeKind.of(KIND, PERMISSION);
  }

  /**
   * The route of a source.
   *
   * @param source an ingestion's source
   * @return the YAML when the source is a camel-route one, else null
   */
  public static @Nullable String yaml(Source source) {
    if (source instanceof Source.Custom custom && custom.type().equals(TYPE)) {
      JsonNode yaml = custom.settingMap().get("yaml");
      return yaml != null && yaml.isString() ? yaml.stringValue() : "";
    }
    return null;
  }

  private static final Set<String> ROUTE_KEYS = Set.of("id", "description", "from");
  private static final Set<String> FROM_KEYS = Set.of("id", "description", "uri", "parameters", "steps");

  /** A route that cannot be used, and why. */
  private static final class Invalid extends Exception {
    private static final long serialVersionUID = 1L;

    Invalid(String message) {
      super(message, null, false, false);
    }
  }

  /**
   * What is wrong with a route.
   *
   * @param yaml the route as written
   * @return the problem, or null when there is none
   */
  public static @Nullable String problem(@Nullable String yaml) {
    try {
      from(yaml);
      return null;
    } catch (Invalid e) {
      return e.getMessage();
    }
  }

  /**
   * The {@code from} endpoint's URI, for a summary.
   *
   * @param yaml the route as written
   * @return the URI, or null when the route cannot be read
   */
  public static @Nullable String uri(String yaml) {
    try {
      return String.valueOf(from(yaml).get("uri"));
    } catch (Invalid e) {
      return null;
    }
  }

  /**
   * The route as the runtime loads it: given its id, its steps run inside a try whose catch sends
   * to {@code failures}, and the body then goes to {@code tail}, the ingestion's split and jobs.
   *
   * @param yaml the route as written
   * @param routeId the id it is given
   * @param tail where the payload goes after the steps
   * @param failures where an exchange goes when a step throws; it reports and fails it
   * @return the route, in Camel's YAML DSL
   * @throws IllegalArgumentException when the route cannot be used; the message says why
   */
  public static String prepare(String yaml, String routeId, String tail, String failures) {
    Map<String, Object> from;
    try {
      from = from(yaml);
    } catch (Invalid e) {
      throw new IllegalArgumentException(e.getMessage());
    }
    List<Object> steps = new ArrayList<>(list(from.get("steps")));
    steps.add(Map.of("to", Map.of("uri", tail)));
    Map<String, Object> doTry = new LinkedHashMap<>();
    doTry.put("steps", steps);
    doTry.put("doCatch", List.of(Map.of("exception", List.of("java.lang.Exception"), "steps", List.of(Map.of("to", Map.of("uri", failures))))));
    Map<String, Object> prepared = new LinkedHashMap<>();
    prepared.put("uri", from.get("uri"));
    if (from.containsKey("parameters")) {
      prepared.put("parameters", from.get("parameters"));
    }
    prepared.put("steps", List.of(Map.of("doTry", doTry)));
    Map<String, Object> route = new LinkedHashMap<>();
    route.put("id", routeId);
    route.put("from", prepared);
    return new Dump(DumpSettings.builder().setDefaultFlowStyle(FlowStyle.BLOCK).build()).dumpToString(List.of(Map.of("route", route)));
  }

  /** The route's {@code from}, checked. */
  private static Map<String, Object> from(@Nullable String yaml) throws Invalid {
    if (yaml == null || yaml.isBlank()) {
      throw new Invalid("is required: a route's from: and its steps");
    }
    Object document;
    try {
      document = new Load(LoadSettings.builder().setLabel("route").build()).loadFromString(yaml);
    } catch (YamlEngineException e) {
      throw new Invalid("is not YAML: " + e.getMessage());
    }
    if (document instanceof List<?> elements) {
      if (elements.size() != 1) {
        throw new Invalid("must be one route, not " + elements.size() + " elements");
      }
      document = elements.getFirst();
    }
    Map<String, Object> top = map(document, "the route");
    if (top.size() != 1 || !(top.containsKey("from") || top.containsKey("route"))) {
      throw new Invalid("must be one route, written from: or route:, and nothing else (no beans, routeConfiguration, rest or templates)");
    }
    Map<String, Object> from;
    if (top.containsKey("route")) {
      Map<String, Object> route = map(top.get("route"), "route");
      only(route, ROUTE_KEYS, "route");
      from = map(route.get("from"), "route.from");
    } else {
      from = map(top.get("from"), "from");
    }
    only(from, FROM_KEYS, "from");
    Object uri = from.get("uri");
    if (!(uri instanceof String text) || text.isBlank()) {
      throw new Invalid("from needs its uri, such as timer:tick?period=60000");
    }
    if (from.containsKey("parameters") && !(from.get("parameters") instanceof Map<?, ?>)) {
      throw new Invalid("from.parameters must be a mapping");
    }
    if (from.containsKey("steps") && !(from.get("steps") instanceof List<?>)) {
      throw new Invalid("from.steps must be a list of steps");
    }
    return from;
  }

  @SuppressWarnings("unchecked")
  private static Map<String, Object> map(@Nullable Object value, String what) throws Invalid {
    if (!(value instanceof Map<?, ?> map)) {
      throw new Invalid(what + " must be a mapping");
    }
    for (Object key : map.keySet()) {
      if (!(key instanceof String)) {
        throw new Invalid(what + " has a key that is not text: " + key);
      }
    }
    return (Map<String, Object>) map;
  }

  private static List<?> list(@Nullable Object value) {
    return value instanceof List<?> list ? list : List.of();
  }

  private static void only(Map<String, Object> map, Set<String> allowed, String what) throws Invalid {
    for (String key : map.keySet()) {
      if (!allowed.contains(key)) {
        throw new Invalid(what + " may not hold " + key + "; only " + String.join(", ", allowed.stream().sorted().toList()));
      }
    }
  }
}
