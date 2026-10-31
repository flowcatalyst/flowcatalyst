package io.flowcatalyst.ingestor.functions.http;

import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.function.FunctionAddress;
import io.flowcatalyst.ingestor.functions.Fx;
import io.flowcatalyst.ingestor.functions.load.JvmFunctionLoader;
import io.flowcatalyst.ingestor.functions.load.Loaded;
import io.flowcatalyst.ingestor.functions.load.LoadedFunction;
import java.nio.file.Files;
import java.nio.file.Path;
import java.time.Duration;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.atomic.AtomicInteger;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

/// The LRU of versions loaded for calls that are not to the live version.
class PinnedVersionsTest {

    @TempDir
    Path dir;
    Path jar;
    PinnedVersions pinned;

    @BeforeEach
    void start() throws Exception {
        jar = dir.resolve("hello.jar");
        Files.write(jar, Fx.hello("p").bytes());
        pinned = new PinnedVersions();
    }

    private LoadedFunction load(int version) {
        return ((Loaded) new JvmFunctionLoader().load(jar, "fixture.app.Hello", FunctionAddress.parse("app.hello"), version)).function();
    }

    private static PinnedVersions.Key key(int version) {
        return new PinnedVersions.Key("app.hello", version);
    }

    private static void awaitClosed(LoadedFunction fn) throws InterruptedException {
        long end = System.nanoTime() + Duration.ofSeconds(5).toNanos();
        while (!fn.isClosed() && System.nanoTime() < end) {
            Thread.sleep(5);
        }
        assertThat(fn.isClosed()).isTrue();
    }

    @Test
    void aVersionIsLoadedOnceAndThenServedFromTheCache() {
        AtomicInteger loads = new AtomicInteger();

        LoadedFunction first = pinned.getOrLoad(key(1), "f", () -> {
            loads.incrementAndGet();
            return load(1);
        });
        LoadedFunction second = pinned.getOrLoad(key(1), "f", () -> {
            loads.incrementAndGet();
            return load(1);
        });

        assertThat(second).isSameAs(first);
        assertThat(loads.get()).isEqualTo(1);
    }

    @Test
    void concurrentCallsForOneVersionLoadItOnce() throws Exception {
        AtomicInteger loads = new AtomicInteger();
        CountDownLatch go = new CountDownLatch(1);
        List<Thread> threads = new ArrayList<>();
        List<LoadedFunction> got = new java.util.concurrent.CopyOnWriteArrayList<>();
        for (int i = 0; i < 30; i++) {
            threads.add(Thread.ofVirtual().start(() -> {
                try {
                    go.await();
                } catch (InterruptedException e) {
                    return;
                }
                got.add(pinned.getOrLoad(key(1), "f", () -> {
                    loads.incrementAndGet();
                    try {
                        Thread.sleep(100);
                    } catch (InterruptedException e) {
                        Thread.currentThread().interrupt();
                    }
                    return load(1);
                }));
            }));
        }
        go.countDown();
        for (Thread t : threads) {
            t.join();
        }

        assertThat(loads.get()).isEqualTo(1);
        assertThat(got).hasSize(30).doesNotContainNull();
        assertThat(got.stream().distinct().count()).isEqualTo(1);
    }

    @Test
    void aSlowLoadOfOneVersionDoesNotHoldUpAnother() throws Exception {
        CountDownLatch slowStarted = new CountDownLatch(1);
        CountDownLatch release = new CountDownLatch(1);
        Thread slow = Thread.ofVirtual().start(() -> pinned.getOrLoad(key(1), "f", () -> {
            slowStarted.countDown();
            try {
                release.await();
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
            }
            return load(1);
        }));
        slowStarted.await();

        long start = System.nanoTime();
        LoadedFunction other = pinned.getOrLoad(key(2), "f", () -> load(2));

        assertThat(other).isNotNull();
        assertThat(Duration.ofNanos(System.nanoTime() - start)).isLessThan(Duration.ofSeconds(2));
        release.countDown();
        slow.join();
    }

    @Test
    void aChangedFingerprintClosesTheStaleVersionAndLoadsAgain() throws Exception {
        LoadedFunction first = pinned.getOrLoad(key(1), "settings-1", () -> load(1));
        LoadedFunction second = pinned.getOrLoad(key(1), "settings-2", () -> load(1));

        assertThat(second).isNotSameAs(first);
        awaitClosed(first);
    }

    @Test
    void theLeastRecentlyUsedVersionIsClosedBeyondTheCapacity() throws Exception {
        List<LoadedFunction> loaded = new ArrayList<>();
        for (int v = 1; v <= PinnedVersions.CAPACITY; v++) {
            int version = v;
            loaded.add(pinned.getOrLoad(key(version), "f", () -> load(version)));
            Thread.sleep(2);
        }
        pinned.getOrLoad(key(1), "f", () -> load(1)); // touch version 1: version 2 is now the oldest

        pinned.getOrLoad(key(99), "f", () -> load(99));

        assertThat(pinned.size()).isEqualTo(PinnedVersions.CAPACITY);
        assertThat(pinned.peek(key(2))).isNull();
        awaitClosed(loaded.get(1));
        assertThat(pinned.peek(key(1))).isNotNull();
        assertThat(loaded.get(0).isClosed()).isFalse();
    }

    @Test
    void sweepClosesWhatTheDocumentNoLongerNames() throws Exception {
        LoadedFunction one = pinned.getOrLoad(key(1), "f", () -> load(1));
        LoadedFunction two = pinned.getOrLoad(key(2), "f", () -> load(2));

        pinned.sweep(k -> k.version() == 2);

        assertThat(pinned.peek(key(1))).isNull();
        awaitClosed(one);
        assertThat(pinned.peek(key(2))).isSameAs(two);
    }

    @Test
    void takeMovesTheInstanceOutWithoutClosingIt() {
        LoadedFunction one = pinned.getOrLoad(key(1), "f", () -> load(1));

        assertThat(pinned.take(key(1), "other-settings")).as("only with the settings it was proven with").isNull();
        assertThat(pinned.take(key(1), "f")).isSameAs(one);

        assertThat(one.isClosing()).isFalse();
        assertThat(pinned.peek(key(1))).isNull();
    }

    @Test
    void aVersionThatCannotBeLoadedIsNotCached() {
        assertThat(pinned.getOrLoad(key(1), "f", () -> null)).isNull();
        assertThat(pinned.size()).isZero();
        assertThat(pinned.getOrLoad(key(1), "f", () -> load(1))).isNotNull();
    }

    @Test
    void closeClosesEverything() {
        LoadedFunction one = pinned.getOrLoad(key(1), "f", () -> load(1));
        LoadedFunction two = pinned.getOrLoad(key(2), "f", () -> load(2));

        pinned.close();

        assertThat(one.isClosed()).isTrue();
        assertThat(two.isClosed()).isTrue();
        assertThat(pinned.size()).isZero();
    }
}
