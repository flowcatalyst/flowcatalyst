package io.flowcatalyst.ingestor.functions.context;

import static org.assertj.core.api.Assertions.assertThat;

import java.lang.reflect.Proxy;
import java.util.List;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.atomic.AtomicLong;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;
import org.slf4j.Logger;

/// A function's log is bounded (docs/ingestor-failure-pacing.md R6): a token bucket per address, a cut at 8 KiB,
/// and one WARN a minute for what was dropped. Time is a number the test moves.
@Timeout(value = 60, threadMode = Timeout.ThreadMode.SEPARATE_THREAD)
class HostLoggerBudgetTest {

    record Line(String level, String message, Throwable cause) {
    }

    final List<Line> lines = new CopyOnWriteArrayList<>();
    final AtomicLong nanos = new AtomicLong(1_000_000_000L);

    /// An SLF4J logger that records what it is given, formatting `{}` itself.
    Logger recorder() {
        return (Logger) Proxy.newProxyInstance(getClass().getClassLoader(), new Class<?>[] {Logger.class}, (proxy, method, args) -> {
            String name = method.getName();
            if (name.startsWith("is") && name.endsWith("Enabled")) {
                return true;
            }
            if (name.equals("getName")) {
                return "fn.app.chatty";
            }
            if (args == null || args.length == 0 || !(args[0] instanceof String format)) {
                return null;
            }
            Throwable cause = null;
            StringBuilder out = new StringBuilder();
            Object[] params;
            if (args.length == 2 && args[1] instanceof Object[] array) {
                params = array;
            } else if (args.length == 2 && args[1] instanceof Throwable t) {
                cause = t;
                params = new Object[0];
            } else {
                params = new Object[0];
            }
            int next = 0;
            for (int i = 0; i < format.length(); i++) {
                if (format.startsWith("{}", i) && next < params.length) {
                    out.append(params[next++]);
                    i++;
                } else {
                    out.append(format.charAt(i));
                }
            }
            lines.add(new Line(name, out.toString(), cause));
            return null;
        });
    }

    HostLogger logger(int rate, int burst) {
        return new HostLogger(recorder(), "app.chatty", new LogBudget(new LogBudget.Limits(rate, burst), nanos::get));
    }

    @Test
    void aHundredThousandLinesInASecondEmitAboutAThousandAndOneHundred() {
        HostLogger log = logger(100, 1000);

        for (int i = 0; i < 100_000; i++) {
            log.log(System.Logger.Level.INFO, "line " + i);
            nanos.addAndGet(10_000); // 100,000 lines over one second
        }
        assertThat(lines).as("budget: 1,000 burst + 100 a second").hasSizeBetween(1000, 1110);
        assertThat(lines).noneMatch(l -> l.message().contains("dropped"));

        // a minute on, the next line carries the one summary of what was dropped
        nanos.addAndGet(60_000_000_000L);
        log.log(System.Logger.Level.INFO, "after");
        List<Line> summaries = lines.stream().filter(l -> l.message().contains("dropped")).toList();
        assertThat(summaries).hasSize(1);
        assertThat(summaries.get(0).level()).isEqualTo("warn");
        assertThat(summaries.get(0).message()).startsWith("function app.chatty dropped 9").endsWith("log lines in the last minute (limit 100/s)");
    }

    @Test
    void theBudgetRefillsAtTheRate() {
        HostLogger log = logger(100, 10);
        for (int i = 0; i < 50; i++) {
            log.log(System.Logger.Level.INFO, "a");
        }
        assertThat(lines).hasSize(10);

        nanos.addAndGet(100_000_000L); // 0.1 s: ten more tokens
        for (int i = 0; i < 50; i++) {
            log.log(System.Logger.Level.INFO, "b");
        }
        assertThat(lines).hasSize(20);
    }

    @Test
    void aLevelThatIsNotLoggedCostsNothing() {
        HostLogger log = new HostLogger(recorder(), "app.chatty", new LogBudget(new LogBudget.Limits(1, 1), nanos::get));
        for (int i = 0; i < 100; i++) {
            log.log(System.Logger.Level.OFF, "never");
        }
        log.log(System.Logger.Level.INFO, "kept");
        assertThat(lines).hasSize(1);
    }

    @Test
    void aMessageOverEightKiBIsCut() {
        HostLogger log = logger(100, 1000);

        log.log(System.Logger.Level.INFO, "x".repeat(20_000));
        log.log(System.Logger.Level.INFO, "y".repeat(8192));

        assertThat(lines.get(0).message()).hasSize(8192 + "… (cut, 20000 bytes)".length()).endsWith("… (cut, 20000 bytes)");
        assertThat(lines.get(1).message()).as("exactly 8 KiB is kept whole").hasSize(8192);
    }

    @Test
    void aThrowableIsOneLineKeepsItsStackAndItsLongMessageIsCut() {
        HostLogger log = logger(100, 3);
        Throwable t = new IllegalStateException("z".repeat(20_000));
        StackTraceElement[] stack = t.getStackTrace();

        log.log(System.Logger.Level.ERROR, "failed", t);
        log.log(System.Logger.Level.ERROR, "failed again", new IllegalStateException("short"));

        assertThat(lines).hasSize(2);
        assertThat(lines.get(0).cause().getMessage()).startsWith("java.lang.IllegalStateException: zzz").endsWith("(cut, 20000 bytes)");
        assertThat(lines.get(0).cause().getStackTrace()).isEqualTo(stack);
        assertThat(lines.get(1).cause()).isInstanceOf(IllegalStateException.class);
    }

    @Test
    void allVersionsOfAnAddressShareOneBudget() {
        LogBudget shared = new LogBudget(new LogBudget.Limits(100, 5), nanos::get);
        HostLogger v1 = new HostLogger(recorder(), "app.chatty", shared);
        HostLogger v2 = new HostLogger(recorder(), "app.chatty", shared);

        for (int i = 0; i < 10; i++) {
            (i % 2 == 0 ? v1 : v2).log(System.Logger.Level.INFO, "m");
        }

        assertThat(lines).hasSize(5);
    }
}
