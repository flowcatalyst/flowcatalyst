package io.flowcatalyst.ingestor.extensions.generator;

import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.ingestor.contract.ExtensionKind;
import io.flowcatalyst.ingestor.contract.SourceKind;
import java.util.Map;
import java.util.ServiceLoader;
import org.junit.jupiter.api.Test;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.json.JsonMapper;

/** The worked example's kind: what its fields allow, and that the runtime finds it. */
class GeneratorKindTest {

  private final SourceKind kind = new GeneratorKind().kind();
  private final JsonMapper mapper = JsonMapper.builder().build();

  @Test
  void nothingGivenIsValidBecauseEveryFieldHasADefault() {
    assertThat(kind.problems(Map.of())).isEmpty();
    assertThat(kind.withDefaults(Map.of()).get("every").asString()).isEqualTo("PT10S");
  }

  @Test
  void theBoundsOfEveryAndCountAreChecked() {
    Map<String, JsonNode> settings = Map.of("every", mapper.valueToTree("PT0.5S"), "count", mapper.valueToTree(5000));

    assertThat(kind.problems(settings)).containsEntry("every", "must be a second or more").containsEntry("count", "must be 1 to 1000");
  }

  @Test
  void aSettingTheKindDoesNotHaveIsRefused() {
    assertThat(kind.problems(Map.of("colour", mapper.valueToTree("red")))).containsEntry("colour", "is not a setting of generator");
  }

  @Test
  void theExtensionIsListedForTheRuntimeToFindAndReturnsThisKind() {
    var found = ServiceLoader.load(io.flowcatalyst.ingestor.extension.IngestExtension.class).stream().map(ServiceLoader.Provider::get).toList();

    assertThat(found).singleElement().isInstanceOf(GeneratorExtension.class);
    ExtensionKind returned = found.getFirst().kind();
    assertThat(returned.kind().type()).isEqualTo("generator");
  }
}
