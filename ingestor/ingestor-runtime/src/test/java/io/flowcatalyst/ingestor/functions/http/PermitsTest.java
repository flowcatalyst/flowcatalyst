package io.flowcatalyst.ingestor.functions.http;

import static org.assertj.core.api.Assertions.assertThat;

import java.util.ArrayList;
import java.util.List;
import java.util.Set;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.atomic.AtomicInteger;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;

/// Per-function permits: taken without waiting, released once, resized by a new limit, never more than the limit under
/// contention.
@Timeout(value = 60, threadMode = Timeout.ThreadMode.SEPARATE_THREAD) // a call that queues instead of answering 429 must fail, not hang
class PermitsTest {

    @Test
    void permitsAreTakenWithoutWaitingAndReleasedOnce() {
        Permits permits = new Permits();
        Permits.Grant a = permits.tryAcquire("app.f", 2);
        Permits.Grant b = permits.tryAcquire("app.f", 2);

        assertThat(a).isNotNull();
        assertThat(b).isNotNull();
        assertThat(permits.tryAcquire("app.f", 2)).as("none left: refused, not queued").isNull();
        assertThat(permits.available("app.f")).isZero();

        a.release();
        a.release(); // a second release gives nothing back
        assertThat(permits.available("app.f")).isEqualTo(1);
        assertThat(permits.tryAcquire("app.f", 2)).isNotNull();
    }

    @Test
    void functionsHaveTheirOwnPermits() {
        Permits permits = new Permits();
        permits.tryAcquire("app.a", 1);
        assertThat(permits.tryAcquire("app.a", 1)).isNull();
        assertThat(permits.tryAcquire("app.b", 1)).isNotNull();
    }

    @Test
    void aNewLimitReplacesTheSemaphoreAndOldGrantsReleaseIntoTheOldOne() {
        Permits permits = new Permits();
        Permits.Grant old = permits.tryAcquire("app.f", 1);
        assertThat(permits.tryAcquire("app.f", 1)).isNull();

        Permits.Grant fresh = permits.tryAcquire("app.f", 3);

        assertThat(fresh).isNotNull();
        assertThat(permits.available("app.f")).isEqualTo(2);
        old.release();
        assertThat(permits.available("app.f")).as("the old grant did not give the new semaphore a permit").isEqualTo(2);
    }

    @Test
    void functionsThatLeftTheDocumentAreForgotten() {
        Permits permits = new Permits();
        permits.tryAcquire("app.a", 1);
        permits.tryAcquire("app.b", 1);

        permits.retainOnly(Set.of("app.b"));

        assertThat(permits.available("app.a")).isEqualTo(-1);
        assertThat(permits.available("app.b")).isZero();
    }

    @Test
    void underContentionNeverMoreThanTheLimitAreHeld() throws Exception {
        Permits permits = new Permits();
        int limit = 5;
        AtomicInteger held = new AtomicInteger();
        AtomicInteger max = new AtomicInteger();
        CountDownLatch go = new CountDownLatch(1);
        List<Thread> threads = new ArrayList<>();
        for (int i = 0; i < 200; i++) {
            threads.add(Thread.ofVirtual().start(() -> {
                try {
                    go.await();
                } catch (InterruptedException e) {
                    return;
                }
                for (int j = 0; j < 200; j++) {
                    Permits.Grant g = permits.tryAcquire("app.f", limit);
                    if (g != null) {
                        max.accumulateAndGet(held.incrementAndGet(), Math::max);
                        held.decrementAndGet();
                        g.release();
                    }
                }
            }));
        }
        go.countDown();
        for (Thread t : threads) {
            t.join();
        }
        assertThat(max.get()).isLessThanOrEqualTo(limit);
        assertThat(permits.available("app.f")).isEqualTo(limit);
    }
}
