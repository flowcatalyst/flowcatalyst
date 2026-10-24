package io.flowcatalyst.ingestor.contract;

import com.fasterxml.jackson.annotation.JsonInclude;
import java.net.URI;
import java.time.Duration;
import java.time.format.DateTimeParseException;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.regex.Pattern;
import org.jspecify.annotations.Nullable;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.node.JsonNodeFactory;

/**
 * A kind of source an extension adds: what an ingestion of this kind is configured with. Its
 * {@link ExtensionKind} is carried by the runtime, which reports it to the platform with every poll
 * (the operator pages draw the form from its fields) and checks a {@link Source.Custom}'s settings
 * against it. An ingestion of a custom kind runs on one runtime at a time, like a polled source.
 *
 * @param type the kind's name, as a custom source names it: lowercase letters and digits in groups
 *     joined by single hyphens, such as {@code acme-orders}
 * @param name what a person calls it
 * @param description what it reads, for the form
 * @param fields its settings, in the order the form shows them
 */
public record SourceKind(String type, String name, String description, List<Field> fields) {

  /** A kind's type: lowercase letters and digits in groups joined by single hyphens. */
  public static final Pattern TYPE = Pattern.compile("[a-z][a-z0-9]*(-[a-z0-9]+)*");

  /** A setting's name: a letter, then letters and digits. */
  public static final Pattern FIELD_NAME = Pattern.compile("[a-zA-Z][a-zA-Z0-9]*");

  /**
   * Copies the list.
   *
   * @throws IllegalArgumentException when the type is not a type's form or is a built-in kind's, or
   *     a field's name is not a setting's form, is {@code kind}, or is used twice
   */
  public SourceKind {
    fields = List.copyOf(fields);
    if (!TYPE.matcher(type).matches() || Source.builtInKinds().contains(type)) {
      throw new IllegalArgumentException("'" + type + "' cannot be an extension's type: it must be lowercase words joined by '-', and not a built-in kind");
    }
    java.util.Set<String> names = new java.util.HashSet<>();
    for (Field field : fields) {
      // A definition file writes the settings beside the kind.
      if (!FIELD_NAME.matcher(field.name()).matches() || field.name().equals("kind") || !names.add(field.name())) {
        throw new IllegalArgumentException(type + "'s setting '" + field.name() + "' must be a letter then letters and digits, not 'kind', and used once");
      }
    }
  }

  /**
   * What is wrong with settings for this kind: a setting it does not have, a required one missing
   * (or blank), or a value that is not what its field holds. The runtime reports an ingestion with any as failed;
   * the runtime checks again before it builds the source.
   *
   * @param settings the settings, by name
   * @return the problems by setting name; empty when there are none
   */
  public Map<String, String> problems(Map<String, JsonNode> settings) {
    Map<String, String> problems = new LinkedHashMap<>();
    for (String name : settings.keySet()) {
      if (fields.stream().noneMatch(f -> f.name().equals(name))) {
        problems.put(name, "is not a setting of " + type);
      }
    }
    for (Field field : fields) {
      JsonNode value = settings.get(field.name());
      // A blank text is not given: forms send what was left empty as "".
      if (value == null || value.isNull() || (value.isString() && value.stringValue().isBlank() && field.type() != Type.MULTILINE)) {
        if (field.required() && field.defaultValue() == null) {
          problems.put(field.name(), "is required");
        }
        continue;
      }
      String problem = field.problem(value);
      if (problem != null) {
        problems.put(field.name(), problem);
      }
    }
    return problems;
  }

  /**
   * The settings with the default of each one not given, as its field would hold it.
   *
   * @param settings the settings given, by name
   * @return a new map: the settings given, then the defaults
   */
  public Map<String, JsonNode> withDefaults(Map<String, JsonNode> settings) {
    Map<String, JsonNode> all = new LinkedHashMap<>(settings);
    for (Field field : fields) {
      JsonNode given = all.get(field.name());
      String fallback = field.defaultValue();
      if ((given == null || given.isNull()) && fallback != null) {
        all.put(field.name(), switch (field.type()) {
          case BOOLEAN -> JsonNodeFactory.instance.booleanNode(Boolean.parseBoolean(fallback));
          case INTEGER -> JsonNodeFactory.instance.numberNode(Long.parseLong(fallback));
          default -> JsonNodeFactory.instance.stringNode(fallback);
        });
      }
    }
    return all;
  }

  /**
   * One setting. {@link #field(String, String, Type)} builds one.
   *
   * @param name its key in the settings
   * @param label what the form calls it
   * @param description help shown under it, or null
   * @param type what it holds
   * @param required whether it must be given
   * @param defaultValue what it is when not given, as text, or null
   * @param choices for a {@code CHOICE}, the values allowed; otherwise null
   * @param pattern for a {@code TEXT}, or each item of a {@code LIST}, a regular expression the
   *     whole value must match; otherwise null
   * @param min the least value: for an {@code INTEGER} a number, for a {@code DURATION} ISO 8601,
   *     for a {@code LIST} the fewest items; or null
   * @param max the most, as {@code min}; or null
   * @param rule what a good value is, as the problem shown when the pattern, the least or the most
   *     is not met, such as "must be between 30 seconds and an hour"; null for a plain one
   */
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public record Field(
      String name,
      String label,
      @Nullable String description,
      Type type,
      boolean required,
      @Nullable String defaultValue,
      @Nullable List<String> choices,
      @Nullable String pattern,
      @Nullable String min,
      @Nullable String max,
      @Nullable String rule) {

    /** Copies the choices. */
    public Field {
      choices = choices == null ? null : List.copyOf(choices);
    }

    /**
     * A setting without a default, choices or bounds.
     *
     * @param name its key
     * @param label what the form calls it
     * @param description help, or null
     * @param type what it holds
     * @param required whether it must be given
     */
    public Field(String name, String label, @Nullable String description, Type type, boolean required) {
      this(name, label, description, type, required, null, null, null, null, null, null);
    }

    /**
     * What is wrong with a value for this setting.
     *
     * @param value the value given, not null
     * @return the problem, or null when there is none
     */
    public @Nullable String problem(JsonNode value) {
      boolean text = value.isString();
      String string = text ? value.stringValue() : value.toString();
      return switch (type) {
        case TEXT -> !text ? "must be text" : matches(string) ? null : ruleOr("must match " + pattern);
        case MULTILINE -> text ? null : "must be text";
        case INTEGER -> integer(value);
        case BOOLEAN -> value.isBoolean() ? null : "must be true or false";
        case DURATION -> text && duration(string) ? durationBounds(Duration.parse(string)) : "must be a duration such as PT30S";
        case URL -> text && url(string) ? null : "must be an http or https address";
        // Its form is the secret stores' to say; the runtime checks it.
        case SECRET -> text && !string.isBlank() ? null : "must be a secret's name, such as api-password";
        case CHOICE -> text && choiceList().contains(string) ? null : "must be one of " + String.join(", ", choiceList());
        case LIST -> list(value);
      };
    }

    private @Nullable String integer(JsonNode value) {
      boolean text = value.isString();
      if (!((value.isIntegralNumber() && value.canConvertToLong()) || (text && value.stringValue().matches("-?[0-9]{1,18}")))) {
        return "must be a whole number";
      }
      long number = text ? Long.parseLong(value.stringValue()) : value.longValue();
      if ((min != null && number < Long.parseLong(min)) || (max != null && number > Long.parseLong(max))) {
        return ruleOr(range(min, max));
      }
      return null;
    }

    private @Nullable String durationBounds(Duration value) {
      if ((min != null && value.compareTo(Duration.parse(min)) < 0) || (max != null && value.compareTo(Duration.parse(max)) > 0)) {
        return ruleOr(range(min, max));
      }
      return null;
    }

    private @Nullable String list(JsonNode value) {
      if (!value.isArray()) {
        return "must be a list of text";
      }
      int size = value.size();
      if ((min != null && size < Integer.parseInt(min)) || (max != null && size > Integer.parseInt(max))) {
        return "must hold " + range(min, max).substring("must be ".length()) + " items";
      }
      for (int i = 0; i < size; i++) {
        JsonNode item = value.get(i);
        if (!item.isString() || !matches(item.stringValue())) {
          return "item " + (i + 1) + " (" + (item.isString() ? item.stringValue() : item.toString()) + ") " + ruleOr(item.isString() ? "must match " + pattern : "must be text");
        }
      }
      return null;
    }

    private boolean matches(String text) {
      return pattern == null || Pattern.compile(pattern).matcher(text).matches();
    }

    private String ruleOr(String plain) {
      return rule == null ? plain : rule;
    }

    private static String range(@Nullable String min, @Nullable String max) {
      if (min != null && max != null) {
        return "must be " + min + " to " + max;
      }
      return min != null ? "must be at least " + min : "must be at most " + max;
    }

    private static boolean duration(String text) {
      try {
        return !Duration.parse(text).isNegative();
      } catch (DateTimeParseException e) {
        return false;
      }
    }

    private static boolean url(String text) {
      try {
        URI url = new URI(text);
        return ("http".equals(url.getScheme()) || "https".equals(url.getScheme())) && url.getHost() != null;
      } catch (java.net.URISyntaxException e) {
        return false;
      }
    }

    /**
     * The choices; none when absent.
     *
     * @return the values allowed
     */
    public List<String> choiceList() {
      List<String> list = choices;
      return list == null ? List.of() : list;
    }
  }

  /**
   * Starts a setting: optional, without a default, choices or bounds.
   *
   * @param name its key in the settings
   * @param label what the form calls it
   * @param type what it holds
   * @return the builder
   */
  public static FieldBuilder field(String name, String label, Type type) {
    return new FieldBuilder(name, label, type);
  }

  /** Builds a {@link Field}. */
  public static final class FieldBuilder {

    private final String name;
    private final String label;
    private final Type type;
    private @Nullable String description;
    private boolean required;
    private @Nullable String defaultValue;
    private @Nullable List<String> choices;
    private @Nullable String pattern;
    private @Nullable String min;
    private @Nullable String max;
    private @Nullable String rule;

    private FieldBuilder(String name, String label, Type type) {
      this.name = name;
      this.label = label;
      this.type = type;
    }

    /**
     * Help shown under it.
     *
     * @param text the help
     * @return this
     */
    public FieldBuilder description(String text) {
      this.description = text;
      return this;
    }

    /**
     * It must be given (or have a default).
     *
     * @return this
     */
    public FieldBuilder required() {
      this.required = true;
      return this;
    }

    /**
     * What it is when not given.
     *
     * @param value the default, as text
     * @return this
     */
    public FieldBuilder defaultValue(String value) {
      this.defaultValue = value;
      return this;
    }

    /**
     * The values a {@code CHOICE} allows.
     *
     * @param values the values
     * @return this
     */
    public FieldBuilder choices(List<String> values) {
      this.choices = values;
      return this;
    }

    /**
     * A regular expression the whole of a {@code TEXT}, or each item of a {@code LIST}, must match.
     *
     * @param regex the expression
     * @return this
     */
    public FieldBuilder pattern(String regex) {
      Pattern.compile(regex);
      this.pattern = regex;
      return this;
    }

    /**
     * The least and the most: numbers for an {@code INTEGER}, ISO 8601 for a {@code DURATION}, item
     * counts for a {@code LIST}; either may be null.
     *
     * @param least the least, or null
     * @param most the most, or null
     * @return this
     */
    public FieldBuilder between(@Nullable String least, @Nullable String most) {
      this.min = least;
      this.max = most;
      return this;
    }

    /**
     * What a good value is, as the problem shown when the pattern or the bounds are not met.
     *
     * @param problem such as "must be between 30 seconds and an hour"
     * @return this
     */
    public FieldBuilder rule(String problem) {
      this.rule = problem;
      return this;
    }

    /**
     * The field.
     *
     * @return the field
     */
    public Field build() {
      return new Field(name, label, description, type, required, defaultValue, choices, pattern, min, max, rule);
    }
  }

  /** What a setting holds. In the settings a value is a JSON string, number or boolean, or a list's array of strings. */
  public enum Type {
    /** One line of text. */
    TEXT,
    /** Several lines of text, such as a query or a template. */
    MULTILINE,
    /** A whole number. */
    INTEGER,
    /** True or false. */
    BOOLEAN,
    /** A duration, ISO 8601 ({@code PT30S}) as the other sources' intervals. */
    DURATION,
    /** An http or https address. */
    URL,
    /** The name of a secret ({@link SecretRef}); never the secret's value. */
    SECRET,
    /** One of {@link Field#choices()}. */
    CHOICE,
    /** A list of text, such as registrations: a JSON array of strings. */
    LIST
  }
}
