package io.flowcatalyst.ingestor.functions.load;

import static io.flowcatalyst.ingestor.functions.load.TestSupport.ADDRESS;
import static io.flowcatalyst.ingestor.functions.load.TestSupport.context;
import static io.flowcatalyst.ingestor.functions.load.TestSupport.isAck;
import static io.flowcatalyst.ingestor.functions.load.TestSupport.request;
import static org.assertj.core.api.Assertions.assertThat;

import java.nio.file.Path;
import org.junit.jupiter.api.io.TempDir;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.ValueSource;

/// A function cannot load a host-only class, and does see the host's framework, Jackson, SLF4J and JSpecify. The
/// check runs **inside** the function (its own `getClassLoader()`, not the test's): the test's own loader can see
/// all of these, so only a check made from inside the isolated function loader proves anything.
///
/// `io.flowcatalyst.ingestor.runtime.RuntimeModule` is the target that also pins the prefix boundary: widening the
/// parent loader's framework match from `io.flowcatalyst.framework.` to `io.flowcatalyst.` would let it through.
class HostOnlyClassTest {

    @ParameterizedTest(name = "[{index}] cannot load {0}")
    @ValueSource(strings = {
        "io.vertx.core.Vertx",
        "io.flowcatalyst.ingestor.runtime.RuntimeModule",
        "io.flowcatalyst.ingestor.functions.load.LoadedFunction",
        "org.apache.camel.CamelContext",
        "org.eclipse.jetty.server.Server",
        "com.zaxxer.hikari.HikariDataSource",
        "org.postgresql.Driver"
    })
    void functionCannotLoadAHostOnlyClass(String target, @TempDir Path dir) throws Exception {
        Path jar = FixtureJars.builder()
                .source("fixture.l3.HostOnlyProbe", """
                        package fixture.l3;
                        import io.flowcatalyst.function.*;
                        public final class HostOnlyProbe implements Function {
                            public Result handle(Request in, FunctionContext ctx) {
                                try {
                                    Class.forName("%s", false, this.getClass().getClassLoader());
                                    return Result.fail("host-only class loaded unexpectedly");
                                } catch (ClassNotFoundException e) {
                                    return Result.ack();
                                }
                            }
                        }
                        """.formatted(target))
                .build(TestSupport.tempJar(dir, "hostonly-" + Math.abs(target.hashCode())));

        LoadOutcome outcome = new JvmFunctionLoader().load(jar, "fixture.l3.HostOnlyProbe", ADDRESS, 1);

        assertThat(outcome).isInstanceOf(Loaded.class);
        try (LoadedFunction function = ((Loaded) outcome).function()) {
            assertThat(isAck(function.invoke(request(), context()))).as(target + " must not be loadable from inside the function").isTrue();
        }
    }

    @ParameterizedTest(name = "[{index}] can load {0} from the host")
    @ValueSource(strings = {
        "io.flowcatalyst.framework.http.Json",
        "tools.jackson.databind.ObjectMapper",
        "com.fasterxml.jackson.annotation.JsonProperty",
        "org.slf4j.LoggerFactory",
        "org.jspecify.annotations.Nullable"
    })
    void functionSeesTheHostsSharedLibraries(String target, @TempDir Path dir) throws Exception {
        Path jar = FixtureJars.builder()
                .source("fixture.l3.SharedProbe", """
                        package fixture.l3;
                        import io.flowcatalyst.function.*;
                        public final class SharedProbe implements Function {
                            public Result handle(Request in, FunctionContext ctx) throws Exception {
                                Class<?> c = Class.forName("%s", false, this.getClass().getClassLoader());
                                return c.getClassLoader() == ClassLoader.getSystemClassLoader()
                                        ? Result.ack() : Result.fail("not the host's: " + c.getClassLoader());
                            }
                        }
                        """.formatted(target))
                .build(TestSupport.tempJar(dir, "shared-" + Math.abs(target.hashCode())));

        LoadOutcome outcome = new JvmFunctionLoader().load(jar, "fixture.l3.SharedProbe", ADDRESS, 1);

        assertThat(outcome).isInstanceOf(Loaded.class);
        try (LoadedFunction function = ((Loaded) outcome).function()) {
            assertThat(isAck(function.invoke(request(), context()))).as(target + " must be the host's own class").isTrue();
        }
    }
}
