package io.flowcatalyst.ingestor.runtime;

import java.io.IOException;
import java.io.InputStream;
import java.util.Properties;
import org.jspecify.annotations.Nullable;

/**
 * This runtime's version and the version of the ingestor API it serves (the function API and the
 * extension API share it), as the build wrote them into {@code versions.properties}. The runtime's
 * moves with every release ({@code ingestor/VERSION}); the API's only when the API changes ({@code
 * ingestor/API_VERSION}). Either is {@code dev} when the classes were not built by Maven.
 */
public final class RuntimeVersions {

  private static final String RESOURCE = "versions.properties";
  private static final Properties VERSIONS = read();

  private RuntimeVersions() {}

  /**
   * This runtime's version.
   *
   * @return for example {@code 0.2.0}
   */
  public static String runtime() {
    return value("runtime");
  }

  /**
   * The version of the ingestor API this runtime serves: a function or an extension built against the
   * same major and this minor or an older one runs on it.
   *
   * @return for example {@code 1.0.0}
   */
  public static String api() {
    return value("api");
  }

  private static String value(String key) {
    @Nullable String value = VERSIONS.getProperty(key);
    return value == null || value.isBlank() || value.contains("${") ? "dev" : value;
  }

  private static Properties read() {
    Properties properties = new Properties();
    try (InputStream in = RuntimeVersions.class.getResourceAsStream(RESOURCE)) {
      if (in != null) {
        properties.load(in);
      }
    } catch (IOException e) {
      // Reported as dev, like a build without the resource.
    }
    return properties;
  }
}
