package io.flowcatalyst.ingestor.functions;

import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.framework.auth.PlatformTokens;
import io.flowcatalyst.framework.auth.TestOidcProvider;
import io.flowcatalyst.framework.http.Routes;
import io.flowcatalyst.framework.httpclient.HttpClients;
import io.flowcatalyst.ingestor.functions.context.DbPools;
import io.flowcatalyst.ingestor.functions.context.LogBudget;
import java.time.Clock;
import java.time.Duration;
import java.time.InstantSource;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;

/// The host runs the one timer that writes its functions' "N log lines dropped" summaries while it is started
/// (docs/ingestor-gap-fixes.md B3): its budgets are timed, so a function's own log calls write none, and they stop
/// being timed when the host stops.
@Timeout(value = 60, threadMode = Timeout.ThreadMode.SEPARATE_THREAD)
class FunctionHostLogSummariesTest {

    @Test
    void aStartedHostRunsTheSummaryTimerAndAStoppedOneDoesNot() {
        TestOidcProvider platform = TestOidcProvider.start(InstantSource.system());
        HttpClients clients = HttpClients.create();
        try {
            PlatformTokens tokens = PlatformTokens.createAllowingPlainHttp(platform.issuer(), Duration.ofSeconds(5), InstantSource.system());
            FunctionHost host = FunctionHost.start("default", "runner-1", "test", new FakeControlPlane(), clients, tokens, new Routes(),
                    Clock.systemUTC(), DbPools.Limits.DEFAULTS, LogBudget.Limits.DEFAULTS);
            LogBudget budget = host.logBudgets().budget("app.hello");
            try {
                assertThat(budget.timed()).isTrue();
            } finally {
                host.close();
            }
            assertThat(budget.timed()).isFalse();
        } finally {
            clients.close();
            platform.close();
        }
    }
}
