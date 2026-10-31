package io.flowcatalyst.function;

import java.io.BufferedReader;
import java.io.IOException;
import java.io.InputStream;
import java.io.InputStreamReader;
import java.io.UncheckedIOException;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.List;
import java.util.stream.Stream;

import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.Arguments;
import org.junit.jupiter.params.provider.MethodSource;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

/// Every row of the address table in `function-address-table.csv`, plus the
/// rendering and component rules of the two-label address
/// `{applicationCode}.{name}`.
class FunctionAddressTest {

    @Test
    void twoPartsAccepted() {
        FunctionAddress address = FunctionAddress.parse("orders.order-created");
        assertThat(address.application()).isEqualTo("orders");
        assertThat(address.name()).isEqualTo("order-created");
        assertThat(address.render()).isEqualTo("orders.order-created");
        assertThat(address.toString()).isEqualTo("orders.order-created");
    }

    @Test
    void nullRejected() {
        assertThatThrownBy(() -> FunctionAddress.parse(null)).isInstanceOf(IllegalArgumentException.class);
    }

    @Test
    void componentsAreValidatedByTheCanonicalConstructor() {
        assertThat(new FunctionAddress("orders", "order-created")).isEqualTo(FunctionAddress.parse("orders.order-created"));
        assertThatThrownBy(() -> new FunctionAddress(null, "x")).isInstanceOf(IllegalArgumentException.class);
        assertThatThrownBy(() -> new FunctionAddress("app", null)).isInstanceOf(IllegalArgumentException.class);
        assertThatThrownBy(() -> new FunctionAddress("a.b", "x")).isInstanceOf(IllegalArgumentException.class);
        assertThatThrownBy(() -> new FunctionAddress("app", "x.y")).isInstanceOf(IllegalArgumentException.class);
    }

    @Test
    void nameIsAtMost63CharactersButTheApplicationCodeHasNoCap() {
        String name63 = "a" + "b".repeat(62);
        String name64 = "a" + "b".repeat(63);
        assertThat(FunctionAddress.parse("app." + name63).name()).isEqualTo(name63);
        assertThatThrownBy(() -> FunctionAddress.parse("app." + name64)).isInstanceOf(IllegalArgumentException.class);
        // The platform's application-code rule is ^[a-z][a-z0-9-]*$ with no length cap.
        assertThat(FunctionAddress.parse(name64 + ".fn").application()).isEqualTo(name64);
    }

    @Test
    void renderEqualsParseInput() {
        assertThat(FunctionAddress.parse("a-1.b-2").render()).isEqualTo("a-1.b-2");
    }

    @ParameterizedTest(name = "[{index}] \"{0}\" -> accepted={1}")
    @MethodSource("sharedTable")
    void sharedTableRow(String raw, boolean accepted) {
        if (accepted) {
            assertThat(FunctionAddress.parse(raw).render()).isEqualTo(raw);
        } else {
            assertThatThrownBy(() -> FunctionAddress.parse(raw)).isInstanceOf(IllegalArgumentException.class);
        }
    }

    static Stream<Arguments> sharedTable() {
        List<Arguments> rows = new ArrayList<>();
        try (InputStream in = FunctionAddressTest.class.getResourceAsStream("/function-address-table.csv")) {
            assertThat(in).as("function-address-table.csv must be on the test classpath").isNotNull();
            try (BufferedReader reader = new BufferedReader(new InputStreamReader(in, StandardCharsets.UTF_8))) {
                String line;
                boolean sawHeader = false;
                while ((line = reader.readLine()) != null) {
                    if (line.isBlank() || line.startsWith("#")) continue;
                    if (!sawHeader) {
                        sawHeader = true;
                        continue;
                    }
                    int open = line.indexOf('"');
                    int close = line.lastIndexOf('"');
                    String raw = line.substring(open + 1, close);
                    boolean accepted = Boolean.parseBoolean(line.substring(close + 1).replace(",", "").trim());
                    rows.add(Arguments.of(raw, accepted));
                }
            }
        } catch (IOException e) {
            throw new UncheckedIOException(e);
        }
        assertThat(rows).as("function-address-table.csv must contain rows").isNotEmpty();
        return rows.stream();
    }
}
