package io.flowcatalyst.ingestor.runtime;

import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.function.FunctionApi;
import io.flowcatalyst.ingestor.extension.ExtensionApi;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import org.junit.jupiter.api.Test;

/** The versions the runtime says at start: its own, and the API's, whose major both API majors are. */
class RuntimeVersionsTest {

  @Test
  void theApiVersionIsApiVersionAndBothMajorsAreItsMajor() throws IOException {
    String api = Files.readString(Path.of("..", "API_VERSION"), StandardCharsets.UTF_8).strip();

    assertThat(RuntimeVersions.api()).isEqualTo(api);
    int major = Integer.parseInt(api.substring(0, api.indexOf('.')));
    assertThat(FunctionApi.MAJOR).isEqualTo(major);
    assertThat(ExtensionApi.MAJOR).isEqualTo(major);
  }

  @Test
  void theRuntimeVersionIsFilledIn() {
    assertThat(RuntimeVersions.runtime()).matches("[0-9]+\\.[0-9]+\\.[0-9]+(-[0-9A-Za-z.-]+)?");
  }
}
