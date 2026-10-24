package io.flowcatalyst.ingestor.runtime;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import io.flowcatalyst.framework.config.Secret;
import io.flowcatalyst.framework.httpclient.module.ServiceClients;
import io.flowcatalyst.ingestor.contract.ExtensionKind;
import io.flowcatalyst.ingestor.contract.RuntimeKind;
import io.flowcatalyst.ingestor.extension.ExtensionApi;
import io.flowcatalyst.ingestor.extension.IngestExtension;
import java.io.IOException;
import java.io.OutputStream;
import java.net.URISyntaxException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.List;
import java.util.jar.Attributes;
import java.util.jar.JarEntry;
import java.util.jar.JarOutputStream;
import java.util.jar.Manifest;
import javax.tools.JavaCompiler;
import javax.tools.ToolProvider;
import org.apache.camel.Endpoint;
import org.jspecify.annotations.Nullable;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;
import tools.jackson.databind.JsonNode;

/**
 * Extension jars in INGEST_EXTENSIONS_DIR: loaded apart from each other, in addition to the class
 * path's, and refused at start when built for another extension API. The jars here are made by
 * compiling a small extension, so no Docker and no second build is needed.
 */
class ExtensionLoadingTest {

  @TempDir Path dir;

  /** An extension of {@code type}, in a class of its own. */
  private static String source(String className, String type) {
    return """
        package demo;
        import io.flowcatalyst.ingestor.contract.ExtensionKind;
        import io.flowcatalyst.ingestor.contract.SourceKind;
        import io.flowcatalyst.ingestor.extension.ExtensionContext;
        import io.flowcatalyst.ingestor.extension.IngestExtension;
        import java.util.List;
        import org.apache.camel.Endpoint;
        public final class %s implements IngestExtension {
          public ExtensionKind kind() { return () -> new SourceKind("%s", "Demo", "A demo.", List.of()); }
          public Endpoint endpoint(ExtensionContext context) { return context.camel().getEndpoint("direct:%s"); }
        }
        """.formatted(className, type, type);
  }

  private static Path location(Class<?> type) throws URISyntaxException {
    return Path.of(type.getProtectionDomain().getCodeSource().getLocation().toURI());
  }

  /** Compiles an extension and writes it as a jar declaring {@code api} (or nothing when null). */
  private Path jar(String name, String className, String type, @Nullable String api, boolean listed) throws Exception {
    Path classes = Files.createTempDirectory(dir, "classes");
    Path src = classes.resolve("demo/" + className + ".java");
    Files.createDirectories(src.getParent());
    Files.writeString(src, source(className, type));
    List<Path> path =
        List.of(
            location(IngestExtension.class), location(ExtensionKind.class), location(Endpoint.class), location(JsonNode.class),
            location(Nullable.class), location(ServiceClients.class), location(Secret.class));
    JavaCompiler compiler = ToolProvider.getSystemJavaCompiler();
    int result =
        compiler.run(null, null, null, "-proc:none", "-d", classes.toString(), "-cp",
            String.join(java.io.File.pathSeparator, path.stream().map(Path::toString).toList()), src.toString());
    assertThat(result).as("the demo extension compiles").isZero();

    Manifest manifest = new Manifest();
    manifest.getMainAttributes().put(Attributes.Name.MANIFEST_VERSION, "1.0");
    if (api != null) {
      manifest.getMainAttributes().putValue(ExtensionApi.MANIFEST_ATTRIBUTE, api);
    }
    Path jar = dir.resolve(name);
    try (OutputStream out = Files.newOutputStream(jar); JarOutputStream jarOut = new JarOutputStream(out, manifest)) {
      jarOut.putNextEntry(new JarEntry("demo/" + className + ".class"));
      jarOut.write(Files.readAllBytes(classes.resolve("demo/" + className + ".class")));
      jarOut.closeEntry();
      if (listed) {
        jarOut.putNextEntry(new JarEntry("META-INF/services/" + IngestExtension.class.getName()));
        jarOut.write(("demo." + className + "\n").getBytes(StandardCharsets.UTF_8));
        jarOut.closeEntry();
      }
    }
    return jar;
  }

  private Path emptyDirectory() throws IOException {
    return Files.createTempDirectory(dir, "extensions");
  }

  /** Jars go in a directory of their own, apart from the classes compiled beside them. */
  private Path directoryWith(Path... jars) throws IOException {
    Path extensions = emptyDirectory();
    for (Path jar : jars) {
      Files.move(jar, extensions.resolve(jar.getFileName()));
    }
    return extensions;
  }

  @Test
  void jarsInTheDirectoryAreLoadedInAdditionToTheClassPathsAndTheirKindsReported() throws Exception {
    Path extensions = directoryWith(jar("echo.jar", "EchoExtension", "echo", "1", true), jar("fleet.jar", "FleetExtension", "fleet", ExtensionApi.MAJOR + ".4", true));

    Extensions loaded = Extensions.load(extensions, false);

    // generator is on this test's class path; echo and fleet come from the directory.
    assertThat(loaded.kinds()).extracting(RuntimeKind::kind).containsExactly("echo", "fleet", "generator");
    assertThat(loaded.kinds()).extracting(RuntimeKind::requiredPermission).containsOnlyNulls();
  }

  @Test
  void eachJarHasAClassLoaderOfItsOwnThatSeesTheRuntimesClasses() throws Exception {
    Path extensions = directoryWith(jar("echo.jar", "EchoExtension", "echo", "1", true));
    List<IngestExtension> found = new ArrayList<>();
    ServiceLoaderProbe.collect(Extensions.load(extensions, false), found);

    // The extension's class is not on the runtime's class path; its loader is the jar's.
    assertThat(found).anySatisfy(e -> assertThat(e.getClass().getClassLoader()).isNotSameAs(Extensions.class.getClassLoader()));
    assertThat(found).allSatisfy(e -> assertThat(e).isInstanceOf(IngestExtension.class));
  }

  @Test
  void aJarBuiltForAnotherMajorVersionOfTheExtensionApiStopsTheStartNamingTheJar() throws Exception {
    Path extensions = directoryWith(jar("old-fleet.jar", "OldExtension", "old", String.valueOf(ExtensionApi.MAJOR + 1) + ".0", true));

    assertThatThrownBy(() -> Extensions.load(extensions, false))
        .isInstanceOf(IllegalStateException.class)
        .hasMessageContaining("old-fleet.jar")
        .hasMessageContaining("extension API " + (ExtensionApi.MAJOR + 1) + ".0")
        .hasMessageContaining("this runtime speaks " + ExtensionApi.MAJOR);
  }

  @Test
  void aJarThatDoesNotSayWhichApiItWasBuiltForOrListsNoExtensionIsRefusedToo() throws Exception {
    Path silent = directoryWith(jar("silent.jar", "SilentExtension", "silent", null, true));
    assertThatThrownBy(() -> Extensions.load(silent, false))
        .isInstanceOf(IllegalStateException.class)
        .hasMessageContaining("silent.jar")
        .hasMessageContaining(ExtensionApi.MANIFEST_ATTRIBUTE);

    Path unlisted = directoryWith(jar("unlisted.jar", "UnlistedExtension", "unlisted", "1", false));
    assertThatThrownBy(() -> Extensions.load(unlisted, false)).hasMessageContaining("unlisted.jar").hasMessageContaining("META-INF/services");

    Path notAJar = emptyDirectory();
    Files.writeString(notAJar.resolve("broken.jar"), "not a jar");
    assertThatThrownBy(() -> Extensions.load(notAJar, false)).hasMessageContaining("broken.jar");
  }

  @Test
  void aTypeTwoJarsOrAJarAndTheClassPathShareStopsTheStartNamingBoth() throws Exception {
    Path twins = directoryWith(jar("a.jar", "AExtension", "same", "1", true), jar("b.jar", "BExtension", "same", "1", true));
    assertThatThrownBy(() -> Extensions.load(twins, false)).hasMessageContaining("b.jar").hasMessageContaining("is taken by a.jar").hasMessageContaining("the type 'same'");

    Path generator = directoryWith(jar("gen.jar", "GenExtension", "generator", "1", true));
    assertThatThrownBy(() -> Extensions.load(generator, false)).hasMessageContaining("gen.jar").hasMessageContaining(Extensions.CLASS_PATH);
  }

  @Test
  void aMissingDirectoryStopsTheStartAndNoDirectoryIsFine() throws Exception {
    assertThatThrownBy(() -> Extensions.load(dir.resolve("nothing"), false)).hasMessageContaining("is not a directory");
    assertThat(Extensions.load(emptyDirectory(), false).kinds()).extracting(RuntimeKind::kind).containsExactly("generator");
    assertThat(Extensions.load(null, false).kinds()).extracting(RuntimeKind::kind).containsExactly("generator");
  }

  @Test
  void camelRouteIsReportedWithItsPermissionOnlyWhenEnabledAndNoExtensionMayTakeItsType() throws Exception {
    assertThat(Extensions.load(null, false).kinds()).extracting(RuntimeKind::kind).doesNotContain("camel-route");

    RuntimeKind route = Extensions.load(null, true).kinds().getLast();
    assertThat(route.kind()).isEqualTo("camel-route");
    assertThat(route.requiredPermission()).isEqualTo("platform:ingestion:ingestion:route");
    assertThat(route.settings()).extracting(field -> field.name()).containsExactly("yaml");

    Path taker = directoryWith(jar("route.jar", "RouteExtension", "camel-route", "1", true));
    assertThatThrownBy(() -> Extensions.load(taker, true)).hasMessageContaining("route.jar").hasMessageContaining("is the runtime's own");
  }

  @Test
  void theExtensionApiMajorTheParentPomGivesTheWorkedExampleIsTheOneTheRuntimeChecksFor() throws Exception {
    String pom = Files.readString(Path.of("..", "pom.xml"));

    assertThat(pom).contains("<ingestor.extension-api>" + ExtensionApi.MAJOR + "</ingestor.extension-api>");
  }

  /** Reads the extensions an {@link Extensions} holds, which keeps them private. */
  private static final class ServiceLoaderProbe {
    static void collect(Extensions extensions, List<IngestExtension> into) throws Exception {
      java.lang.reflect.Field byType = Extensions.class.getDeclaredField("byType");
      byType.setAccessible(true);
      for (Object loaded : ((java.util.Map<?, ?>) byType.get(extensions)).values()) {
        java.lang.reflect.Method extension = loaded.getClass().getDeclaredMethod("extension");
        extension.setAccessible(true);
        into.add((IngestExtension) extension.invoke(loaded));
      }
    }
  }
}
