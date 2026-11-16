package io.flowcatalyst.function;

import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;

import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;

/// [FunctionApi#MAJOR] is the major of the ingestor API's version, `ingestor/API_VERSION`, and the parent
/// pom's `ingestor-api.version` (the version this module is built as) is that file's version. A release of
/// the API (`scripts/release.sh ingestor-api`) moves the file and the property; a new major also moves
/// this constant and `ExtensionApi.MAJOR`, or this test fails.
class FunctionApiTest {

    @Test
    void theMajorIsTheMajorOfTheApiVersion() throws IOException {
        String version = apiVersion();

        assertThat(version).matches("[0-9]+\\.[0-9]+\\.[0-9]+");
        assertThat(FunctionApi.MAJOR)
                .as("FunctionApi.MAJOR against ingestor/API_VERSION " + version)
                .isEqualTo(Integer.parseInt(version.substring(0, version.indexOf('.'))));
    }

    @Test
    void theParentPomBuildsTheApiAtTheApiVersion() throws IOException {
        String pom = Files.readString(Path.of("..", "pom.xml"), StandardCharsets.UTF_8);

        assertThat(pom).contains("<ingestor-api.version>" + apiVersion() + "</ingestor-api.version>");
    }

    private static String apiVersion() throws IOException {
        return Files.readString(Path.of("..", "API_VERSION"), StandardCharsets.UTF_8).strip();
    }
}
