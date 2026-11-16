package io.flowcatalyst.ingestor.functions.load;

import io.flowcatalyst.function.FunctionApi;

import java.nio.file.Files;
import java.nio.file.Path;

import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.ValueSource;

import static io.flowcatalyst.ingestor.functions.load.TestSupport.ADDRESS;
import static org.assertj.core.api.Assertions.assertThat;

/// A function jar says which major of the function API it was built for in its manifest
/// ([FunctionApi#MANIFEST_ATTRIBUTE]). The host loads a jar of its own major, and one that declares none
/// (taken as major 1: every function deployed before the attribute declares none); it refuses another
/// major, or a value that is not a version, as `API_MAJOR`, before any class of the jar is loaded.
class ApiMajorTest {

    private static final String ENTRYPOINT = "fixture.api.Probe";

    @Test
    void aJarOfTheHostsMajorLoads(@TempDir Path dir) throws Exception {
        Path jar = probe(dir.resolve("probe.marker"))
                .manifestAttribute(FunctionApi.MANIFEST_ATTRIBUTE, String.valueOf(FunctionApi.MAJOR))
                .build(TestSupport.tempJar(dir, "same-major"));

        LoadOutcome outcome = new JvmFunctionLoader().load(jar, ENTRYPOINT, ADDRESS, 1);

        assertThat(outcome).isInstanceOf(Loaded.class);
        ((Loaded) outcome).function().close();
    }

    @Test
    void aJarOfAnOlderMinorOfTheHostsMajorLoads(@TempDir Path dir) throws Exception {
        Path jar = probe(dir.resolve("probe.marker"))
                .manifestAttribute(FunctionApi.MANIFEST_ATTRIBUTE, FunctionApi.MAJOR + ".0")
                .build(TestSupport.tempJar(dir, "same-major-minor"));

        LoadOutcome outcome = new JvmFunctionLoader().load(jar, ENTRYPOINT, ADDRESS, 1);

        assertThat(outcome).isInstanceOf(Loaded.class);
        ((Loaded) outcome).function().close();
    }

    @Test
    void aJarThatDeclaresNoMajorIsTakenAsMajor1AndLoads(@TempDir Path dir) throws Exception {
        Path jar = probe(dir.resolve("probe.marker")).build(TestSupport.tempJar(dir, "undeclared"));

        LoadOutcome outcome = new JvmFunctionLoader().load(jar, ENTRYPOINT, ADDRESS, 1);

        assertThat(JvmFunctionLoader.UNDECLARED_API_MAJOR).isEqualTo(1);
        assertThat(FunctionApi.MAJOR)
                .as("this host serves major 1, so a jar without the attribute loads; on another major this test must change")
                .isEqualTo(1);
        assertThat(outcome).isInstanceOf(Loaded.class);
        ((Loaded) outcome).function().close();
    }

    @Test
    void aJarOfAnotherMajorIsRefusedNamingTheFunctionAndBothMajorsBeforeAnyClassLoads(@TempDir Path dir) throws Exception {
        Path marker = dir.resolve("probe.marker");
        int other = FunctionApi.MAJOR + 1;
        Path jar = probe(marker)
                .manifestAttribute(FunctionApi.MANIFEST_ATTRIBUTE, String.valueOf(other))
                .build(TestSupport.tempJar(dir, "other-major"));

        LoadOutcome outcome = new JvmFunctionLoader().load(jar, ENTRYPOINT, ADDRESS, 7);

        assertThat(outcome).isEqualTo(new Refused(Reason.API_MAJOR,
                "function fixture.probe v7 was built for function API " + other
                        + "; this runtime serves function API " + FunctionApi.MAJOR
                        + ". Rebuild it against function-api " + FunctionApi.MAJOR + ".x, or run it on a runtime that serves " + other));
        assertThat(Files.exists(marker)).as("refused before the entrypoint class is loaded").isFalse();
    }

    @ParameterizedTest
    @ValueSource(strings = {"one", "", "v1"})
    void aDeclarationThatIsNotAVersionIsRefused(String declared, @TempDir Path dir) throws Exception {
        Path jar = probe(dir.resolve("probe.marker"))
                .manifestAttribute(FunctionApi.MANIFEST_ATTRIBUTE, declared)
                .build(TestSupport.tempJar(dir, "not-a-version"));

        LoadOutcome outcome = new JvmFunctionLoader().load(jar, ENTRYPOINT, ADDRESS, 1);

        assertThat(outcome).isInstanceOf(Refused.class);
        assertThat(((Refused) outcome).reason()).isEqualTo(Reason.API_MAJOR);
        assertThat(((Refused) outcome).detail())
                .contains("function fixture.probe v1")
                .contains("'" + declared + "', which is not a version number");
    }

    @Test
    void onlyTheApiMajorRefusalSendsItsDetailToThePlatform() {
        for (Reason reason : Reason.values()) {
            assertThat(reason.detailReported()).as(reason.name()).isEqualTo(reason == Reason.API_MAJOR);
        }
        assertThat(Reason.API_MAJOR.transientReason()).as("the same bytes fail the same way: FAILED at once").isFalse();
    }

    /// A function whose class writes `marker` when it is initialised: proof that a refused jar's classes
    /// were never loaded.
    private static FixtureJars.Builder probe(Path marker) {
        String markerPath = marker.toString().replace("\\", "\\\\");
        return FixtureJars.builder()
                .source(ENTRYPOINT, """
                        package fixture.api;
                        import io.flowcatalyst.function.*;
                        public final class Probe implements Function {
                            static {
                                try {
                                    java.nio.file.Files.write(java.nio.file.Path.of("%s"), new byte[] {1});
                                } catch (java.io.IOException e) {
                                    throw new RuntimeException(e);
                                }
                            }
                            public Result handle(Request in, FunctionContext ctx) { return Result.ack(); }
                        }
                        """.formatted(markerPath));
    }
}
