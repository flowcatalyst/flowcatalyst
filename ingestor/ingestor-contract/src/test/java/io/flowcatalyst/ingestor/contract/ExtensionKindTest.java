package io.flowcatalyst.ingestor.contract;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import io.flowcatalyst.ingestor.contract.SourceKind.Type;
import java.util.List;
import java.util.Map;
import org.jspecify.annotations.Nullable;
import org.junit.jupiter.api.Test;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.json.JsonMapper;

/** An extension's kind: what its fields check and its own rules. */
class ExtensionKindTest {

  private static final SourceKind KIND =
      new SourceKind(
          "fleet",
          "Fleet",
          "Vehicles.",
          List.of(
              SourceKind.field("account", "Account", Type.TEXT).required().pattern("[a-z]+").rule("must be lowercase letters").build(),
              SourceKind.field("every", "Every", Type.DURATION).defaultValue("PT1M").between("PT30S", "PT1H").build(),
              SourceKind.field("from", "From", Type.DURATION).build(),
              SourceKind.field("workers", "Workers", Type.INTEGER).defaultValue("2").between("1", "8").build(),
              SourceKind.field("plates", "Plates", Type.LIST).pattern("[A-Z0-9]+").between(null, "2").build()));

  /** Its own rule: from at least every. */
  private static final ExtensionKind FLEET =
      new ExtensionKind() {
        @Override
        public SourceKind kind() {
          return KIND;
        }

        @Override
        public Map<String, String> problems(Map<String, JsonNode> settings, @Nullable Split split) {
          JsonNode every = settings.get("every");
          JsonNode from = settings.get("from");
          return every != null && from != null && java.time.Duration.parse(from.asString()).compareTo(java.time.Duration.parse(every.asString())) < 0
              ? Map.of("from", "must be at least every")
              : Map.of();
        }
      };

  @Test
  void theFieldsCheckPatternsBoundsListsAndBlanksAndTheKindsOwnRulesAreToldWithThem() {
    Map<String, String> problems =
        FLEET.check(Source.Custom.of("fleet", Map.of("account", " ", "every", "PT5S", "workers", 9, "plates", List.of("AB1", "cd-2"))).settingMap(), null);
    assertThat(problems)
        .containsExactly(
            Map.entry("account", "is required"),
            Map.entry("every", "must be PT30S to PT1H"),
            Map.entry("workers", "must be 1 to 8"),
            Map.entry("plates", "item 2 (cd-2) must match [A-Z0-9]+"));

    // The kind's rule sees the default of every, PT1M, and the fields' problems come with it.
    assertThat(FLEET.check(Source.Custom.of("fleet", Map.of("account", "Acme", "from", "PT40S", "plates", List.of("A", "B", "C"))).settingMap(), null))
        .containsExactly(
            Map.entry("account", "must be lowercase letters"),
            Map.entry("plates", "must hold at most 2 items"),
            Map.entry("from", "must be at least every"));

    assertThat(FLEET.check(Source.Custom.of("fleet", Map.of("account", "acme", "from", "PT2M")).settingMap(), null)).isEmpty();
  }

  @Test
  void aKindsTypeAndSettingsAreCheckedWhenItIsMade() {
    assertThatThrownBy(() -> new SourceKind("sftp", "Mine", "", List.of())).hasMessageContaining("not a built-in kind");
    assertThatThrownBy(() -> new SourceKind("Fleet", "Mine", "", List.of())).hasMessageContaining("lowercase words");
    assertThatThrownBy(() -> new SourceKind("fleet", "Mine", "", List.of(SourceKind.field("kind", "Kind", Type.TEXT).build())))
        .hasMessageContaining("not 'kind'");
  }
}
