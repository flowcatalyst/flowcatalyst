package io.flowcatalyst.ingestor.functions;

import io.flowcatalyst.function.DispatchException;
import io.flowcatalyst.function.EmitResult;
import io.flowcatalyst.function.PlatformTokenException;
import io.flowcatalyst.ingestor.functions.control.Control;
import io.flowcatalyst.ingestor.functions.control.ControlPlane;
import io.flowcatalyst.ingestor.functions.control.ControlPlaneException;
import java.time.Duration;
import java.util.List;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.function.Function;
import org.jspecify.annotations.Nullable;

/// An in-memory platform for the reconciler's tests: the desired document (re-served with a new etag on every
/// [#push]), artifacts by digest, and what the host sent back (heartbeats, emits, dispatches).
public final class FakeControlPlane implements ControlPlane {

    private volatile Control.@Nullable Desired document;
    private final AtomicInteger etag = new AtomicInteger();
    private final Map<String, byte[]> artifacts = new ConcurrentHashMap<>();
    public final List<Control.Heartbeat> heartbeats = new CopyOnWriteArrayList<>();
    public final List<Control.EmitRequest> emits = new CopyOnWriteArrayList<>();
    public final List<Control.DispatchRequest> dispatches = new CopyOnWriteArrayList<>();
    public final AtomicInteger artifactFetches = new AtomicInteger();
    public final AtomicInteger desiredCalls = new AtomicInteger();
    public volatile @Nullable ControlPlaneException desiredFailure;
    /// Thrown out of `desired` as it is (an unchecked exception or an `Error`), to test a cycle that blows up.
    public volatile @Nullable Throwable desiredThrows;
    public volatile @Nullable ControlPlaneException heartbeatFailure;
    public final AtomicInteger heartbeatCalls = new AtomicInteger();
    public volatile @Nullable Function<String, ControlPlaneException> artifactFailure;
    public volatile Function<Control.EmitRequest, EmitResult> emitAnswer = r -> new EmitResult.Emitted("evt_1");
    public volatile Function<Control.DispatchRequest, String> dispatchAnswer = r -> "djb_1";
    public final List<Control.TokenRequest> tokenRequests = new CopyOnWriteArrayList<>();
    public volatile Function<Control.TokenRequest, Control.TokenResponse> tokenAnswer = r -> new Control.TokenResponse("fake-platform-token", "Bearer", 3600);
    public volatile @Nullable String lastRequestedPool;
    /// The trace batches the host shipped, in order (only those the fake took).
    public final List<Control.TraceBatch> traceBatches = new CopyOnWriteArrayList<>();
    public final AtomicInteger traceCalls = new AtomicInteger();
    /// Answers a trace batch: returns normally to take it, throws to refuse it.
    public volatile TraceAnswer traceAnswer = batch -> new Control.TraceAck(batch.records().size(), 0);

    /// What the fake does with a trace batch.
    @FunctionalInterface
    public interface TraceAnswer {
        Control.TraceAck answer(Control.TraceBatch batch) throws ControlPlaneException;
    }

    /// Every trace record shipped so far, in order.
    public List<Control.TraceRecord> traceRecords() {
        return traceBatches.stream().flatMap(b -> b.records().stream()).toList();
    }

    public void addArtifact(Fx.Jar jar) {
        artifacts.put(jar.digest(), jar.bytes());
    }

    /// Serves `doc` from now on, as a changed document.
    public void push(Control.Desired doc) {
        document = doc;
        etag.incrementAndGet();
    }

    @Override
    public Fetched desired(String pool, @Nullable String known, Duration wait) throws ControlPlaneException {
        desiredCalls.incrementAndGet();
        lastRequestedPool = pool;
        Throwable thrown = desiredThrows;
        if (thrown instanceof Error error) {
            throw error;
        }
        if (thrown instanceof RuntimeException runtime) {
            throw runtime;
        }
        ControlPlaneException failure = desiredFailure;
        if (failure != null) {
            throw failure;
        }
        Control.Desired doc = document;
        if (doc == null) {
            throw new ControlPlaneException(503, "", "no document", null);
        }
        String current = "\"" + etag.get() + "\"";
        if (current.equals(known)) {
            return new Fetched.NotModified();
        }
        return new Fetched.Changed(current, doc);
    }

    @Override
    public void heartbeat(Control.Heartbeat heartbeat) throws ControlPlaneException {
        heartbeatCalls.incrementAndGet();
        ControlPlaneException failure = heartbeatFailure;
        if (failure != null) {
            throw failure;
        }
        heartbeats.add(heartbeat);
    }

    @Override
    public Control.TraceAck traceLog(Control.TraceBatch batch) throws ControlPlaneException {
        traceCalls.incrementAndGet();
        Control.TraceAck ack = traceAnswer.answer(batch);
        traceBatches.add(batch);
        return ack;
    }

    @Override
    public byte[] artifact(String digest) throws ControlPlaneException {
        artifactFetches.incrementAndGet();
        Function<String, ControlPlaneException> failing = artifactFailure;
        if (failing != null) {
            ControlPlaneException e = failing.apply(digest);
            if (e != null) {
                throw e;
            }
        }
        byte[] bytes = artifacts.get(digest);
        if (bytes == null) {
            throw new ControlPlaneException(404, "NOT_FOUND", "no such artifact", null);
        }
        return bytes;
    }

    @Override
    public EmitResult emit(Control.EmitRequest request) {
        emits.add(request);
        return emitAnswer.apply(request);
    }

    @Override
    public String dispatch(Control.DispatchRequest request) throws DispatchException {
        dispatches.add(request);
        return dispatchAnswer.apply(request);
    }

    @Override
    public Control.TokenResponse platformToken(Control.TokenRequest request) throws PlatformTokenException {
        tokenRequests.add(request);
        return tokenAnswer.apply(request);
    }

    /// The last heartbeat's report of version `number` of function `functionId`, or `null` if it is not reported.
    public Control.@Nullable VersionReport report(String functionId, int number) {
        if (heartbeats.isEmpty()) {
            return null;
        }
        for (Control.VersionReport r : heartbeats.get(heartbeats.size() - 1).versions()) {
            if (r.functionId().equals(functionId) && r.number() == number) {
                return r;
            }
        }
        return null;
    }
}
