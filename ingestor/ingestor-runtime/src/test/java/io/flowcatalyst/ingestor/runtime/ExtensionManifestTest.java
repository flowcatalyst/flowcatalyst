package io.flowcatalyst.ingestor.runtime;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import io.flowcatalyst.framework.config.Secret;
import io.flowcatalyst.framework.httpclient.module.ServiceClients;
import io.flowcatalyst.ingestor.contract.ExtensionKind;
import io.flowcatalyst.ingestor.contract.RuntimeKind;
import io.flowcatalyst.ingestor.extension.ExtensionApi;
import io.flowcatalyst.ingestor.extension.IngestExtension;
import java.io.ByteArrayInputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.URISyntaxException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.security.MessageDigest;
import java.util.HashMap;
import java.util.HexFormat;
import java.util.List;
import java.util.Map;
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

/** Extensions pinned by a manifest in S3, read through a fake bucket so no AWS is needed. */
class ExtensionManifestTest {

  private static final String LOCATION = "s3://ext-bucket/manifest.json";

  @TempDir Path dir;

  /** A bucket held in memory. */
  private static final class FakeBucket implements ExtensionObjects {
    final Map<String, byte[]> objects = new HashMap<>();
    @Nullable IOException failure;

    @Override
    public InputStream open(String bucket, String key) throws IOException {
      if (failure != null) {
        throw failure;
      }
      byte[] content = "ext-bucket".equals(bucket) ? objects.get(key) : null;
      if (content == null) {
        throw new Missing("s3://" + bucket + "/" + key + " does not exist");
      }
      return new ByteArrayInputStream(content);
    }
  }

  private static String sha256(byte[] bytes) throws Exception {
    return HexFormat.of().formatHex(MessageDigest.getInstance("SHA-256").digest(bytes));
  }

  private static String manifest(int version, String... entries) {
    return "{\"version\":" + version + ",\"environment\":\"np\",\"publishedAt\":\"2026-10-29T12:00:00Z\",\"future\":true,"
        + "\"source\":{\"repository\":\"r/m\",\"commit\":\"0123456789abcdef0123456789abcdef01234567\"},\"extensions\":[" + String.join(",", entries) + "]}";
  }

  private static String entry(String name, String key, String sha) {
    return "{\"name\":\"" + name + "\",\"version\":\"0.1.0\",\"key\":\"" + key + "\",\"sha256\":\"" + sha + "\",\"extra\":1}";
  }

  private static Path location(Class<?> type) throws URISyntaxException {
    return Path.of(type.getProtectionDomain().getCodeSource().getLocation().toURI());
  }

  /** A real extension of {@code type}, compiled and written as a jar's bytes. */
  private byte[] jar(String className, String type) throws Exception {
    Path classes = Files.createTempDirectory(dir, "classes");
    Path src = classes.resolve("demo/" + className + ".java");
    Files.createDirectories(src.getParent());
    Files.writeString(
        src,
        """
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
        """.formatted(className, type, type));
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
    manifest.getMainAttributes().putValue(ExtensionApi.MANIFEST_ATTRIBUTE, String.valueOf(ExtensionApi.MAJOR));
    Path jar = dir.resolve(className + ".jar");
    try (OutputStream out = Files.newOutputStream(jar); JarOutputStream jarOut = new JarOutputStream(out, manifest)) {
      jarOut.putNextEntry(new JarEntry("demo/" + className + ".class"));
      jarOut.write(Files.readAllBytes(classes.resolve("demo/" + className + ".class")));
      jarOut.closeEntry();
      jarOut.putNextEntry(new JarEntry("META-INF/services/" + IngestExtension.class.getName()));
      jarOut.write(("demo." + className + "\n").getBytes(StandardCharsets.UTF_8));
      jarOut.closeEntry();
    }
    return Files.readAllBytes(jar);
  }

  private static FakeBucket bucketWith(String manifest) {
    FakeBucket bucket = new FakeBucket();
    bucket.objects.put("manifest.json", manifest.getBytes(StandardCharsets.UTF_8));
    return bucket;
  }

  private Extensions load(FakeBucket bucket) throws IOException {
    return Extensions.load(Files.createTempDirectory(dir, "empty"), LOCATION, bucket, false);
  }

  @Test
  void theListedJarsAreDownloadedVerifiedAndLoadedWithTheUsualChecks() throws Exception {
    byte[] echo = jar("EchoExtension", "echo");
    byte[] fleet = jar("FleetExtension", "fleet");
    FakeBucket bucket = bucketWith(manifest(1, entry("ingest-extension-echo", "jars/echo-abc.jar", sha256(echo)), entry("fleet", "jars/fleet.jar", sha256(fleet))));
    bucket.objects.put("jars/echo-abc.jar", echo);
    bucket.objects.put("jars/fleet.jar", fleet);

    Extensions loaded = load(bucket);

    // generator is on this test's class path; echo and fleet come from the manifest.
    assertThat(loaded.kinds()).extracting(RuntimeKind::kind).containsExactly("echo", "fleet", "generator");
  }

  @Test
  void aManifestThatDoesNotExistStartsWithNoExtensions() throws Exception {
    Extensions loaded = load(new FakeBucket());

    assertThat(loaded.kinds()).extracting(RuntimeKind::kind).containsExactly("generator");
  }

  @Test
  void aShaMismatchStopsTheStartNamingTheJarAndBothHashes() throws Exception {
    byte[] echo = jar("EchoExtension", "echo");
    String expected = "0".repeat(64);
    FakeBucket bucket = bucketWith(manifest(1, entry("echo", "jars/echo.jar", expected)));
    bucket.objects.put("jars/echo.jar", echo);

    assertThatThrownBy(() -> load(bucket))
        .isInstanceOf(IllegalStateException.class)
        .hasMessageContaining("echo.jar")
        .hasMessageContaining("expected sha256 " + expected)
        .hasMessageContaining("got " + sha256(echo));
  }

  @Test
  void aJarTheManifestListsButTheBucketLacksStopsTheStart() throws Exception {
    FakeBucket bucket = bucketWith(manifest(1, entry("echo", "jars/gone.jar", "a".repeat(64))));

    assertThatThrownBy(() -> load(bucket)).hasMessageContaining("echo").hasMessageContaining("jars/gone.jar").hasMessageContaining(LOCATION);
  }

  @Test
  void aManifestThatCannotBeUsedStopsTheStartNamingIt() throws Exception {
    String sha = "a".repeat(64);
    Map<String, String> bad =
        Map.of(
            "not json", "{nope",
            "version 2", manifest(2),
            "no version", "{\"extensions\":[]}",
            "twice", manifest(1, entry("a", "k1", sha), entry("a", "k2", sha)),
            "name form", manifest(1, entry("Upper", "k", sha)),
            "sha form", manifest(1, entry("a", "k", "ABC")),
            "no key", manifest(1, "{\"name\":\"a\",\"version\":\"1\",\"sha256\":\"" + sha + "\"}"));
    for (Map.Entry<String, String> each : bad.entrySet()) {
      assertThatThrownBy(() -> load(bucketWith(each.getValue()))).as(each.getKey()).isInstanceOf(IllegalStateException.class).hasMessageContaining(LOCATION);
    }
  }

  @Test
  void aReadFailureOtherThanMissingStopsTheStart() throws Exception {
    FakeBucket bucket = new FakeBucket();
    bucket.failure = new IOException("access denied");

    assertThatThrownBy(() -> load(bucket)).hasMessageContaining(LOCATION).hasMessageContaining("access denied");
  }

  @Test
  void aSettingThatIsNotAnS3LocationStopsTheStart() throws Exception {
    assertThatThrownBy(() -> Extensions.load(null, "https://x/y", new FakeBucket(), false)).hasMessageContaining("s3://<bucket>/<key>");
    assertThatThrownBy(() -> Extensions.load(null, "s3://bucket-only", new FakeBucket(), false)).hasMessageContaining("s3://<bucket>/<key>");
  }

  @Test
  void jarsInTheDirectoryAlongsideAManifestStopTheStartButAnEmptyOrAbsentDirectoryIsFine() throws Exception {
    Path withJar = Files.createTempDirectory(dir, "extensions");
    Files.write(withJar.resolve("x.jar"), new byte[] {1});

    assertThatThrownBy(() -> Extensions.load(withJar, LOCATION, new FakeBucket(), false)).hasMessageContaining("not both").hasMessageContaining(LOCATION);
    assertThat(Extensions.load(dir.resolve("absent"), LOCATION, new FakeBucket(), false).kinds()).hasSize(1);
    assertThat(Extensions.load(Files.createTempDirectory(dir, "empty2"), LOCATION, new FakeBucket(), false).kinds()).hasSize(1);
  }
}
