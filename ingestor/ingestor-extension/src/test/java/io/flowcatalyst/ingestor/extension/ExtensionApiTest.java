package io.flowcatalyst.ingestor.extension;

import static org.assertj.core.api.Assertions.assertThat;

import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import org.junit.jupiter.api.Test;

/**
 * {@link ExtensionApi#MAJOR} is the major of the ingestor API's version, {@code ingestor/API_VERSION}. A
 * new major of the API moves the file and this constant together, or this test fails.
 */
class ExtensionApiTest {

  @Test
  void theMajorIsTheMajorOfTheApiVersion() throws IOException {
    String version = Files.readString(Path.of("..", "API_VERSION"), StandardCharsets.UTF_8).strip();

    assertThat(version).matches("[0-9]+\\.[0-9]+\\.[0-9]+");
    assertThat(ExtensionApi.MAJOR)
        .as("ExtensionApi.MAJOR against ingestor/API_VERSION " + version)
        .isEqualTo(Integer.parseInt(version.substring(0, version.indexOf('.'))));
  }
}
