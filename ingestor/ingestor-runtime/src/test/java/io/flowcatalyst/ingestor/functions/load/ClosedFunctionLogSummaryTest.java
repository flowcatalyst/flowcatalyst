package io.flowcatalyst.ingestor.functions.load;

import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.framework.httpclient.HttpClients;
import io.flowcatalyst.function.FunctionAddress;
import io.flowcatalyst.function.FunctionContext;
import io.flowcatalyst.ingestor.functions.FakeControlPlane;
import io.flowcatalyst.ingestor.functions.context.ContextFactory;
import io.flowcatalyst.ingestor.functions.context.DbPools;
import io.flowcatalyst.ingestor.functions.context.LogBudget;
import io.flowcatalyst.ingestor.functions.context.OutboundClients;
import io.flowcatalyst.ingestor.functions.control.Control;
import io.flowcatalyst.ingestor.functions.control.Describe;
import io.flowcatalyst.ingestor.runtime.LogCapture;
import java.time.Clock;
import java.util.List;
import java.util.Map;
import org.junit.jupiter.api.Test;

/// Closing a function (a version unloaded, the host stopping) writes the "N log lines dropped" summary it has
/// pending, through the host's logger for it (docs/ingestor-gap-fixes.md B3).
class ClosedFunctionLogSummaryTest {

    @Test
    void aClosedFunctionWritesWhatItsLogDroppedSinceTheLastSummary() throws Exception {
        HttpClients clients = HttpClients.create();
        DbPools pools = new DbPools(DbPools.Limits.DEFAULTS, Clock.systemUTC());
        try {
            Control.FunctionDoc doc = new Control.FunctionDoc("fn_1", "app.flood", "app_1", null, null, false, null,
                    Map.of(), Map.of(), Map.of(), List.of());
            Describe describe = new Describe(1, List.of(), List.of(), List.of(), List.of(), List.of(), List.of(), List.of(), new Describe.Jvm("a.B"));
            ContextFactory contexts = new ContextFactory(
                    new OutboundClients(clients), Clock.systemUTC(), new FakeControlPlane(), pools, new LogBudget.Limits(1, 2));
            FunctionContext ctx = contexts.build(doc, 1, describe);
            LoadedFunction loaded = new LoadedFunction((in, c) -> null, () -> {}, FunctionAddress.parse("app.flood"), 1);
            loaded.attachContext(ctx);

            try (LogCapture log = new LogCapture()) {
                for (int i = 0; i < 12; i++) {
                    ctx.logger().log(System.Logger.Level.INFO, "line " + i);
                }
                // It falls silent and is unloaded within the minute.
                loaded.close();

                assertThat(log.records()).filteredOn(r -> r.logger().equals("fn.app.flood")).extracting(LogCapture.Record::level, LogCapture.Record::message)
                        .containsExactly(
                                org.assertj.core.groups.Tuple.tuple("INFO", "line 0"),
                                org.assertj.core.groups.Tuple.tuple("INFO", "line 1"),
                                org.assertj.core.groups.Tuple.tuple("WARN", "function app.flood dropped 10 log lines in the last minute (limit 1/s)"));
            }
        } finally {
            pools.close();
            clients.close();
        }
    }
}
