package io.flowcatalyst.ingestor.functions.load;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import io.flowcatalyst.function.Function;
import java.io.IOException;
import java.net.URL;
import java.util.ArrayList;
import java.util.Collections;
import java.util.Enumeration;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.ValueSource;

/// Unit-level coverage of [FunctionParentLoader]'s own routing rules (R4): the JDK, `io.flowcatalyst.function`
/// exactly, `io.flowcatalyst.framework.` and its subpackages, the Jackson the framework exposes, `org.slf4j.` and
/// `org.jspecify.`; everything else is refused.
class FunctionParentLoaderTest {

    @Test
    void loadsAJdkClassFromThePlatformLoader() throws ClassNotFoundException {
        FunctionParentLoader loader = newLoader();
        Class<?> loaded = loader.loadClass("java.util.ArrayList");
        assertThat(loaded).isSameAs(ArrayList.class);
        assertThat(loaded.getClassLoader()).isNotEqualTo(loader);
    }

    @Test
    void loadsAnApiClassFromTheHostLoader() throws ClassNotFoundException {
        assertThat(newLoader().loadClass("io.flowcatalyst.function.Function")).isSameAs(Function.class);
    }

    @ParameterizedTest(name = "[{index}] serves {0} from the host")
    @ValueSource(strings = {
        "io.flowcatalyst.framework.http.Json",
        "io.flowcatalyst.framework.config.Setting",
        "tools.jackson.databind.ObjectMapper",
        "tools.jackson.core.JsonParser",
        "com.fasterxml.jackson.annotation.JsonProperty",
        "org.slf4j.Logger",
        "org.slf4j.LoggerFactory",
        "org.jspecify.annotations.Nullable"
    })
    void servesTheSharedLibrariesFromTheHost(String className) throws ClassNotFoundException {
        FunctionParentLoader loader = newLoader();
        Class<?> loaded = loader.loadClass(className);
        assertThat(loaded.getClassLoader()).as("defined by the host's loader, not the function's parent").isSameAs(getClass().getClassLoader());
    }

    @ParameterizedTest(name = "[{index}] rejects {0}")
    @ValueSource(strings = {
        "io.flowcatalyst.functionx.Y",
        "io.flowcatalyst.function.sub.Y",
        "io.flowcatalyst.functio",
        "io.flowcatalyst.frameworkx.Y",
        "io.flowcatalyst.framework",
        "io.flowcatalyst.ingestor.runtime.RuntimeModule",
        "io.flowcatalyst.ingestor.functions.load.LoadedFunction",
        "io.flowcatalyst.ingestor.contract.Ingestion",
        "com.example.lib.Version",
        "io.vertx.core.Vertx",
        "org.apache.camel.CamelContext",
        "org.eclipse.jetty.server.Server",
        "com.fasterxml.jackson.databind.ObjectMapper",
        "org.slf4jx.Logger"
    })
    void rejectsEverythingOutsideTheRoutedCases(String className) {
        FunctionParentLoader loader = newLoader();
        assertThatThrownBy(() -> loader.loadClass(className)).isInstanceOf(ClassNotFoundException.class);
    }

    @Test
    void getResourceRoutesOnlyJdkPrefixedNames() {
        FunctionParentLoader loader = newLoader();
        assertThat(loader.getResource("java/util/ArrayList.class")).isNotNull();
        assertThat(loader.getResource("META-INF/services/java.security.Provider")).isNull();
        assertThat(loader.getResource("io/flowcatalyst/function/Function.class")).isNull();
        assertThat(loader.getResource("org/slf4j/Logger.class")).isNull();
    }

    @Test
    void getResourcesNeverExposesServiceRegistrations() throws IOException {
        Enumeration<URL> resources = newLoader().getResources("META-INF/services/java.util.function.Supplier");
        assertThat(Collections.list(resources)).isEmpty();
    }

    private static FunctionParentLoader newLoader() {
        // The package-private constructor pins the host loader explicitly rather than relying on the loader that
        // defined FunctionParentLoader, which is what the no-arg constructor uses in production.
        return new FunctionParentLoader(FunctionParentLoaderTest.class.getClassLoader());
    }
}
