package io.flowcatalyst.ingestor.functions.context;

import static org.assertj.core.api.Assertions.assertThat;

import java.lang.reflect.Proxy;
import java.time.Duration;
import java.util.List;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.atomic.AtomicLong;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;
import org.slf4j.Logger;

/// The "N log lines dropped" summary is written on time (docs/ingestor-gap-fixes.md B3): one timer for the whole
/// host writes every function's pending summary once a minute, so a function that floods and then falls silent
/// still has its last minute reported; closing writes what is pending. Time for the budgets is a number the test
/// moves; the timer is a real thread.
@Timeout(value = 60, threadMode = Timeout.ThreadMode.SEPARATE_THREAD)
class LogBudgetsTest {

    record Line(String logger, String message, Thread thread) {
    }

    final List<Line> lines = new CopyOnWriteArrayList<>();
    final AtomicLong nanos = new AtomicLong(1_000_000_000L);

    /// An SLF4J logger named `name` that records what it is given, formatting `{}` itself.
    Logger recorder(String name) {
        return (Logger) Proxy.newProxyInstance(getClass().getClassLoader(), new Class<?>[] {Logger.class}, (proxy, method, args) -> {
            String m = method.getName();
            if (m.startsWith("is") && m.endsWith("Enabled")) {
                return true;
            }
            if (m.equals("getName")) {
                return name;
            }
            if (args == null || args.length == 0 || !(args[0] instanceof String format)) {
                return null;
            }
            Object[] params = args.length == 2 && args[1] instanceof Object[] array ? array : new Object[0];
            StringBuilder out = new StringBuilder();
            int next = 0;
            for (int i = 0; i < format.length(); i++) {
                if (format.startsWith("{}", i) && next < params.length) {
                    out.append(params[next++]);
                    i++;
                } else {
                    out.append(format.charAt(i));
                }
            }
            lines.add(new Line(name, out.toString(), Thread.currentThread()));
            return null;
        });
    }

    LogBudgets budgets() {
        return new LogBudgets(new LogBudget.Limits(1, 5), nanos::get, this::recorder);
    }

    HostLogger logger(LogBudgets budgets, String address) {
        return new HostLogger(recorder("fn." + address), address, budgets.budget(address));
    }

    List<Line> summaries() {
        return lines.stream().filter(l -> l.message().contains("dropped")).toList();
    }

    static void flood(HostLogger log, int lines) {
        for (int i = 0; i < lines; i++) {
            log.log(System.Logger.Level.INFO, "line " + i);
        }
    }

    static void await(java.util.function.BooleanSupplier condition) throws InterruptedException {
        long deadline = System.nanoTime() + Duration.ofSeconds(20).toNanos();
        while (!condition.getAsBoolean()) {
            assertThat(System.nanoTime()).as("waited 20 s").isLessThan(deadline);
            Thread.sleep(20);
        }
    }

    @Test
    void aFunctionThatFloodsThenFallsSilentHasItsDropsWrittenByTheHostsOneTimer() throws Exception {
        LogBudgets budgets = budgets();
        HostLogger chatty = logger(budgets, "app.chatty");
        HostLogger noisy = logger(budgets, "app.noisy");
        flood(chatty, 20);
        flood(noisy, 8);
        assertThat(lines).hasSize(10);

        // Neither logs again; the timer writes both.
        budgets.start(Duration.ofMillis(50));
        try {
            await(() -> summaries().size() == 2);
        } finally {
            budgets.close();
        }

        assertThat(summaries()).extracting(Line::logger, Line::message).containsExactlyInAnyOrder(
                org.assertj.core.groups.Tuple.tuple("fn.app.chatty", "function app.chatty dropped 15 log lines in the last minute (limit 1/s)"),
                org.assertj.core.groups.Tuple.tuple("fn.app.noisy", "function app.noisy dropped 3 log lines in the last minute (limit 1/s)"));
        // One thread for the whole host, not one a function.
        assertThat(summaries()).extracting(Line::thread).containsOnly(summaries().getFirst().thread());
        assertThat(summaries().getFirst().thread().isVirtual()).isTrue();
        assertThat(summaries().getFirst().thread().getName()).isEqualTo("function-log-summaries");
    }

    @Test
    void whileTheTimerRunsALogCallAfterTheMinuteAddsNoSecondSummaryAndClosingWritesWhatIsPending() {
        LogBudgets budgets = budgets();
        HostLogger chatty = logger(budgets, "app.chatty");
        budgets.start(Duration.ofHours(1));
        try {
            flood(chatty, 20);
            nanos.addAndGet(61_000_000_000L);
            chatty.log(System.Logger.Level.INFO, "after the minute");

            assertThat(summaries()).as("the timer writes it, at most once a minute").isEmpty();
        } finally {
            budgets.close();
        }
        assertThat(summaries()).singleElement().satisfies(l ->
                assertThat(l.message()).isEqualTo("function app.chatty dropped 15 log lines in the last minute (limit 1/s)"));
    }

    @Test
    void withoutTheTimerTheNextLogCallAfterTheMinuteWritesTheSummaryAsBefore() {
        LogBudgets budgets = budgets();
        HostLogger chatty = logger(budgets, "app.chatty");
        flood(chatty, 20);
        nanos.addAndGet(61_000_000_000L);
        chatty.log(System.Logger.Level.INFO, "after the minute");

        assertThat(summaries()).singleElement().satisfies(l -> assertThat(l.message()).contains("dropped 15 log lines"));
    }

    @Test
    void aFunctionThatDroppedNothingWritesNoSummary() {
        LogBudgets budgets = budgets();
        HostLogger quiet = logger(budgets, "app.quiet");
        flood(quiet, 3);
        budgets.flush();
        budgets.close();

        assertThat(summaries()).isEmpty();
    }
}
