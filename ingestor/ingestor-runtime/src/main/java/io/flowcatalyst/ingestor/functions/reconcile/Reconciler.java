package io.flowcatalyst.ingestor.functions.reconcile;

import io.flowcatalyst.function.FunctionAddress;
import io.flowcatalyst.ingestor.extension.Backoff;
import io.flowcatalyst.ingestor.extension.FailureLog;
import io.flowcatalyst.ingestor.functions.context.ContextFactory;
import io.flowcatalyst.ingestor.functions.context.ContextLoadException;
import io.flowcatalyst.ingestor.functions.context.HostFunctionContext;
import io.flowcatalyst.ingestor.functions.context.SettingsFingerprint;
import io.flowcatalyst.ingestor.functions.control.Control;
import io.flowcatalyst.ingestor.functions.control.ControlPlane;
import io.flowcatalyst.ingestor.functions.control.ControlPlaneException;
import io.flowcatalyst.ingestor.functions.control.Describe;
import io.flowcatalyst.ingestor.functions.http.EndpointRouter;
import io.flowcatalyst.ingestor.functions.http.PinnedVersions;
import io.flowcatalyst.ingestor.functions.load.FunctionRegistry;
import io.flowcatalyst.ingestor.functions.load.JvmFunctionLoader;
import io.flowcatalyst.ingestor.functions.load.Loaded;
import io.flowcatalyst.ingestor.functions.load.LoadedFunction;
import io.flowcatalyst.ingestor.functions.load.LoadOutcome;
import io.flowcatalyst.ingestor.functions.load.MetaspaceGuard;
import io.flowcatalyst.ingestor.functions.load.Reason;
import io.flowcatalyst.ingestor.functions.load.Refused;
import java.lang.management.ManagementFactory;
import java.nio.file.Path;
import java.time.Clock;
import java.time.Duration;
import java.time.Instant;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.Semaphore;
import java.util.concurrent.ThreadLocalRandom;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.locks.ReentrantLock;
import org.jspecify.annotations.Nullable;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/// Desired state → fetch → load → heartbeat, one cycle at a time. [ReconcileLoop] calls [#reconcileOnce]; this
/// class owns no thread of its own, which is what makes it directly testable.
///
/// **What it holds.** The platform's desired document for the pool, filtered to `jvm` versions: functions with
/// versions, each version with a role (`live`, `candidate`, `alias:x`). Every `jvm` version is loaded once to
/// prove it loads (the platform makes a version READY only on a `LOADED` report): the live version goes to
/// the [FunctionRegistry] (a *warm* function is exempt from eviction, a lazy one is closed after an idle hour
/// and loaded again by its next call), the others to [PinnedVersions] (a version evicted from there stays
/// proven and loads again when a call names it). **New before old:** a new live version is loaded and
/// initialised before it replaces the serving one, which is then closed once its in-flight calls drain.
///
/// **What it reports.** Only a permanent failure is `FAILED` (the platform latches the first `FAILED` report
/// for a version): a bad jar (native library, bundled API, entrypoint missing or not a function, `init`
/// throwing), a bad describe, an artifact the control plane refuses (404, a wrong digest). A condition that
/// can pass is `RETRYING` and tried again with backoff: the control plane or the artifact store being down,
/// the metaspace headroom. After the retry budget (5 minutes) a transient failure is reported `FAILED` and is
/// still retried. See [#heartbeat].
///
/// **Concurrency.** The invoke path ([#resolve]) reads only volatile and concurrent state and never takes a
/// lock when the version is loaded. A load on demand (a lazy function evicted when idle, a pinned version) is
/// guarded by a lock per address or per version.
public final class Reconciler implements AutoCloseable {

    private static final Logger LOG = LoggerFactory.getLogger(Reconciler.class);

    /// A lazy function idle longer than this is closed, but keeps its route (it loads again on its next call).
    static final Duration IDLE_UNLOAD = Duration.ofHours(1);

    /// The shortest wait [#nextWake] returns.
    private static final Duration MIN_WAKE = Duration.ofSeconds(1);

    /// Artifacts are fetched at most this many at a time.
    private static final int MAX_CONCURRENT_PREPARES = 4;

    /// Timing of the retries of a transient load failure (the Go runner's `LoadRetry` and `retryBase`).
    public record Timing(Duration initial, Duration max, Duration budget, Duration retryBase) {
        public static final Timing DEFAULTS =
                new Timing(Duration.ofSeconds(1), Duration.ofSeconds(30), Duration.ofMinutes(5), Duration.ofSeconds(5));
    }

    /// The phase of a version, as reported.
    public enum Phase { PREPARING, LOADED, RETRYING, FAILED }

    /// What a version declared and how it fares. Mutable fields are written by the reconciling thread (and
    /// by a load on demand under the address lock) and read by anyone.
    public static final class VersionRecord {
        final String functionId;
        final String address;
        final int number;
        final String digest;
        final @Nullable Describe describe;
        final @Nullable EndpointRouter router;
        volatile List<String> roles;
        volatile Phase phase = Phase.PREPARING;
        volatile String reason = "";
        volatile @Nullable Path artifact;
        // retry bookkeeping
        volatile Instant nextAttempt = Instant.MIN;
        volatile Instant failedAt = Instant.MIN;
        volatile boolean permanent;
        int attempts;
        @Nullable Instant firstTransient;

        VersionRecord(String functionId, String address, Control.VersionDoc doc, @Nullable Describe describe, @Nullable EndpointRouter router) {
            this.functionId = functionId;
            this.address = address;
            this.number = doc.number();
            this.digest = doc.digest();
            this.describe = describe;
            this.router = router;
            this.roles = doc.roles();
        }

        public int number() {
            return number;
        }

        public Phase phase() {
            return phase;
        }

        public String reason() {
            return reason;
        }
    }

    /// One function of the current document.
    record FunctionView(
            Control.FunctionDoc doc,
            @Nullable FunctionAddress api,
            Map<Integer, VersionRecord> versions,
            int live,
            Map<String, Integer> aliases,
            String fingerprint) {
    }

    /// The current document, immutable once built.
    record Snapshot(long revision, Map<String, FunctionView> functions) {
        static final Snapshot EMPTY = new Snapshot(0, Map.of());
    }

    /// What the host remembers with a loaded version (its [LoadedFunction#attachment]).
    public record Served(int number, Describe describe, EndpointRouter router, String fingerprint) {
    }

    /// How a call's target resolved.
    public sealed interface Resolution {
        /// No function has this address.
        record Unknown() implements Resolution {
        }

        /// The function has no such alias.
        record AliasNotFound() implements Resolution {
        }

        /// The function has no such version here.
        record VersionNotFound() implements Resolution {
        }

        /// Not servable now: still loading (503, retry soon) or failed / unloaded (503, retry later).
        record Unavailable(String code, String message, int retryAfterSeconds) implements Resolution {
        }

        /// A loaded version, RETAINED: the caller must pass it to an invocation (which releases it) or call
        /// `function.release()`.
        record Ready(Control.FunctionDoc doc, Served served, LoadedFunction function) implements Resolution {
        }
    }

    private final String pool;
    private final String runnerId;
    private final String runnerVersion;
    private final ControlPlane controlPlane;
    private final ArtifactCache artifacts;
    private final JvmFunctionLoader loader;
    private final FunctionRegistry registry;
    private final PinnedVersions pinned = new PinnedVersions();
    private final ContextFactory contextFactory;
    private final MetaspaceGuard metaspaceGuard;
    private final Clock clock;
    private final Timing timing;
    private final Instant startedAt;

    /// `address → lock` so two calls (or a call and a reconcile) load one address once.
    private final Map<String, ReentrantLock> loadLocks = new ConcurrentHashMap<>();

    /// The records by `address@version`, kept across documents while a version's digest is unchanged.
    private final Map<String, VersionRecord> records = new ConcurrentHashMap<>();

    private final List<Runnable> changeListeners = new CopyOnWriteArrayList<>();
    private final List<Runnable> postReconcileListeners = new CopyOnWriteArrayList<>();

    private volatile Snapshot snapshot = Snapshot.EMPTY;
    private volatile @Nullable String etag;
    private volatile long appliedRevision;
    private volatile boolean reconcileAttempted;
    private volatile boolean everReconciled;
    private final AtomicBoolean draining = new AtomicBoolean();

    /// Failures that can repeat (docs/ingestor-failure-pacing.md R3): the control plane's desired and heartbeat
    /// calls (keys `control:desired`, `control:heartbeat`) and each version's artifact fetch.
    private final FailureLog failures;

    /// Heartbeats refused (401/403) in a row, and the time before which none is sent (R6, F10).
    private final AtomicInteger heartbeatRefusals = new AtomicInteger();
    private volatile Instant heartbeatHeldUntil = Instant.MIN;

    public Reconciler(
            String pool,
            String runnerId,
            String runnerVersion,
            ControlPlane controlPlane,
            ArtifactCache artifacts,
            JvmFunctionLoader loader,
            FunctionRegistry registry,
            ContextFactory contextFactory,
            MetaspaceGuard metaspaceGuard,
            Clock clock,
            Timing timing) {
        this(pool, runnerId, runnerVersion, controlPlane, artifacts, loader, registry, contextFactory, metaspaceGuard, clock, timing,
                new FailureLog((message, cause) -> {
                    if (cause == null) {
                        LOG.warn(message);
                    } else {
                        LOG.warn(message, cause);
                    }
                }, Duration.ofMinutes(1), clock));
    }

    /// @param failures where repeated failures are logged (tests give one with a clock they move)
    public Reconciler(
            String pool,
            String runnerId,
            String runnerVersion,
            ControlPlane controlPlane,
            ArtifactCache artifacts,
            JvmFunctionLoader loader,
            FunctionRegistry registry,
            ContextFactory contextFactory,
            MetaspaceGuard metaspaceGuard,
            Clock clock,
            Timing timing,
            FailureLog failures) {
        this.failures = Objects.requireNonNull(failures, "failures");
        this.pool = Objects.requireNonNull(pool, "pool");
        this.runnerId = Objects.requireNonNull(runnerId, "runnerId");
        this.runnerVersion = Objects.requireNonNull(runnerVersion, "runnerVersion");
        this.controlPlane = Objects.requireNonNull(controlPlane, "controlPlane");
        this.artifacts = Objects.requireNonNull(artifacts, "artifacts");
        this.loader = Objects.requireNonNull(loader, "loader");
        this.registry = Objects.requireNonNull(registry, "registry");
        this.contextFactory = Objects.requireNonNull(contextFactory, "contextFactory");
        this.metaspaceGuard = Objects.requireNonNull(metaspaceGuard, "metaspaceGuard");
        this.clock = Objects.requireNonNull(clock, "clock");
        this.timing = Objects.requireNonNull(timing, "timing");
        this.startedAt = clock.instant();
    }

    /// Runs `listener` whenever something a heartbeat reports changed (a version loaded, failed, retrying).
    public void addChangeListener(Runnable listener) {
        changeListeners.add(Objects.requireNonNull(listener, "listener"));
    }

    /// Runs `listener` at the end of every [#reconcileOnce].
    public void addPostReconcileListener(Runnable listener) {
        postReconcileListeners.add(Objects.requireNonNull(listener, "listener"));
    }

    public FunctionRegistry registry() {
        return registry;
    }

    public PinnedVersions pinned() {
        return pinned;
    }

    /// The addresses of the current document's functions.
    public java.util.Set<String> addresses() {
        return snapshot.functions().keySet();
    }

    /// Whether a desired document has been applied (`/readyz`'s question).
    public boolean ready() {
        return everReconciled && !draining.get();
    }

    /// Begins shutdown: nothing new is resolved.
    public void drain() {
        draining.set(true);
    }

    // ── the cycle ──────────────────────────────────────────────────────────────────────────

    /// [#reconcileOnce(Instant, Duration)] without waiting for a change.
    public Outcome reconcileOnce(Instant now) {
        return reconcileOnce(now, Duration.ZERO);
    }

    /// How a cycle's fetch went.
    ///
    /// `REFUSED` is an outage of one kind: the platform answered 401/403 after the token was refreshed (the
    /// service account is not a function runner). The loop waits much longer for it than for a plain outage.
    public enum Outcome { CHANGED, NOT_MODIFIED, OUTAGE, REFUSED }

    /// One full cycle: fetch the document (the platform holds the request up to `wait` for a change), then
    /// fetch, load and unload, then heartbeat — in that order, and no earlier step is skipped because a later
    /// one fails.
    ///
    /// A control-plane outage unloads nothing and loads nothing: what is loaded keeps serving. `NotModified`
    /// still runs the load step against the same document, because retries and idle eviction are driven by
    /// the clock, not by the document.
    public Outcome reconcileOnce(Instant now, Duration wait) {
        Objects.requireNonNull(now, "now");
        metaspaceGuard.beginCycle();
        boolean outage = false;
        Outcome outcome = Outcome.NOT_MODIFIED;
        try {
            switch (controlPlane.desired(pool, etag, wait)) {
                case ControlPlane.Fetched.NotModified ignored -> everReconciled = true;
                case ControlPlane.Fetched.Changed(String newEtag, Control.Desired doc) -> {
                    etag = newEtag;
                    apply(doc, now);
                    everReconciled = true;
                    outcome = Outcome.CHANGED;
                }
            }
            failures.recovered("control:desired", "the platform answers desired-state requests again");
        } catch (ControlPlaneException e) {
            outage = true;
            if (isRefusal(e)) {
                failures.failed("control:desired", refusedText(e), null);
                outcome = Outcome.REFUSED;
            } else {
                failures.failed("control:desired", "desired-state fetch failed; keeping what is already loaded: " + e.getMessage(), null);
                outcome = Outcome.OUTAGE;
            }
        }
        reconcileAttempted = true;

        if (!outage) {
            Snapshot current = snapshot;
            prepare(current, now);
            load(current, now);
            unload(current, now);
        }
        sendHeartbeat();
        for (Runnable listener : postReconcileListeners) {
            listener.run();
        }
        return outcome;
    }

    /// How long the loop may wait for the platform to answer before something here is due again (a retry),
    /// at most `max`, at least 1 s (a wait of 0 would make the platform answer at once, and the loop would
    /// long-poll in a tight loop).
    public Duration nextWake(Instant now, Duration max) {
        Instant earliest = null;
        for (VersionRecord r : records.values()) {
            Phase p = r.phase;
            if ((p == Phase.RETRYING || (p == Phase.FAILED && !r.permanent)) && (earliest == null || r.nextAttempt.isBefore(earliest))) {
                earliest = r.nextAttempt;
            }
        }
        if (earliest == null) {
            return max;
        }
        Duration until = Duration.between(now, earliest);
        if (until.compareTo(MIN_WAKE) < 0) {
            until = MIN_WAKE;
        }
        return until.compareTo(max) < 0 ? until : max;
    }

    // ── step 1: apply the document ─────────────────────────────────────────────────────────────

    private void apply(Control.Desired doc, Instant now) {
        Map<String, FunctionView> functions = new LinkedHashMap<>();
        Map<String, VersionRecord> seen = new LinkedHashMap<>();
        for (Control.FunctionDoc fn : doc.functions()) {
            Map<Integer, VersionRecord> versions = new LinkedHashMap<>();
            int live = 0;
            Map<String, Integer> aliases = new LinkedHashMap<>();
            for (Control.VersionDoc v : fn.versions()) {
                if (!Control.RUNTIME_JVM.equals(v.runtime())) {
                    // Not ours: the platform filters by the `runtimes` we asked for, so this is a platform that
                    // ignored the filter. Hold nothing and report nothing: a FAILED report would be latched on a
                    // version another runner can run.
                    LOG.warn("skipping a version of a runtime this host does not have: {} v{} runtime={}", fn.address(), v.number(), v.runtime());
                    continue;
                }
                String key = fn.address() + "@" + v.number();
                VersionRecord rec = records.get(key);
                boolean sameFunction = rec != null && rec.functionId.equals(fn.id());
                if (rec == null || !rec.digest.equals(v.digest()) || !sameFunction) {
                    rec = newRecord(fn, v, now);
                    records.put(key, rec);
                } else {
                    rec.roles = v.roles();
                    // A failed version is never pinned: a re-rendered desired state retries it, at most once
                    // per retryBase.
                    if (rec.phase == Phase.FAILED && now.isAfter(rec.failedAt.plus(timing.retryBase()))
                            && rec.describe != null && rec.router != null) {
                        rec.phase = Phase.PREPARING;
                        rec.reason = "";
                        rec.permanent = false;
                        rec.attempts = 0;
                        rec.firstTransient = null;
                        rec.nextAttempt = Instant.MIN;
                    }
                }
                versions.put(v.number(), rec);
                seen.put(key, rec);
                for (String role : v.roles()) {
                    if (role.equals(Control.ROLE_LIVE)) {
                        live = v.number();
                    } else if (role.startsWith(Control.ROLE_ALIAS_PREFIX) && role.length() > Control.ROLE_ALIAS_PREFIX.length()) {
                        aliases.put(role.substring(Control.ROLE_ALIAS_PREFIX.length()), v.number());
                    }
                }
            }
            String fingerprint = SettingsFingerprint.of(fn.config(), fn.secrets(), fn.db());
            @Nullable FunctionAddress api = null;
            try {
                api = FunctionAddress.parse(fn.address());
            } catch (IllegalArgumentException e) {
                // reported per version by loadNew
            }
            functions.put(fn.address(), new FunctionView(fn, api, Map.copyOf(versions), live, Map.copyOf(aliases), fingerprint));
        }
        records.keySet().removeIf(k -> !seen.containsKey(k));
        appliedRevision = doc.revision();
        snapshot = new Snapshot(doc.revision(), Map.copyOf(functions));
        notifyChange();
    }

    private VersionRecord newRecord(Control.FunctionDoc fn, Control.VersionDoc v, Instant now) {
        try {
            Describe d = Describe.parse(v.describe());
            EndpointRouter router = new EndpointRouter(d.endpoints());
            return new VersionRecord(fn.id(), fn.address(), v, d, router);
        } catch (Describe.DescribeException | IllegalArgumentException e) {
            // The document itself is bad: retrying cannot help.
            VersionRecord rec = new VersionRecord(fn.id(), fn.address(), v, null, null);
            rec.phase = Phase.FAILED;
            rec.reason = "DESCRIBE: " + e.getMessage();
            rec.permanent = true;
            rec.failedAt = now;
            LOG.warn("a function version has a describe document this host cannot serve: {} v{}: {}", fn.address(), v.number(), e.getMessage());
            return rec;
        }
    }

    // ── step 2: fetch artifacts ────────────────────────────────────────────────────────────────

    private void prepare(Snapshot current, Instant now) {
        List<VersionRecord> due = new ArrayList<>();
        for (FunctionView view : current.functions().values()) {
            for (VersionRecord rec : view.versions().values()) {
                if (rec.describe != null && rec.artifact == null && isDue(rec, now)) {
                    due.add(rec);
                }
            }
        }
        if (due.isEmpty()) {
            return;
        }
        Semaphore gate = new Semaphore(MAX_CONCURRENT_PREPARES);
        try (ExecutorService executor = Executors.newVirtualThreadPerTaskExecutor()) {
            List<Future<?>> futures = new ArrayList<>();
            for (VersionRecord rec : due) {
                futures.add(executor.submit(() -> {
                    gate.acquireUninterruptibly();
                    try {
                        fetchArtifact(rec, now);
                    } finally {
                        gate.release();
                    }
                }));
            }
            for (Future<?> f : futures) {
                try {
                    f.get();
                } catch (InterruptedException e) {
                    Thread.currentThread().interrupt();
                    return;
                } catch (java.util.concurrent.ExecutionException e) {
                    Throwable cause = e.getCause() == null ? e : e.getCause();
                    failures.failed("reconcile:prepare:" + cause.getClass().getName(), "fetching an artifact failed unexpectedly", cause);
                }
            }
        }
    }

    /// Whether a version should be attempted now: not yet tried, or retrying and its delay has passed.
    private static boolean isDue(VersionRecord rec, Instant now) {
        return switch (rec.phase) {
            case PREPARING -> !now.isBefore(rec.nextAttempt);
            case RETRYING -> !now.isBefore(rec.nextAttempt);
            case FAILED -> !rec.permanent && !now.isBefore(rec.nextAttempt);
            case LOADED -> false;
        };
    }

    private void fetchArtifact(VersionRecord rec, Instant now) {
        try {
            rec.artifact = artifacts.get(rec.digest);
            failures.recovered(rec.address + "@" + rec.number + ":artifact");
        } catch (ControlPlaneException e) {
            fail(rec, "ARTIFACT: " + e.getMessage(), !e.isTransient(), now);
            failures.failed(rec.address + "@" + rec.number + ":artifact",
                    "an artifact could not be fetched: " + rec.address + " v" + rec.number + ": " + e.getMessage(), null);
        } catch (RuntimeException e) {
            // Anything else from the fetch (a malformed digest, a bug) is no reason to leave the version due and be
            // asked again on every cycle: it is a transient failure, retried by the same backoff as a fetch error.
            fail(rec, "ARTIFACT: " + e.getClass().getSimpleName() + ": " + e.getMessage(), false, now);
            failures.failed(rec.address + "@" + rec.number + ":artifact",
                    "an artifact could not be fetched: " + rec.address + " v" + rec.number + ": " + e.getClass().getName(), e);
        }
    }

    // ── step 3: load ──────────────────────────────────────────────────────────────────────────

    private void load(Snapshot current, Instant now) {
        for (FunctionView view : current.functions().values()) {
            Control.FunctionDoc doc = view.doc();
            boolean warm = doc.warm();
            // The live version first: callers are waiting on it.
            if (view.live() != 0) {
                VersionRecord rec = view.versions().get(view.live());
                if (rec != null) {
                    ensureLive(view, rec, warm, now, false);
                }
            }
            for (VersionRecord rec : view.versions().values()) {
                if (rec.number == view.live()) {
                    continue;
                }
                ensureProven(view, rec, now);
            }
        }
    }

    /// Makes `rec` the serving version of its address, new before old. `onDemand` is a call finding the
    /// registry empty (a lazy function evicted when idle).
    private @Nullable LoadedFunction ensureLive(FunctionView view, VersionRecord rec, boolean warm, Instant now, boolean onDemand) {
        String address = view.doc().address();
        @Nullable FunctionAddress api = view.api();
        if (api == null) {
            fail(rec, "ADDRESS_UNSUPPORTED", true, now);
            return null;
        }
        ReentrantLock lock = loadLocks.computeIfAbsent(address, a -> new ReentrantLock());
        lock.lock();
        try {
            LoadedFunction current = registry.peek(api);
            Served served = current == null ? null : servedOf(current);
            boolean sameVersion = served != null && served.number() == rec.number;
            boolean sameSettings = served != null && served.fingerprint().equals(view.fingerprint());
            if (current != null && sameVersion && sameSettings) {
                if (rec.phase != Phase.LOADED) {
                    markLoaded(rec);
                }
                if (registry.isWarm(api) != warm) {
                    registry.put(current, warm); // the warm flag changed; same instance, nothing displaced
                }
                return current;
            }
            // A lazy version that has been proven needs no instance until a call comes for it.
            if (!onDemand && !warm && current == null && rec.phase == Phase.LOADED && sameSettingsAsProven(rec, view)) {
                return null;
            }
            if (rec.describe == null || rec.router == null) {
                return current; // FAILED already: whatever serves keeps serving
            }
            // A candidate being promoted: the very instance that was proven.
            LoadedFunction promoted = pinned.take(new PinnedVersions.Key(address, rec.number), view.fingerprint());
            LoadedFunction fresh = promoted;
            if (fresh == null) {
                if (rec.artifact == null || !isDueForLoad(rec, now, onDemand)) {
                    return current;
                }
                fresh = loadNew(view, rec, now);
                if (fresh == null) {
                    return current; // the failure is recorded; the old version (if any) keeps serving
                }
            }
            LoadedFunction displaced;
            try {
                displaced = registry.put(fresh, warm);
            } catch (IllegalStateException e) {
                fail(rec, "LOAD:REGISTRY_FULL", false, now);
                LOG.warn("registry refused a live function: {}", e.getMessage());
                closeAsync(fresh);
                return current;
            }
            markLoaded(rec);
            provenFingerprints.put(proofKey(rec), view.fingerprint());
            if (displaced != null && displaced != fresh) {
                closeAsync(displaced);
            }
            return fresh;
        } finally {
            lock.unlock();
        }
    }

    private boolean isDueForLoad(VersionRecord rec, Instant now, boolean onDemand) {
        if (onDemand) {
            return rec.phase == Phase.LOADED;
        }
        return rec.phase == Phase.LOADED || isDue(rec, now);
    }

    /// The settings a proven version was proven with (so a settings change reloads even a lazy one).
    private final Map<String, String> provenFingerprints = new ConcurrentHashMap<>();

    private static String proofKey(VersionRecord rec) {
        return rec.address + "@" + rec.number;
    }

    private boolean sameSettingsAsProven(VersionRecord rec, FunctionView view) {
        return view.fingerprint().equals(provenFingerprints.get(proofKey(rec)));
    }

    /// Loads a non-live version once so that it is proven (the platform promotes a candidate only after a
    /// `LOADED` report), and keeps it in the pinned cache.
    private void ensureProven(FunctionView view, VersionRecord rec, Instant now) {
        if (rec.describe == null || rec.router == null || rec.artifact == null) {
            return;
        }
        if (rec.phase == Phase.LOADED && sameSettingsAsProven(rec, view)) {
            return;
        }
        if (rec.phase != Phase.LOADED && !isDue(rec, now)) {
            return;
        }
        ReentrantLock lock = loadLocks.computeIfAbsent(view.doc().address() + "@" + rec.number, a -> new ReentrantLock());
        lock.lock();
        try {
            LoadedFunction fresh = loadNew(view, rec, now);
            if (fresh == null) {
                return;
            }
            pinned.put(new PinnedVersions.Key(view.doc().address(), rec.number), view.fingerprint(), fresh);
            markLoaded(rec);
            provenFingerprints.put(proofKey(rec), view.fingerprint());
        } finally {
            lock.unlock();
        }
    }

    /// The loader, the context and `init`, with the metaspace guard in front. On failure the attempt is
    /// recorded against `rec` and `null` is returned; on success the returned function is initialised.
    private @Nullable LoadedFunction loadNew(FunctionView view, VersionRecord rec, Instant now) {
        Describe describe = Objects.requireNonNull(rec.describe);
        EndpointRouter router = Objects.requireNonNull(rec.router);
        Path artifact = Objects.requireNonNull(rec.artifact);
        @Nullable FunctionAddress address = view.api();
        if (address == null) {
            fail(rec, "ADDRESS_UNSUPPORTED", true, now);
            return null;
        }
        MetaspaceGuard.Result headroom = metaspaceGuard.check();
        if (!headroom.hasHeadroom()) {
            fail(rec, "LOAD:" + Reason.METASPACE_HEADROOM.name(), false, now);
            LOG.warn("a function version was not loaded: no metaspace headroom: {}", headroom.detail());
            return null;
        }
        LoadOutcome outcome = loader.load(artifact, describe.entrypoint(), address, rec.number);
        switch (outcome) {
            case Refused(Reason reason, String detail) -> {
                fail(rec, "LOAD:" + reason.name(), !reason.transientReason(), now);
                LOG.warn("a function version was refused at load: {} v{}: {}: {}", rec.address, rec.number, reason, detail);
                return null;
            }
            case Loaded(LoadedFunction fn) -> {
                return initialise(view, rec, fn, describe, router, now);
            }
        }
    }

    private @Nullable LoadedFunction initialise(
            FunctionView view, VersionRecord rec, LoadedFunction fn, Describe describe, EndpointRouter router, Instant now) {
        HostFunctionContext ctx;
        try {
            ctx = contextFactory.build(view.doc(), rec.number, describe);
        } catch (ContextLoadException e) {
            fail(rec, e.code(), !e.isTransient(), now);
            LOG.warn("a function version's context cannot be built: {} v{}: {}", rec.address, rec.number, e.getMessage());
            fn.close();
            return null;
        }
        fn.attachContext(ctx);
        fn.attach(new Served(rec.number, describe, router, view.fingerprint()));
        try {
            fn.init(ctx);
        } catch (Error e) {
            // A catchable OutOfMemoryError: Metaspace (or Compressed class space) fails ONLY this one load — the
            // class loader is closed so the space can be reclaimed, and whatever served keeps serving. A
            // Java-heap OOM is not this call's to swallow.
            OutOfMemoryError oom = JvmFunctionLoader.findMetaspaceOom(e);
            if (oom == null) {
                if (e instanceof LinkageError) {
                    // NoClassDefFoundError, ExceptionInInitializerError, UnsatisfiedLinkError...: the function's own
                    // bytes fail to link, the same way every time: like an exception, permanently INIT_FAILED. It
                    // must not leave the reconcile cycle.
                    fail(rec, "LOAD:INIT_FAILED", true, now);
                    LOG.warn("a function version's init() failed: {} v{}", rec.address, rec.number, e);
                    fn.close();
                    return null;
                }
                throw e;
            }
            fail(rec, "LOAD:" + Reason.OUT_OF_METASPACE.name(), false, now);
            fn.close(); // BEFORE logging: at a real fence the log call itself can throw
            LOG.error("function version failed to init: out of metaspace: {} v{}", rec.address, rec.number);
            return null;
        } catch (Exception e) {
            fail(rec, "LOAD:INIT_FAILED", true, now);
            LOG.warn("a function version's init() failed: {} v{}", rec.address, rec.number, e);
            fn.close();
            return null;
        }
        return fn;
    }

    // ── bookkeeping of one version's attempts ─────────────────────────────────────────────────────

    private void markLoaded(VersionRecord rec) {
        if (rec.phase != Phase.LOADED) {
            rec.phase = Phase.LOADED;
            rec.reason = "";
            rec.permanent = false;
            rec.attempts = 0;
            rec.firstTransient = null;
            notifyChange();
        }
    }

    /// Records a failed attempt. `permanent`: the same bytes fail the same way, so `FAILED` at once (the platform
    /// latches it). Otherwise the version stays `RETRYING` with growing delays until the retry budget is
    /// spent, then `FAILED` — and is still retried.
    private void fail(VersionRecord rec, String reason, boolean permanent, Instant now) {
        rec.attempts++;
        rec.failedAt = now;
        if (permanent) {
            rec.phase = Phase.FAILED;
            rec.reason = reason;
            rec.permanent = true;
        } else {
            Instant first = rec.firstTransient;
            if (first == null) {
                first = now;
                rec.firstTransient = first;
            }
            Duration delay = retryDelay(rec.attempts);
            rec.nextAttempt = now.plus(delay);
            rec.permanent = false;
            if (Duration.between(first, now).plus(delay).compareTo(timing.budget()) <= 0) {
                rec.phase = Phase.RETRYING;
                rec.reason = "attempt " + rec.attempts + " failed, retrying in " + delay.toMillis() + " ms: " + reason;
            } else {
                rec.phase = Phase.FAILED;
                rec.reason = reason + " (gave up after " + rec.attempts + " attempts over " + Duration.between(first, now).toSeconds() + " s)";
                // Keep trying, backing off between rounds.
                long shift = Math.min(rec.attempts - 1, 20);
                Duration longer = timing.retryBase().multipliedBy(1L << shift);
                Duration cap = Duration.ofMinutes(5);
                rec.nextAttempt = now.plus(longer.compareTo(cap) < 0 ? longer : cap);
            }
        }
        notifyChange();
    }

    /// `delay`: initial·2^(attempt-1) capped at max, with "equal jitter" (uniform in [d/2, d]) so hosts that
    /// failed together do not retry in lockstep.
    Duration retryDelay(int attempt) {
        long initial = timing.initial().toMillis();
        long max = timing.max().toMillis();
        long d = max;
        int shift = attempt - 1;
        if (shift < 40 && (initial << shift) > 0 && (initial << shift) < max) {
            d = initial << shift;
        }
        long half = d / 2;
        return Duration.ofMillis(half <= 0 ? d : half + ThreadLocalRandom.current().nextLong(half + 1));
    }

    private void notifyChange() {
        for (Runnable listener : changeListeners) {
            try {
                listener.run();
            } catch (RuntimeException e) {
                LOG.warn("a change listener failed", e);
            }
        }
    }

    // ── step 4: unload ─────────────────────────────────────────────────────────────────────────

    private void unload(Snapshot current, Instant now) {
        // Registry entries whose function left the document, or that no longer has a live version.
        for (FunctionRegistry.Snapshot s : registry.snapshot()) {
            String address = s.address().render();
            FunctionView view = current.functions().get(address);
            if (view == null || view.live() == 0) {
                LoadedFunction removed = registry.remove(s.address());
                if (removed != null) {
                    closeAsync(removed);
                }
                continue;
            }
            // A serving version that is not the live one is kept until its successor has loaded (ensureLive
            // swaps it); one whose live version FAILED stays too: a bad new version must not take the old one down.
            // Idle lazy eviction: closed, but the route stays; the next call loads it again.
            if (!s.warm() && s.lastAccessed().isBefore(now.minus(IDLE_UNLOAD))) {
                LoadedFunction removed = registry.remove(s.address());
                if (removed != null) {
                    closeAsync(removed);
                }
            }
        }
        pinned.sweep(key -> {
            FunctionView view = current.functions().get(key.address());
            return view != null && view.versions().containsKey(key.version());
        });
        loadLocks.keySet().removeIf(k -> {
            int at = k.indexOf('@');
            return !current.functions().containsKey(at < 0 ? k : k.substring(0, at));
        });
        provenFingerprints.keySet().removeIf(k -> {
            int at = k.indexOf('@');
            return !records.containsKey(k) && at >= 0;
        });
    }

    // ── the invoke path ─────────────────────────────────────────────────────────────────────────

    /// Finds the version a call names. `alias` and `version` select a non-live version (`version > 0` wins);
    /// both empty selects the live (serving) one. The result is RETRIEVED (see [Resolution.Ready]).
    ///
    /// Lock-free when the version is loaded: a snapshot read, a registry or pinned lookup and an atomic increment.
    public Resolution resolve(String address, @Nullable String alias, int version) {
        if (draining.get()) {
            return new Resolution.Unavailable("FUNCTION_UNAVAILABLE", "the host is shutting down", 5);
        }
        FunctionView view = snapshot.functions().get(address);
        if (view == null) {
            return new Resolution.Unknown();
        }
        VersionRecord rec;
        boolean live = false;
        if (version > 0) {
            rec = view.versions().get(version);
            if (rec == null) {
                return new Resolution.VersionNotFound();
            }
        } else if (alias != null && !alias.isEmpty()) {
            Integer n = view.aliases().get(alias);
            if (n == null) {
                return new Resolution.AliasNotFound();
            }
            rec = view.versions().get(n);
            if (rec == null) {
                return new Resolution.AliasNotFound();
            }
        } else {
            live = true;
            VersionRecord liveRec = view.live() == 0 ? null : view.versions().get(view.live());
            // The serving version: the registry's, which is the old one until the new one has loaded.
            @Nullable FunctionAddress api = view.api();
            LoadedFunction serving = api == null ? null : registry.get(api);
            if (serving != null && serving.tryRetain()) {
                Object a = serving.attachment();
                if (a instanceof Served served) {
                    return new Resolution.Ready(view.doc(), served, serving);
                }
                serving.release();
            }
            if (liveRec == null) {
                return new Resolution.Unavailable("FUNCTION_UNAVAILABLE", "the function has no loaded version", 5);
            }
            rec = liveRec;
        }
        switch (rec.phase) {
            case PREPARING, RETRYING -> {
                return new Resolution.Unavailable("FUNCTION_PREPARING", "the version is still being prepared", 2);
            }
            case FAILED -> {
                // the reason is in the heartbeat and the log, not the caller's reply
                return new Resolution.Unavailable("FUNCTION_UNAVAILABLE", "the version could not be loaded", 30);
            }
            case LOADED -> {
                // proven: load it now if no instance is held (a lazy function evicted when idle, a pinned version)
            }
        }
        LoadedFunction fn = live
                ? ensureLive(view, rec, view.doc().warm(), clock.instant(), true)
                : loadPinned(view, rec);
        if (fn == null || !fn.tryRetain()) {
            return new Resolution.Unavailable("FUNCTION_UNAVAILABLE", "the function is not available right now", 5);
        }
        Object a = fn.attachment();
        if (a instanceof Served served) {
            return new Resolution.Ready(view.doc(), served, fn);
        }
        fn.release();
        return new Resolution.Unavailable("FUNCTION_UNAVAILABLE", "the function is not available right now", 5);
    }

    private @Nullable LoadedFunction loadPinned(FunctionView view, VersionRecord rec) {
        if (rec.describe == null || rec.router == null || rec.artifact == null) {
            return null;
        }
        return pinned.getOrLoad(new PinnedVersions.Key(view.doc().address(), rec.number), view.fingerprint(), () -> {
            Instant now = clock.instant();
            return loadNew(view, rec, now);
        });
    }

    private static @Nullable Served servedOf(LoadedFunction fn) {
        return fn.attachment() instanceof Served s ? s : null;
    }

    // ── heartbeat ──────────────────────────────────────────────────────────────────────────────

    /// The report: every `jvm` version of the document with its state. `LOADED` once it has loaded (even if its
    /// instance was since evicted), `FAILED` for a permanent failure, `RETRYING` for a transient one; a version
    /// still preparing is not reported (still pending on the platform).
    public Control.Heartbeat heartbeat() {
        List<Control.VersionReport> versions = new ArrayList<>();
        for (FunctionView view : snapshot.functions().values()) {
            for (VersionRecord rec : view.versions().values()) {
                switch (rec.phase) {
                    case LOADED -> versions.add(new Control.VersionReport(rec.functionId, rec.number, Control.STATE_LOADED, null));
                    case FAILED -> versions.add(new Control.VersionReport(rec.functionId, rec.number, Control.STATE_FAILED, rec.reason));
                    case RETRYING -> versions.add(new Control.VersionReport(rec.functionId, rec.number, Control.STATE_RETRYING, rec.reason));
                    case PREPARING -> {
                        // not reported
                    }
                }
            }
        }
        Runtime rt = Runtime.getRuntime();
        long used = rt.totalMemory() - rt.freeMemory();
        int loaded = registry.snapshot().size() + pinned.size();
        Control.BudgetReport budget = new Control.BudgetReport(rt.maxMemory(), 0, rt.maxMemory(), used, 0, 0, used, loaded, 0);
        Map<String, Object> extra = new LinkedHashMap<>();
        extra.put("jvm", System.getProperty("java.version", ""));
        extra.put("pid", ManagementFactory.getRuntimeMXBean().getPid());
        Control.RunnerCapabilities capabilities =
                new Control.RunnerCapabilities(Control.PROTOCOL_V1, List.of(Control.RUNTIME_JVM), "ingestor-jvm", runnerVersion, extra);
        return new Control.Heartbeat(runnerId, pool, startedAt.toString(), appliedRevision, budget, versions, capabilities);
    }

    /// Sends one heartbeat; a failure is logged, never thrown.
    ///
    /// A refused heartbeat (401/403 after the token was refreshed) holds the next one back by
    /// [Backoff#REFUSED_LOGIN]: five minutes, doubling to an hour (R6, F10).
    public void sendHeartbeat() {
        Instant now = clock.instant();
        if (now.isBefore(heartbeatHeldUntil)) {
            return;
        }
        try {
            controlPlane.heartbeat(heartbeat());
            heartbeatHeldUntil = Instant.MIN;
            heartbeatRefusals.set(0);
            failures.recovered("control:heartbeat", "heartbeats are accepted again");
        } catch (ControlPlaneException e) {
            if (isRefusal(e)) {
                heartbeatHeldUntil = now.plus(Backoff.REFUSED_LOGIN.delay(heartbeatRefusals.incrementAndGet()));
                failures.failed("control:heartbeat", "heartbeat refused: " + refusedText(e), null);
            } else {
                failures.failed("control:heartbeat", "heartbeat failed: " + e.getMessage(), null);
            }
        }
    }

    private static boolean isRefusal(ControlPlaneException e) {
        return e.status() == 401 || e.status() == 403;
    }

    private static String refusedText(ControlPlaneException e) {
        return "the platform refused this runtime as a function runner (" + e.status()
                + "): give its service account platform:function-runner";
    }

    /// Whether a reconcile has resolved, for diagnostics.
    public boolean reconcileAttempted() {
        return reconcileAttempted;
    }

    /// The record of a version, for tests and diagnostics.
    public @Nullable VersionRecord record(String address, int number) {
        return records.get(address + "@" + number);
    }

    // ── shutdown ─────────────────────────────────────────────────────────────────────────────────

    private static void closeAsync(LoadedFunction fn) {
        // After its in-flight calls drain, on a thread of its own so that the reconcile does not wait for them.
        Thread.ofVirtual().name("fn-close-" + fn.address().render()).start(fn::close);
    }

    /// Unloads everything, waiting for in-flight calls to finish (each version up to its drain timeout).
    @Override
    public void close() {
        drain();
        List<LoadedFunction> all = new ArrayList<>();
        for (FunctionRegistry.Snapshot s : registry.snapshot()) {
            LoadedFunction removed = registry.remove(s.address());
            if (removed != null) {
                all.add(removed);
            }
        }
        try (ExecutorService executor = Executors.newVirtualThreadPerTaskExecutor()) {
            for (LoadedFunction fn : all) {
                executor.submit(fn::close);
            }
            executor.submit(pinned::close);
        }
        artifacts.close();
    }
}
