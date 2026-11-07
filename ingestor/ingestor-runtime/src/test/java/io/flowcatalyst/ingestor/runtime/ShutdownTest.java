package io.flowcatalyst.ingestor.runtime;

import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.ingestor.contract.ConnectionTest;
import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.JobRecord;
import io.flowcatalyst.ingestor.contract.PollAnswer;
import io.flowcatalyst.ingestor.contract.RuntimeReport;
import io.flowcatalyst.ingestor.contract.SecretRef;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.contract.Split;
import io.flowcatalyst.ingestor.contract.Target;
import java.nio.file.Files;
import java.nio.file.Path;
import java.time.Duration;
import java.time.Instant;
import java.time.InstantSource;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.TreeMap;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.function.Function;
import java.util.stream.Stream;
import org.apache.camel.CamelContext;
import org.apache.camel.Consumer;
import org.apache.camel.Endpoint;
import org.apache.camel.Processor;
import org.apache.camel.Producer;
import org.apache.camel.Route;
import org.apache.camel.impl.DefaultCamelContext;
import org.apache.camel.spi.CamelEvent;
import org.apache.camel.support.DefaultEndpoint;
import org.apache.camel.support.EventNotifierSupport;
import org.apache.camel.support.ScheduledPollConsumer;
import org.apache.camel.support.service.ServiceHelper;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;
import org.junit.jupiter.api.io.TempDir;

/**
 * Stopping routes and shutting down (docs/ingestor-gap-fixes.md, A1 to A3): the order of the
 * shutdown, a shutdown that does not wait for a reconcile stuck starting a route, and routes drained
 * at the same time rather than one after another.
 */
@Timeout(120)
class ShutdownTest {

  /** Every step of a shutdown, and every goodbye the platform saw, in order. */
  final List<String> log = new CopyOnWriteArrayList<>();

  /** The platform's runtime API held in memory; a release is written to the log. */
  final class Platform implements RuntimeApi {
    volatile List<Ingestion> active = List.of();
    final List<RuntimeLoopTest.FakePlatform.Release> releases = new CopyOnWriteArrayList<>();
    /** Run when a release arrives, before it is answered. */
    volatile Runnable onRelease = () -> {};

    @Override
    public Polled poll(Duration timeout) {
      return new Polled(new PollAnswer(Instant.EPOCH, Duration.ofMinutes(3), active, List.of(), List.of()), List.of(), List.of());
    }

    @Override
    public void report(RuntimeReport report, Duration timeout) {}

    @Override
    public int release(List<String> ingestionIds, boolean goodbye, Duration timeout) {
      onRelease.run();
      log.add((goodbye ? "goodbye " : "release ") + ingestionIds.stream().sorted().toList());
      releases.add(new RuntimeLoopTest.FakePlatform.Release(List.copyOf(ingestionIds), goodbye, timeout));
      return ingestionIds.size();
    }
  }

  final Platform platform = new Platform();
  /** How long each batch of an ingestion takes. */
  final Map<String, Duration> slow = new ConcurrentHashMap<>();
  /** A batch of the ingestion waits for its latch. */
  final Map<String, CountDownLatch> hold = new ConcurrentHashMap<>();
  /** Counted down when a batch of the ingestion begins. */
  final Map<String, CountDownLatch> begun = new ConcurrentHashMap<>();
  /** The payloads whose jobs were taken. */
  final List<String> taken = new CopyOnWriteArrayList<>();
  /** Endpoints other than {@code direct:<id>}, by ingestion id. */
  final Map<String, Function<CamelContext, Endpoint>> special = new ConcurrentHashMap<>();
  final ExchangeCounters counters = new ExchangeCounters(InstantSource.system());
  CamelContext camel;
  Reconciler reconciler;
  RuntimeLoop loop;
  /** Feeders stop when their route no longer takes anything. */
  final List<Thread> feeders = new ArrayList<>();

  @BeforeEach
  void start() {
    camel = new DefaultCamelContext();
    camel.getManagementStrategy()
        .addEventNotifier(
            new EventNotifierSupport() {
              @Override
              public void notify(CamelEvent event) {
                if (event instanceof CamelEvent.RouteStoppedEvent stopped) {
                  log.add("stopped " + stopped.getRoute().getRouteId());
                }
              }
            });
    camel.start();
    JobSink sink =
        (ingestion, jobs) -> {
          CountDownLatch began = begun.get(ingestion.id());
          if (began != null) {
            began.countDown();
          }
          Duration takes = slow.get(ingestion.id());
          if (takes != null) {
            Thread.sleep(takes.toMillis());
          }
          CountDownLatch wait = hold.get(ingestion.id());
          if (wait != null) {
            wait.await();
          }
          jobs.stream().map(JobRecord::payload).forEach(taken::add);
        };
    SourceEndpoints endpoints =
        (context, ingestion) -> {
          Function<CamelContext, Endpoint> made = special.get(ingestion.id());
          return made != null ? made.apply(context) : context.getEndpoint("direct:" + ingestion.id());
        };
    reconciler = new Reconciler(camel, ingestion -> new IngestionRoute(ingestion, endpoints, sink, counters));
    loop =
        new RuntimeLoop(
            "runtime-1",
            Duration.ofMinutes(1),
            platform,
            reconciler,
            counters,
            request -> new ConnectionTest(request.id(), ConnectionTest.Outcome.RESULTS, List.of(), null, null),
            InstantSource.system(),
            System::nanoTime);
  }

  @AfterEach
  void stop() throws InterruptedException {
    hold.values().forEach(CountDownLatch::countDown);
    camel.stop();
    for (Thread feeder : feeders) {
      feeder.join(5_000);
    }
  }

  /** A polled (single-owner) ingestion; the test reads it from {@code direct:<id>} unless told otherwise. */
  private static Ingestion polled(String id) {
    return SourceHarness.ingestion(
        id,
        1,
        new Source.Sftp("sftp.acme.example", 22, "/out", "ingest", new SecretRef("sftp"), null, null, Duration.ofMinutes(1)),
        new Split.None(),
        new Target("logistics:orders:file:map", "https://f.example/map", "pool_1", null, null, Target.Mode.IMMEDIATE, 1),
        false);
  }

  /** A shared (every runtime) ingestion. */
  private static Ingestion queue(String id) {
    return SourceHarness.ingestion(
        id, 1, new Source.Sqs("eu-west-1", id, null), new Split.None(), SourceHarness.target(Target.Mode.BLOCK_ON_ERROR, 1), false);
  }

  /** Sends one payload to the ingestion on a thread of its own; a refusal (the route stopped taking) is ignored. */
  private Thread send(String id, String body) {
    return Thread.ofPlatform()
        .start(
            () -> {
              try (var producer = camel.createProducerTemplate()) {
                producer.sendBody("direct:" + id + "?block=false", body);
              } catch (Exception e) {
                // Cut off, or refused once the route no longer takes anything.
              }
            });
  }

  /** Sends payloads to the ingestion one after another, as a source with more to read does, until it refuses one. */
  private void feed(String id) {
    feeders.add(
        Thread.ofVirtual()
            .start(
                () -> {
                  try (var producer = camel.createProducerTemplate()) {
                    for (int i = 0; ; i++) {
                      producer.sendBody("direct:" + id + "?block=false", id + "-" + i);
                    }
                  } catch (Exception e) {
                    // The route stopped taking: a suspended direct consumer refuses at once.
                  }
                }));
  }

  private RuntimeModule.Parts parts() {
    return new RuntimeModule.Parts(
        () -> log.add("functions"),
        () -> log.add("watchdog"),
        () -> {
          log.add("loop");
          loop.close();
        },
        reconciler,
        loop::goodbye,
        within -> log.add("traces"));
  }

  /** Runs a shutdown with a 10 s bound on a thread of its own, failing when it has not returned within {@code limit}. */
  private static void stopWithin(RuntimeModule.Parts parts, Duration limit) throws Exception {
    CompletableFuture<Void> done = new CompletableFuture<>();
    Thread.ofPlatform()
        .daemon()
        .start(
            () -> {
              try {
                RuntimeModule.stop(parts, Duration.ofSeconds(10));
                done.complete(null);
              } catch (Throwable t) {
                done.completeExceptionally(t);
              }
            });
    done.get(limit.toMillis(), TimeUnit.MILLISECONDS);
  }

  private boolean suspended(String routeId) {
    Route route = camel.getRoute(routeId);
    return route != null && ServiceHelper.isSuspended(route.getConsumer());
  }

  // A3: the order.

  @Test
  void shutdownStopsTheFunctionHostTheWatchdogTheLoopThenTheRoutesThenSaysGoodbyeThenFlushesTraces() throws Exception {
    ListingEndpoint listing = new ListingEndpoint(camel);
    special.put("listing", context -> listing);
    platform.active = List.of(polled("files"), polled("other"), polled("listing"), queue("queue"));
    loop.tick();
    assertThat(listing.entered.await(10, TimeUnit.SECONDS)).isTrue();
    // Taken after the routes stopped and before the goodbye was answered.
    List<String> routesAtGoodbye = new CopyOnWriteArrayList<>();
    platform.onRelease = () -> camel.getRoutes().forEach(r -> routesAtGoodbye.add(r.getRouteId()));

    try {
      RuntimeModule.stop(parts(), Duration.ofSeconds(10));
    } finally {
      listing.leave.countDown();
    }

    List<String> seen = List.copyOf(log);
    assertThat(seen.subList(0, 3)).containsExactly("functions", "watchdog", "loop");
    assertThat(seen.subList(3, 7)).containsExactlyInAnyOrder("stopped files", "stopped other", "stopped listing", "stopped queue");
    // listing was stopped, but the poll it had begun was still running: its lease is left to run out.
    assertThat(seen.subList(7, seen.size())).containsExactly("goodbye [files, other]", "traces");
    assertThat(routesAtGoodbye).isEmpty();
    assertThat(platform.releases).singleElement().satisfies(r -> assertThat(r.timeout()).isLessThanOrEqualTo(RuntimeLoop.GOODBYE_WITHIN));

    // Shut down: a round that comes after (one the application left behind) starts nothing again.
    loop.tick();
    assertThat(camel.getRoutes()).isEmpty();
    assertThat(reconciler.running()).isEmpty();
  }

  @Test
  void aFunctionHostSlowToCloseDoesNotHoldUpTheRoutesOrTheGoodbye() throws Exception {
    platform.active = List.of(polled("files"));
    loop.tick();
    CountDownLatch closed = new CountDownLatch(1);
    RuntimeModule.Parts slowHost =
        new RuntimeModule.Parts(
            () -> {
              // In-flight calls the host waits for, up to 30 s each.
              try {
                closed.await();
              } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
              }
            },
            () -> {},
            loop::close,
            reconciler,
            loop::goodbye,
            within -> {});
    try {
      long began = System.nanoTime();
      stopWithin(slowHost, Duration.ofSeconds(15));
      assertThat(Duration.ofNanos(System.nanoTime() - began)).isLessThan(RuntimeModule.FUNCTIONS_CLOSE_WAIT.plusSeconds(2));
    } finally {
      closed.countDown();
    }
    assertThat(platform.releases).singleElement().satisfies(r -> assertThat(r.ingestionIds()).containsExactly("files"));
  }

  // A1: a reconcile stuck starting a route.

  /** A polled source whose poll does not return until let go, ignoring interrupts: a listing in progress. */
  static final class ListingEndpoint extends DefaultEndpoint {
    final CountDownLatch entered = new CountDownLatch(1);
    final CountDownLatch leave = new CountDownLatch(1);

    ListingEndpoint(CamelContext camel) {
      setCamelContext(camel);
    }

    @Override
    protected String createEndpointUri() {
      return "ingest-listing:slow";
    }

    @Override
    public Producer createProducer() {
      throw new UnsupportedOperationException();
    }

    @Override
    public Consumer createConsumer(Processor processor) {
      ScheduledPollConsumer consumer =
          new ScheduledPollConsumer(this, processor) {
            @Override
            protected int poll() {
              entered.countDown();
              boolean interrupted = false;
              while (true) {
                try {
                  leave.await();
                  break;
                } catch (InterruptedException e) {
                  interrupted = true;
                }
              }
              if (interrupted) {
                Thread.currentThread().interrupt();
              }
              return 0;
            }
          };
      consumer.setInitialDelay(0);
      consumer.setDelay(50);
      return consumer;
    }
  }

  /** A source whose consumer fails to suspend. */
  static final class StubbornEndpoint extends DefaultEndpoint {
    StubbornEndpoint(CamelContext camel) {
      setCamelContext(camel);
    }

    @Override
    protected String createEndpointUri() {
      return "ingest-stubborn:slow";
    }

    @Override
    public Producer createProducer() {
      throw new UnsupportedOperationException();
    }

    @Override
    public Consumer createConsumer(Processor processor) {
      return new StubbornConsumer(this, processor);
    }
  }

  static final class StubbornConsumer extends org.apache.camel.support.DefaultConsumer implements org.apache.camel.Suspendable {
    StubbornConsumer(Endpoint endpoint, Processor processor) {
      super(endpoint, processor);
    }

    @Override
    protected void doSuspend() {
      throw new IllegalStateException("cannot be suspended");
    }
  }

  @Test
  void aShutdownDoesNotWaitForAReconcileStuckStartingARouteAndGivesBackOnlyTheRoutesThatWentQuiet() throws Exception {
    ListingEndpoint listing = new ListingEndpoint(camel);
    special.put("listing", context -> listing);
    special.put("stubborn", StubbornEndpoint::new);
    CountDownLatch entered = new CountDownLatch(1);
    CountDownLatch leave = new CountDownLatch(1);
    special.put("slow", context -> new RuntimeLoopTest.StuckEndpoint(context, entered, leave));
    List<Ingestion> running =
        List.of(polled("files"), polled("late"), polled("busy"), polled("listing"), polled("stubborn"), queue("queue"));
    platform.active = running;
    loop.tick();
    assertThat(listing.entered.await(10, TimeUnit.SECONDS)).isTrue();

    // The next round gets stuck starting slow, holding the reconciler's lock and Camel's; after it is
    // listed after slow and must never be started once the shutdown began.
    platform.active = Stream.concat(running.stream(), Stream.of(polled("slow"), polled("after"))).toList();
    Thread round =
        Thread.ofPlatform()
            .start(
                () -> {
                  try {
                    loop.tick();
                  } catch (InterruptedException e) {
                    Thread.currentThread().interrupt();
                  }
                });
    CountDownLatch busy = new CountDownLatch(1);
    hold.put("busy", busy);
    slow.put("late", Duration.ofSeconds(4));
    begun.put("busy", new CountDownLatch(1));
    begun.put("late", new CountDownLatch(1));
    Map<String, Boolean> suspendedAtGoodbye = new TreeMap<>();
    platform.onRelease =
        () -> {
          for (String id : List.of("files", "late", "busy", "listing", "queue")) {
            suspendedAtGoodbye.put(id, suspended(id));
          }
          suspendedAtGoodbye.put("round stuck", round.isAlive());
        };
    try {
      assertThat(entered.await(10, TimeUnit.SECONDS)).isTrue();
      Thread lateExchange = send("late", "L");
      Thread busyExchange = send("busy", "B");
      assertThat(begun.get("late").await(10, TimeUnit.SECONDS)).isTrue();
      assertThat(begun.get("busy").await(10, TimeUnit.SECONDS)).isTrue();

      long began = System.nanoTime();
      stopWithin(parts(), Duration.ofSeconds(15));
      Duration took = Duration.ofNanos(System.nanoTime() - began);

      // It waited for the lock, then went on without it, within the framework's bound on the step.
      assertThat(took).isGreaterThanOrEqualTo(RuntimeModule.SHUTDOWN_LOCK_WAIT).isLessThan(Duration.ofSeconds(10));
      // Exactly the routes whose intake was suspended with nothing in flight and no poll running:
      // late finished its exchange while the shutdown waited; busy did not, listing is still polling,
      // stubborn could not be suspended, queue is shared, slow was never started.
      assertThat(platform.releases).singleElement().satisfies(r -> {
        assertThat(r.goodbye()).isTrue();
        assertThat(r.ingestionIds()).containsExactlyInAnyOrder("files", "late");
        // What was left of the bound after the routes' time, less the grace: 10 - 0.5 - 6 - 1 s.
        assertThat(r.timeout()).isLessThanOrEqualTo(Duration.ofMillis(2_600));
      });
      assertThat(log).containsExactly("functions", "watchdog", "loop", "goodbye [files, late]", "traces");
      assertThat(suspendedAtGoodbye)
          .containsExactlyInAnyOrderEntriesOf(
              Map.of("files", true, "late", true, "busy", true, "listing", true, "queue", true, "round stuck", true));
      assertThat(taken).containsExactly("L");
      lateExchange.join();
      assertThat(busyExchange.isAlive()).isTrue();
    } finally {
      busy.countDown();
      listing.leave.countDown();
      leave.countDown();
      round.join();
    }
    // The stuck start finished; nothing after it was started.
    assertThat(camel.getRoute("slow")).isNotNull();
    assertThat(camel.getRoute("after")).isNull();
  }

  // A2: routes stopped at the same time.

  @Test
  void severalRoutesWithAnExchangeInFlightStopTogetherNotOneAfterAnother() throws Exception {
    List<Ingestion> five = Stream.of("a", "b", "c", "d", "e").map(ShutdownTest::polled).toList();
    platform.active = five;
    loop.tick();
    for (Ingestion ingestion : five) {
      slow.put(ingestion.id(), Duration.ofSeconds(3));
      begun.put(ingestion.id(), new CountDownLatch(1));
      feed(ingestion.id());
    }
    for (Ingestion ingestion : five) {
      assertThat(begun.get(ingestion.id()).await(10, TimeUnit.SECONDS)).isTrue();
    }

    long began = System.nanoTime();
    Reconciler.Stopped stopped = reconciler.stop(i -> true, RuntimeModule.STOP_WITHIN);
    Duration took = Duration.ofNanos(System.nanoTime() - began);

    // One after another, each would wait for the exchange its source took while the others stopped: 5 x 3 s.
    assertThat(took).isLessThan(Duration.ofSeconds(6));
    assertThat(stopped.clean()).extracting(Ingestion::id).containsExactlyInAnyOrder("a", "b", "c", "d", "e");
    assertThat(camel.getRoutes()).isEmpty();
  }

  @Test
  void aRebalanceStopsSeveralBusyRoutesTogether() throws Exception {
    List<Ingestion> five = Stream.of("a", "b", "c", "d", "e").map(ShutdownTest::polled).toList();
    platform.active = five;
    loop.tick();
    for (Ingestion ingestion : five) {
      slow.put(ingestion.id(), Duration.ofSeconds(3));
      begun.put(ingestion.id(), new CountDownLatch(1));
      feed(ingestion.id());
    }
    for (Ingestion ingestion : five) {
      assertThat(begun.get(ingestion.id()).await(10, TimeUnit.SECONDS)).isTrue();
    }

    long began = System.nanoTime();
    // Moved to other runtimes: no longer listed.
    List<Ingestion> stopped = reconciler.reconcile(List.of());
    Duration took = Duration.ofNanos(System.nanoTime() - began);

    assertThat(took).isLessThan(Duration.ofSeconds(6));
    assertThat(stopped).extracting(Ingestion::id).containsExactlyInAnyOrder("a", "b", "c", "d", "e");
    assertThat(camel.getRoutes()).isEmpty();
  }

  @Test
  void aFileBatchCutOffByAStopLeavesTheFilesItDidNotReachWhereTheyAre(@TempDir Path dir) throws Exception {
    for (String name : List.of("a.csv", "b.csv", "c.csv", "d.csv", "e.csv")) {
      Files.writeString(dir.resolve(name), name);
    }
    special.put(
        "files",
        context ->
            context.getEndpoint(
                "file:" + dir + "?initialDelay=0&delay=100&sortBy=file:name&move=${exchangeProperty." + IngestionRoute.FOLDER + "}/${file:name}"));
    slow.put("files", Duration.ofSeconds(1));
    begun.put("files", new CountDownLatch(1));
    platform.active = List.of(polled("files"));
    loop.tick();
    assertThat(begun.get("files").await(10, TimeUnit.SECONDS)).isTrue();

    long began = System.nanoTime();
    Reconciler.Stopped stopped = reconciler.stop(i -> true, RuntimeModule.STOP_WITHIN);
    Duration took = Duration.ofNanos(System.nanoTime() - began);

    // The exchange in flight finished and its file moved; the batch went no further, and what it did
    // not reach is where it was, for the next owner.
    assertThat(took).isLessThan(Duration.ofMillis(2_500));
    assertThat(stopped.clean()).extracting(Ingestion::id).containsExactly("files");
    assertThat(taken).containsExactly("a.csv");
    assertThat(dir.resolve(".done/a.csv")).exists();
    try (Stream<Path> left = Files.list(dir)) {
      assertThat(left.map(p -> p.getFileName().toString())).containsExactlyInAnyOrder(".done", "b.csv", "c.csv", "d.csv", "e.csv");
    }
  }
}
