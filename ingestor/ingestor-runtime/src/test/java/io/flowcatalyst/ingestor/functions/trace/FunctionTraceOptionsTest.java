package io.flowcatalyst.ingestor.functions.trace;

import static org.assertj.core.api.Assertions.assertThat;

import java.util.Map;
import org.junit.jupiter.api.Test;

/// The three environment settings, as the Go runner reads them.
class FunctionTraceOptionsTest {

    @Test
    void theDefaultsApplyWithNothingSet() {
        FunctionTraceOptions o = FunctionTraceOptions.fromEnvironment(name -> null);

        assertThat(o.queueRecords()).isEqualTo(5000);
        assertThat(o.queueBytes()).isEqualTo(32L << 20);
        assertThat(o.bodyBytes()).isEqualTo(65536);
        assertThat(o).isEqualTo(FunctionTraceOptions.DEFAULTS);
    }

    @Test
    void theEnvironmentSetsThem() {
        Map<String, String> env = Map.of("FC_FN_TRACE_QUEUE", "100", "FC_FN_TRACE_QUEUE_BYTES", "2097152", "FC_FN_TRACE_BODY_BYTES", "1024");

        FunctionTraceOptions o = FunctionTraceOptions.fromEnvironment(env::get);

        assertThat(o.queueRecords()).isEqualTo(100);
        assertThat(o.queueBytes()).isEqualTo(2L << 20);
        assertThat(o.bodyBytes()).isEqualTo(1024);
    }

    @Test
    void aValueThatIsNotANumberOrIsOutOfRangeIsIgnored() {
        Map<String, String> env = Map.of("FC_FN_TRACE_QUEUE", "lots", "FC_FN_TRACE_QUEUE_BYTES", "10", "FC_FN_TRACE_BODY_BYTES", "-5");

        assertThat(FunctionTraceOptions.fromEnvironment(env::get)).isEqualTo(FunctionTraceOptions.DEFAULTS);
    }
}
