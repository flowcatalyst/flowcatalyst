package io.flowcatalyst.ingestor.extension;

import io.flowcatalyst.ingestor.contract.SourceKind;
import io.flowcatalyst.framework.config.Secret;
import java.net.URI;
import java.time.Duration;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.function.Function;
import tools.jackson.databind.JsonNode;

/**
 * An ingestion's settings, read by their kind's fields: a setting not given reads as its field's
 * default. The settings have been checked against the kind, so a value read as its field's type is
 * that type; asking for a setting the kind does not have, or as another type, is a mistake in the
 * extension and throws {@link IllegalArgumentException}.
 */
public final class Settings {

  private final SourceKind kind;
  private final Map<String, JsonNode> values;
  private final Function<String, Secret> secrets;

  /**
   * Settings for one ingestion; made by the runtime.
   *
   * @param kind the kind they are for
   * @param values the values given
   * @param secrets resolves a secret's name for the ingestion's client
   */
  public Settings(SourceKind kind, Map<String, JsonNode> values, Function<String, Secret> secrets) {
    this.kind = kind;
    this.values = Map.copyOf(values);
    this.secrets = secrets;
  }

  /**
   * A text, multi-line or choice setting.
   *
   * @param name the setting
   * @return its value, when it has one
   */
  public Optional<String> text(String name) {
    return value(name, SourceKind.Type.TEXT, SourceKind.Type.MULTILINE, SourceKind.Type.CHOICE);
  }

  /**
   * A required text, multi-line or choice setting.
   *
   * @param name the setting
   * @return its value
   */
  public String requireText(String name) {
    return text(name).orElseThrow(() -> missing(name));
  }

  /**
   * A whole-number setting.
   *
   * @param name the setting
   * @return its value, when it has one
   */
  public Optional<Long> integer(String name) {
    return value(name, SourceKind.Type.INTEGER).map(Long::parseLong);
  }

  /**
   * A true-or-false setting; false when it has no value.
   *
   * @param name the setting
   * @return its value
   */
  public boolean flag(String name) {
    return value(name, SourceKind.Type.BOOLEAN).map(Boolean::parseBoolean).orElse(false);
  }

  /**
   * A duration setting.
   *
   * @param name the setting
   * @return its value, when it has one
   */
  public Optional<Duration> duration(String name) {
    return value(name, SourceKind.Type.DURATION).map(Duration::parse);
  }

  /**
   * An address setting.
   *
   * @param name the setting
   * @return its value, when it has one
   */
  public Optional<URI> url(String name) {
    return value(name, SourceKind.Type.URL).map(URI::create);
  }

  /**
   * The secret a setting names, resolved for the ingestion's client.
   *
   * @param name the setting
   * @return the secret, when the setting names one
   * @throws InvalidSettingsException when the secret is not set
   */
  public Optional<Secret> secret(String name) {
    return value(name, SourceKind.Type.SECRET).map(secrets);
  }

  /**
   * The secret a required setting names, resolved for the ingestion's client.
   *
   * @param name the setting
   * @return the secret
   * @throws InvalidSettingsException when the setting has no value or the secret is not set
   */
  public Secret requireSecret(String name) {
    return secret(name).orElseThrow(() -> missing(name));
  }

  /**
   * A list setting; empty when it has no value.
   *
   * @param name the setting
   * @return its items
   */
  public List<String> list(String name) {
    field(name, SourceKind.Type.LIST);
    JsonNode value = values.get(name);
    if (value == null || !value.isArray()) {
      return List.of();
    }
    List<String> items = new ArrayList<>();
    value.forEach(item -> items.add(item.asString()));
    return List.copyOf(items);
  }

  /** The value as text: a string's own text, a number's or a boolean's as JSON writes it; else the field's default. */
  private Optional<String> value(String name, SourceKind.Type... types) {
    SourceKind.Field field = field(name, types);
    JsonNode value = values.get(name);
    if (value == null || value.isNull()) {
      return Optional.ofNullable(field.defaultValue());
    }
    return Optional.of(value.isString() ? value.stringValue() : value.toString());
  }

  /** The setting's field, which is one of the types. */
  private SourceKind.Field field(String name, SourceKind.Type... types) {
    SourceKind.Field field =
        kind.fields().stream()
            .filter(f -> f.name().equals(name))
            .findFirst()
            .orElseThrow(() -> new IllegalArgumentException(kind.type() + " has no setting " + name));
    boolean typed = false;
    for (SourceKind.Type type : types) {
      typed |= field.type() == type;
    }
    if (!typed) {
      throw new IllegalArgumentException(kind.type() + "'s setting " + name + " is a " + field.type());
    }
    return field;
  }

  private static InvalidSettingsException missing(String name) {
    return new InvalidSettingsException("the setting " + name + " has no value");
  }
}
