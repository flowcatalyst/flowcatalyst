package io.flowcatalyst.ingestor.runtime;

import java.io.IOException;
import java.io.InputStream;
import java.net.URI;
import java.net.URISyntaxException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.attribute.PosixFilePermissions;
import java.security.DigestInputStream;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.HexFormat;
import java.util.List;
import java.util.Set;
import java.util.regex.Pattern;
import org.jspecify.annotations.Nullable;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import tools.jackson.core.JacksonException;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.json.JsonMapper;

/**
 * The extensions a deployed environment is pinned to: a version 1 JSON manifest in S3, at {@code
 * s3://bucket/key}, listing jars in the same bucket with their SHA-256. Unknown fields are ignored so
 * the publisher can add to the manifest without a runtime change; a {@code version} other than 1 is
 * refused. Every refusal names the manifest, because the runtime does not start with a manifest it
 * cannot honour.
 */
final class ExtensionManifest {

  private static final Logger LOG = LoggerFactory.getLogger(ExtensionManifest.class);
  private static final JsonMapper MAPPER = JsonMapper.builder().build();
  private static final Pattern NAME = Pattern.compile("[a-z0-9][a-z0-9.-]*");
  private static final Pattern SHA256 = Pattern.compile("[0-9a-f]{64}");
  /** A manifest is a few lines; anything near this is not one. */
  private static final int MAX_BYTES = 1 << 20;

  /** One pinned extension jar. */
  record Entry(String name, String version, String key, String sha256) {}

  private final String location;
  private final String bucket;
  private final String source;
  private final List<Entry> entries;

  private ExtensionManifest(String location, String bucket, String source, List<Entry> entries) {
    this.location = location;
    this.bucket = bucket;
    this.source = source;
    this.entries = entries;
  }

  List<Entry> entries() {
    return entries;
  }

  /**
   * Fetches and checks the manifest.
   *
   * @param location {@code s3://bucket/key}
   * @return the manifest, or null when it does not exist (nothing published yet)
   * @throws IllegalStateException when the location, the read or the content is not usable
   */
  static @Nullable ExtensionManifest fetch(String location, ExtensionObjects objects) {
    String[] parts = split(location);
    byte[] bytes;
    try (InputStream in = objects.open(parts[0], parts[1])) {
      bytes = in.readNBytes(MAX_BYTES + 1);
    } catch (ExtensionObjects.Missing e) {
      LOG.warn("extensions: the manifest {} does not exist (nothing has been published for this environment yet); starting with no extensions", location);
      return null;
    } catch (IOException e) {
      throw new IllegalStateException("the extensions manifest " + location + " cannot be read: " + e.getMessage(), e);
    }
    if (bytes.length > MAX_BYTES) {
      throw new IllegalStateException("the extensions manifest " + location + " is larger than " + MAX_BYTES + " bytes");
    }
    return parse(location, parts[0], bytes);
  }

  private static String[] split(String location) {
    URI uri;
    try {
      uri = new URI(location.strip());
    } catch (URISyntaxException e) {
      throw new IllegalStateException("the extensions manifest setting '" + location + "' is not an s3://<bucket>/<key> location");
    }
    String key = uri.getPath() == null ? "" : uri.getPath().replaceFirst("^/", "");
    if (!"s3".equals(uri.getScheme()) || uri.getHost() == null || key.isEmpty()) {
      throw new IllegalStateException("the extensions manifest setting '" + location + "' is not an s3://<bucket>/<key> location");
    }
    return new String[] {uri.getHost(), key};
  }

  private static ExtensionManifest parse(String location, String bucket, byte[] bytes) {
    JsonNode root;
    try {
      root = MAPPER.readTree(bytes);
    } catch (JacksonException e) {
      throw new IllegalStateException("the extensions manifest " + location + " is not valid JSON: " + e.getOriginalMessage(), e);
    }
    if (root == null || !root.isObject()) {
      throw new IllegalStateException("the extensions manifest " + location + " is not a JSON object");
    }
    JsonNode version = root.get("version");
    if (version == null || !version.isIntegralNumber() || version.asInt() != 1) {
      throw new IllegalStateException("the extensions manifest " + location + " has version " + (version == null ? "none" : version) + "; this runtime reads version 1");
    }
    JsonNode listed = root.get("extensions");
    if (listed == null || !listed.isArray()) {
      throw new IllegalStateException("the extensions manifest " + location + " has no 'extensions' array");
    }
    List<Entry> entries = new ArrayList<>();
    Set<String> names = new HashSet<>();
    int index = 0;
    for (JsonNode each : listed) {
      String where = "the extensions manifest " + location + ", extensions[" + index++ + "]";
      String name = text(each, "name", where);
      if (!NAME.matcher(name).matches()) {
        throw new IllegalStateException(where + ": the name '" + name + "' must match " + NAME.pattern());
      }
      if (!names.add(name)) {
        throw new IllegalStateException(where + ": the name '" + name + "' is listed twice");
      }
      String key = text(each, "key", where);
      if (key.startsWith("/")) {
        throw new IllegalStateException(where + ": the key of '" + name + "' must not start with '/'");
      }
      String sha256 = text(each, "sha256", where);
      if (!SHA256.matcher(sha256).matches()) {
        throw new IllegalStateException(where + ": the sha256 of '" + name + "' must be 64 lowercase hex digits");
      }
      entries.add(new Entry(name, text(each, "version", where), key, sha256));
    }
    JsonNode commit = root.path("source").path("commit");
    String source = commit.isString() ? commit.asString() : "unknown";
    return new ExtensionManifest(location, bucket, source, List.copyOf(entries));
  }

  private static String text(JsonNode object, String field, String where) {
    JsonNode value = object.isObject() ? object.get(field) : null;
    if (value == null || !value.isString() || value.asString().isBlank()) {
      throw new IllegalStateException(where + ": '" + field + "' is missing or not a string");
    }
    return value.asString();
  }

  /**
   * Downloads the listed jars, each checked against its SHA-256 while it streams, into a fresh
   * directory only this process can read; the directory is removed when the process ends.
   *
   * @return the jars, in manifest order, each named {@code <name>.jar}
   * @throws IllegalStateException when a jar is missing, cannot be read, or does not match its hash
   */
  List<Path> download(ExtensionObjects objects) {
    Path directory = privateDirectory();
    Runtime.getRuntime().addShutdownHook(new Thread(() -> remove(directory), "extensions-cleanup"));
    List<Path> jars = new ArrayList<>();
    for (Entry entry : entries) {
      Path jar = directory.resolve(entry.name() + ".jar");
      String actual;
      try (InputStream raw = objects.open(bucket, entry.key());
          DigestInputStream in = new DigestInputStream(raw, MessageDigest.getInstance("SHA-256"))) {
        Files.copy(in, jar);
        actual = HexFormat.of().formatHex(in.getMessageDigest().digest());
      } catch (IOException e) {
        throw new IllegalStateException("the extension " + entry.name() + " (s3://" + bucket + "/" + entry.key() + ", listed by " + location + ") cannot be fetched: " + e.getMessage(), e);
      } catch (NoSuchAlgorithmException e) {
        throw new IllegalStateException("SHA-256 is not available", e);
      }
      if (!actual.equals(entry.sha256())) {
        throw new IllegalStateException(
            "the extension " + entry.name() + ".jar (s3://" + bucket + "/" + entry.key() + ", listed by " + location + ") does not match the manifest: expected sha256 "
                + entry.sha256() + ", got " + actual);
      }
      LOG.info("extensions: {} {} sha256 {} from {} (source commit {})", entry.name(), entry.version(), actual.substring(0, 12), location, source);
      jars.add(jar);
    }
    return List.copyOf(jars);
  }

  private static Path privateDirectory() {
    try {
      try {
        return Files.createTempDirectory("ingestor-extensions-", PosixFilePermissions.asFileAttribute(PosixFilePermissions.fromString("rwx------")));
      } catch (UnsupportedOperationException e) {
        // Not a POSIX file system: the platform's default for a temporary directory is all there is.
        return Files.createTempDirectory("ingestor-extensions-");
      }
    } catch (IOException e) {
      throw new IllegalStateException("a directory for the extension jars cannot be made: " + e.getMessage(), e);
    }
  }

  private static void remove(Path directory) {
    try (var files = Files.walk(directory)) {
      files.sorted(java.util.Comparator.reverseOrder()).forEach(path -> path.toFile().delete());
    } catch (IOException e) {
      // The process is ending; a leftover temporary directory is not worth a failure.
    }
  }
}
