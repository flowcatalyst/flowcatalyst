package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.ingestor.contract.CamelRoutes;
import io.flowcatalyst.ingestor.contract.ExtensionKind;
import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.RuntimeKind;
import io.flowcatalyst.ingestor.contract.SecretRef;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.contract.SourceKind;
import io.flowcatalyst.ingestor.extension.ExtensionApi;
import io.flowcatalyst.ingestor.extension.ExtensionContext;
import io.flowcatalyst.ingestor.extension.IngestExtension;
import io.flowcatalyst.ingestor.extension.InvalidSettingsException;
import io.flowcatalyst.ingestor.extension.Settings;
import io.flowcatalyst.framework.httpclient.module.ServiceClients;
import java.io.IOException;
import java.net.MalformedURLException;
import java.net.URL;
import java.net.URLClassLoader;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.ServiceConfigurationError;
import java.util.ServiceLoader;
import java.util.function.Function;
import java.util.jar.JarFile;
import java.util.jar.Manifest;
import java.util.stream.Stream;
import org.apache.camel.CamelContext;
import org.apache.camel.Endpoint;
import org.jspecify.annotations.Nullable;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import tools.jackson.databind.JsonNode;

/**
 * The extensions this runtime carries: those on the class path and the jars in the extensions
 * directory, each adding a kind of source ({@link IngestExtension}, its kind an {@link
 * ExtensionKind}), and the {@code camel-route} kind when routes are enabled. The runtime reports
 * their kinds to the platform with every poll, builds a {@code custom} source's endpoint with the
 * extension of its type, and runs its "test connection".
 */
final class Extensions {

  private static final Logger LOG = LoggerFactory.getLogger(Extensions.class);

  /** Where the extensions that are not from a jar come from. */
  static final String CLASS_PATH = "the class path";

  private final Map<String, Loaded> byType;
  private final boolean camelRoutes;

  private record Loaded(IngestExtension extension, ExtensionKind definition, String origin) {
    SourceKind kind() {
      return definition.kind();
    }
  }

  /** An extension found, and where. */
  record Found(IngestExtension extension, String origin) {}

  private Extensions(Map<String, Loaded> byType, boolean camelRoutes) {
    this.byType = byType;
    this.camelRoutes = camelRoutes;
  }

  /** No extensions, and no camel routes. */
  static Extensions none() {
    return new Extensions(Map.of(), false);
  }

  /**
   * The extensions given, from the class path.
   *
   * @param extensions the extensions
   * @return them, checked
   * @throws IllegalStateException when a type is malformed, built in, or taken twice, or a kind's
   *     fields are malformed: the runtime does not start with a broken extension
   */
  static Extensions of(List<IngestExtension> extensions) {
    return of(extensions.stream().map(e -> new Found(e, CLASS_PATH)).toList(), false);
  }

  /**
   * The extensions given.
   *
   * @param found the extensions and where each came from
   * @param camelRoutes whether the {@code camel-route} kind is carried
   * @return them, checked
   * @throws IllegalStateException as {@link #of(List)}; the message names where the extension came from
   */
  static Extensions of(List<Found> found, boolean camelRoutes) {
    Map<String, Loaded> byType = new LinkedHashMap<>();
    for (Found each : found) {
      IngestExtension extension = each.extension();
      String from = each.origin() + ": " + extension.getClass().getName();
      ExtensionKind definition;
      SourceKind kind;
      try {
        definition = extension.kind();
        // SourceKind checks the type (its form, not a built-in kind's) and the settings' names.
        kind = definition.kind();
      } catch (IllegalArgumentException e) {
        throw new IllegalStateException(from + ": " + e.getMessage(), e);
      }
      String type = kind.type();
      if (type.equals(CamelRoutes.TYPE)) {
        throw new IllegalStateException(from + ": the type '" + type + "' is the runtime's own");
      }
      if (byType.containsKey(type)) {
        throw new IllegalStateException(from + ": the type '" + type + "' is taken by " + byType.get(type).origin() + ": " + byType.get(type).extension().getClass().getName());
      }
      Map<String, JsonNode> defaults = kind.withDefaults(Map.of());
      for (SourceKind.Field field : kind.fields()) {
        if (field.type() == SourceKind.Type.CHOICE && field.choiceList().isEmpty()) {
          throw new IllegalStateException(from + ": the choice '" + field.name() + "' has no choices");
        }
        JsonNode fallback = defaults.get(field.name());
        String problem = fallback == null ? null : field.problem(fallback);
        if (problem != null) {
          throw new IllegalStateException(from + ": the default of '" + field.name() + "' " + problem);
        }
      }
      byType.put(type, new Loaded(extension, definition, each.origin()));
    }
    return new Extensions(Map.copyOf(byType), camelRoutes);
  }

  /**
   * The extensions on the class path and in the directory.
   *
   * @param directory a directory of extension jars, or null for none
   * @param camelRoutes whether the {@code camel-route} kind is carried
   * @return them, checked
   * @throws IllegalStateException when the directory cannot be read, a jar was built for another
   *     extension API or lists no extension, or {@link #of(List, boolean)} refuses one: the message names the jar
   */
  static Extensions load(@Nullable Path directory, boolean camelRoutes) {
    ClassLoader parent = Extensions.class.getClassLoader();
    List<Found> found = new ArrayList<>();
    ServiceLoader.load(IngestExtension.class, parent).forEach(e -> found.add(new Found(e, CLASS_PATH)));
    if (directory != null) {
      for (Path jar : jars(directory)) {
        found.addAll(fromJar(jar, parent));
      }
    }
    Extensions extensions = of(found, camelRoutes);
    if (!extensions.byType.isEmpty()) {
      LOG.info("extensions: {}", String.join(", ", extensions.byType.keySet()));
    }
    if (camelRoutes) {
      LOG.warn("camel routes are enabled: a camel-route ingestion runs with this runtime's network, identity and environment");
    }
    return extensions;
  }

  private static List<Path> jars(Path directory) {
    if (!Files.isDirectory(directory)) {
      throw new IllegalStateException("the extensions directory " + directory + " is not a directory");
    }
    try (Stream<Path> files = Files.list(directory)) {
      return files.filter(file -> file.getFileName().toString().endsWith(".jar")).sorted().toList();
    } catch (IOException e) {
      throw new IllegalStateException("the extensions directory " + directory + " cannot be read: " + e.getMessage(), e);
    }
  }

  /**
   * The extensions in one jar, loaded apart from every other jar's: the jar sees the runtime's
   * classes (the contract, the extension API, Camel) and its own, and not the other jars'.
   */
  private static List<Found> fromJar(Path jar, ClassLoader parent) {
    String name = jar.getFileName().toString();
    checkApi(jar, name);
    URLClassLoader loader;
    try {
      // Open for the life of the process: the extension's classes load from it on demand.
      loader = new URLClassLoader(name, new URL[] {jar.toUri().toURL()}, parent);
    } catch (MalformedURLException e) {
      throw new IllegalStateException("extension " + name + " cannot be opened: " + e.getMessage(), e);
    }
    List<Found> found = new ArrayList<>();
    try {
      ServiceLoader.load(IngestExtension.class, loader).stream()
          // The parent's own providers are found too: they are the class path's, already loaded.
          .filter(provider -> provider.type().getClassLoader() == loader)
          .forEach(provider -> found.add(new Found(provider.get(), name)));
    } catch (ServiceConfigurationError e) {
      throw new IllegalStateException("extension " + name + " cannot be loaded: " + e.getMessage(), e);
    }
    if (found.isEmpty()) {
      throw new IllegalStateException("extension " + name + " lists no " + IngestExtension.class.getName() + " in META-INF/services");
    }
    return found;
  }

  /** Refuses a jar built for another major version of the extension API, or one that does not say. */
  private static void checkApi(Path jar, String name) {
    String declared;
    try (JarFile file = new JarFile(jar.toFile())) {
      Manifest manifest = file.getManifest();
      declared = manifest == null ? null : manifest.getMainAttributes().getValue(ExtensionApi.MANIFEST_ATTRIBUTE);
    } catch (IOException e) {
      throw new IllegalStateException("extension " + name + " cannot be read as a jar: " + e.getMessage(), e);
    }
    if (declared == null) {
      throw new IllegalStateException(
          "extension " + name + " does not declare the extension API it was built against: its manifest needs "
              + ExtensionApi.MANIFEST_ATTRIBUTE + ": " + ExtensionApi.MAJOR);
    }
    int major;
    try {
      major = Integer.parseInt(declared.strip().split("\\.", 2)[0]);
    } catch (NumberFormatException e) {
      throw new IllegalStateException("extension " + name + " declares extension API '" + declared + "', which is not a version number");
    }
    if (major != ExtensionApi.MAJOR) {
      throw new IllegalStateException(
          "extension " + name + " was built for extension API " + declared + "; this runtime speaks " + ExtensionApi.MAJOR
              + ". Rebuild it against ingestor-extension " + ExtensionApi.MAJOR + ".x");
    }
  }

  /**
   * The kinds to report with every poll: the extensions', and camel-route when enabled.
   *
   * @return the kinds, extensions in type order
   */
  List<RuntimeKind> kinds() {
    List<RuntimeKind> kinds = new ArrayList<>();
    byType.values().stream().map(Loaded::kind).sorted(Comparator.comparing(SourceKind::type)).forEach(kind -> kinds.add(RuntimeKind.of(kind, null)));
    if (camelRoutes) {
      kinds.add(CamelRoutes.kind());
    }
    return List.copyOf(kinds);
  }

  /** Whether this runtime runs {@code camel-route} ingestions. */
  boolean camelRoutes() {
    return camelRoutes;
  }

  /**
   * The endpoint for a custom source.
   *
   * @throws UnsupportedIngestionException when no extension has the type, or the settings cannot be used
   */
  Endpoint endpoint(CamelContext camel, Ingestion ingestion, Source.Custom custom, Secrets secrets, StateStore state, @Nullable ServiceClients http) {
    Loaded loaded = loaded(custom);
    Context context = context(camel, ingestion, custom, loaded, secrets, state, http, prefix -> ApiPollClient.name(prefix, ingestion.id(), "v" + ingestion.version()));
    try {
      return loaded.extension().endpoint(context);
    } catch (InvalidSettingsException e) {
      throw new UnsupportedIngestionException(String.valueOf(e.getMessage()));
    } catch (UnsupportedIngestionException e) {
      throw e;
    } catch (Exception e) {
      throw new IllegalStateException("the " + custom.type() + " extension could not make its endpoint: " + e.getMessage(), e);
    }
  }

  /**
   * Tries a custom source once (Test connection), adding to the steps as it goes.
   *
   * @param test the test's number, for its HTTP clients' names
   * @return what it found, or null when the kind offers no test
   * @throws UnsupportedIngestionException when no extension has the type, or the settings cannot be used
   * @throws Exception when the source failed
   */
  IngestExtension.@Nullable Tested test(
      CamelContext camel, Ingestion ingestion, Source.Custom custom, Secrets secrets, @Nullable ServiceClients http, long test, List<String> steps)
      throws Exception {
    Loaded loaded = loaded(custom);
    // A test keeps no state: it never reads or moves the ingestion's.
    StateStore none =
        new StateStore() {
          @Override
          public Optional<String> get(String ingestionId, String key) {
            return Optional.empty();
          }

          @Override
          public void put(String ingestionId, String key, String value) {}
        };
    Context context = context(camel, ingestion, custom, loaded, secrets, none, http, prefix -> ApiPollClient.name(prefix, "test", test));
    try {
      return loaded.extension().test(context, steps);
    } catch (InvalidSettingsException e) {
      throw new UnsupportedIngestionException(String.valueOf(e.getMessage()));
    }
  }

  private Loaded loaded(Source.Custom custom) {
    Loaded loaded = byType.get(custom.type());
    if (loaded == null) {
      throw new UnsupportedIngestionException("this runtime has no extension for the source type '" + custom.type() + "'");
    }
    return loaded;
  }

  private static Context context(
      CamelContext camel,
      Ingestion ingestion,
      Source.Custom custom,
      Loaded loaded,
      Secrets secrets,
      StateStore state,
      @Nullable ServiceClients http,
      Function<String, String> names) {
    Map<String, String> problems = loaded.definition().check(custom.settingMap(), ingestion.split());
    if (!problems.isEmpty()) {
      List<String> each = new ArrayList<>();
      problems.forEach((name, problem) -> each.add(name + " " + problem));
      throw new UnsupportedIngestionException("the settings of " + custom.type() + " cannot be used: " + String.join("; ", each));
    }
    Settings settings =
        new Settings(
            loaded.kind(),
            custom.settingMap(),
            name -> {
              try {
                return secrets.require(ingestion, new SecretRef(name));
              } catch (UnsupportedIngestionException e) {
                throw new InvalidSettingsException(String.valueOf(e.getMessage()));
              }
            });
    return new Context(camel, ingestion, settings, state, http, names);
  }

  private record Context(
      CamelContext camel, Ingestion ingestion, Settings settings, StateStore store, @Nullable ServiceClients clients, Function<String, String> names)
      implements ExtensionContext {

    @Override
    public ServiceClients http() {
      ServiceClients engine = clients;
      if (engine == null) {
        throw new UnsupportedIngestionException("this runtime was built without an HTTP client");
      }
      return engine;
    }

    @Override
    public String clientName(String prefix) {
      return names.apply(prefix);
    }

    @Override
    public State state() {
      String id = ingestion.id();
      return new State() {
        @Override
        public Optional<String> get(String key) throws IOException, InterruptedException {
          return store.get(id, key);
        }

        @Override
        public void put(String key, String value) throws IOException, InterruptedException {
          store.put(id, key, value);
        }
      };
    }
  }
}
