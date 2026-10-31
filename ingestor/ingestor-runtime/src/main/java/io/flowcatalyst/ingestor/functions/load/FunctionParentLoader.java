package io.flowcatalyst.ingestor.functions.load;

import java.io.IOException;
import java.net.URL;
import java.util.Collections;
import java.util.Enumeration;
import java.util.List;
import java.util.Objects;
import org.jspecify.annotations.Nullable;

/// The parent loader for every function's [JvmFunctionLoader]-built
/// `URLClassLoader`. Filters what a function can see through its parent
/// chain to exactly these things (docs/jvm-functions-in-ingestor.md R4):
///
/// 1. the JDK itself (routed to [ClassLoader#getPlatformClassLoader()]) —
///    wider than `java.*`/`javax.sql.*` alone, because a function using
///    `java.net.http` or XML needs the JDK's own implementation classes,
///    and none of those can come from the host's application class path;
/// 2. the API package `io.flowcatalyst.function`, **exactly** — not
///    subpackages, not a same-prefixed package like `io.flowcatalyst.functionx`;
/// 3. the framework: `io.flowcatalyst.framework.` and its subpackages;
/// 4. the Jackson the framework exposes: `tools.jackson.` and
///    `com.fasterxml.jackson.annotation.`;
/// 5. `org.slf4j.` and `org.jspecify.`.
///
/// (2) to (5) are routed to `hostLoader`, so the host and every function agree on
/// one class identity for them. Everything else is refused: no ingestor type
/// (`io.flowcatalyst.ingestor.`), no Camel, no JDBC driver.
/// `getResource`/`getResources` refuse the same way, including
/// `META-INF/services/*` — otherwise a function's own `ServiceLoader.load`
/// would discover the host's providers by resource even though it could
/// never load their classes, and fail with `ServiceConfigurationError`
/// instead of finding only what the function itself bundled.
///
/// Functions are first-party only (R6): this filter is fault isolation, not a sandbox.
public final class FunctionParentLoader extends ClassLoader {

    private static final String API_PACKAGE = "io.flowcatalyst.function";

    private static final List<String> JDK_CLASS_PREFIXES =
            List.of("java.", "javax.", "jdk.", "sun.", "com.sun.", "org.w3c.", "org.xml.");

    /// Package prefixes (dot-terminated, so `io.flowcatalyst.frameworkx.` never matches) the host serves.
    private static final List<String> HOST_CLASS_PREFIXES = List.of(
            "io.flowcatalyst.framework.",
            "tools.jackson.",
            "com.fasterxml.jackson.annotation.",
            "org.slf4j.",
            "org.jspecify.");

    private static final List<String> JDK_RESOURCE_PREFIXES =
            JDK_CLASS_PREFIXES.stream().map(prefix -> prefix.replace('.', '/')).toList();

    private final ClassLoader hostLoader;

    static {
        registerAsParallelCapable();
    }

    /// Production use: the host types are routed to whichever loader loaded
    /// this class, which is the host's own application loader — the one
    /// loader that has the API jar (and everything else) on its class path.
    /// The filter, not a narrower loader, is what keeps a function from
    /// seeing anything beyond the routed cases.
    public FunctionParentLoader() {
        this(Objects.requireNonNull(FunctionParentLoader.class.getClassLoader()));
    }

    /// @param hostLoader the loader consulted for the host types (rules 2 to 5).
    ///                    Package-private: tests use this to pin a specific
    ///                    host loader rather than relying on however the
    ///                    test JVM happened to load this class.
    FunctionParentLoader(ClassLoader hostLoader) {
        super(ClassLoader.getPlatformClassLoader());
        this.hostLoader = Objects.requireNonNull(hostLoader, "hostLoader");
    }

    @Override
    protected Class<?> loadClass(String name, boolean resolve) throws ClassNotFoundException {
        synchronized (getClassLoadingLock(name)) {
            Class<?> loaded = findLoadedClass(name);
            if (loaded == null) {
                if (isJdkType(name)) {
                    loaded = getParent().loadClass(name);
                } else if (isHostType(name)) {
                    loaded = hostLoader.loadClass(name);
                } else {
                    throw new ClassNotFoundException(name);
                }
            }
            if (resolve) {
                resolveClass(loaded);
            }
            return loaded;
        }
    }

    @Override
    public @Nullable URL getResource(String name) {
        return isJdkResource(name) ? getParent().getResource(name) : null;
    }

    @Override
    public Enumeration<URL> getResources(String name) throws IOException {
        return isJdkResource(name) ? getParent().getResources(name) : Collections.emptyEnumeration();
    }

    private static boolean isJdkType(String className) {
        for (String prefix : JDK_CLASS_PREFIXES) {
            if (className.startsWith(prefix)) return true;
        }
        return false;
    }

    private static boolean isHostType(String className) {
        if (isApiType(className)) return true;
        for (String prefix : HOST_CLASS_PREFIXES) {
            if (className.startsWith(prefix)) return true;
        }
        return false;
    }

    /// `io.flowcatalyst.function` exactly: the class's package (everything
    /// before the last `.`) must equal the API package, not merely start
    /// with it — `io.flowcatalyst.functionx.Y` is package
    /// `io.flowcatalyst.functionx`, which does not equal it.
    private static boolean isApiType(String className) {
        int lastDot = className.lastIndexOf('.');
        return lastDot > 0 && className.substring(0, lastDot).equals(API_PACKAGE);
    }

    private static boolean isJdkResource(String name) {
        for (String prefix : JDK_RESOURCE_PREFIXES) {
            if (name.startsWith(prefix)) return true;
        }
        return false;
    }
}
