package io.flowcatalyst.ingestor.runtime;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import io.flowcatalyst.framework.core.App;
import io.flowcatalyst.ingestor.contract.RuntimeKind;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.HashMap;
import java.util.Map;
import org.apache.camel.CamelContext;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

/** The application starts from the config it ships, with no platform to talk to. */
class IngestRuntimeTest {

  @TempDir Path dir;

  private static Map<String, String> env() {
    Map<String, String> env = new HashMap<>();
    // Not local or test: the developer's own config/secrets.local.yaml is not read.
    env.put("APP_ENV", "ci");
    env.put("HTTP_PORT", "0");
    env.put("HTTP_HOST", "127.0.0.1");
    // Not local or test, so the platform must be https; nothing answers there.
    env.put("FLOWCATALYST_BASE_URL", "https://127.0.0.1:1");
    env.put("FLOWCATALYST_CLIENT_ID", "oac_test");
    env.put("FLOWCATALYST_CLIENT_SECRET", "test-secret");
    return env;
  }

  @Test
  void startsWithoutAPlatformAndReportsItAsDown() {
    try (App app = IngestRuntime.builder().configDirectory(Path.of("config")).environment(env()).start()) {
      assertThat(app.moduleIds()).containsExactly("core", "http-client", "flowcatalyst", "ingest-runtime", "http");
      assertThat(app.get(CamelContext.class).isStarted()).isTrue();
      // Camel's own threads (consumers, timers, scheduled polls) are virtual: every source is I/O bound.
      assertThat(System.getProperty(RuntimeModule.CAMEL_VIRTUAL_THREADS)).isEqualTo("true");
      assertThat(app.get(CamelContext.class).getExecutorServiceManager().newThread("check", () -> {}).isVirtual()).isTrue();
      assertThat(app.health().check().isUp()).isFalse();
      assertThat(app.health().criticality("background:" + RuntimeModule.LOOP)).isNotNull();
      // The runtime is named after its host unless told, and carries no camel-route unless enabled.
      assertThat(app.config().get(RuntimeModule.RUNTIME_ID)).startsWith("runtime-");
      assertThat(app.get(Extensions.class).kinds()).extracting(RuntimeKind::kind).doesNotContain("camel-route");
    }
  }

  @Test
  void theEnvironmentNamesTheRuntimeTheExtensionsDirectoryAndTheCamelFlag() {
    Map<String, String> env = env();
    env.put("INGEST_RUNTIME_ID", "runtime-blue");
    env.put("INGEST_EXTENSIONS_DIR", dir.toString());
    env.put("INGEST_CAMEL_ROUTES_ENABLED", "true");

    try (App app = IngestRuntime.builder().configDirectory(Path.of("config")).environment(env).start()) {
      assertThat(app.config().get(RuntimeModule.RUNTIME_ID)).isEqualTo("runtime-blue");
      assertThat(app.get(Extensions.class).kinds()).extracting(RuntimeKind::kind).contains("camel-route");
    }
  }

  @Test
  void aRuntimeIdThePlatformWouldRefuseStopsTheStart() {
    Map<String, String> env = env();
    env.put("INGEST_RUNTIME_ID", "runtime blue");

    assertThatThrownBy(() -> IngestRuntime.builder().configDirectory(Path.of("config")).environment(env).start().close())
        .hasStackTraceContaining("ingest.runtime-id");
  }

  @Test
  void aBadExtensionJarStopsTheStartAndNamesTheJar() throws Exception {
    Files.writeString(dir.resolve("bad.jar"), "not a jar");
    Map<String, String> env = env();
    env.put("INGEST_EXTENSIONS_DIR", dir.toString());

    assertThatThrownBy(() -> IngestRuntime.builder().configDirectory(Path.of("config")).environment(env).start().close())
        .hasStackTraceContaining("bad.jar");
  }
}
