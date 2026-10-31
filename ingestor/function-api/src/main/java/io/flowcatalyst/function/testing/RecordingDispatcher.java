package io.flowcatalyst.function.testing;

import io.flowcatalyst.function.DispatchException;
import io.flowcatalyst.function.DispatchJob;
import io.flowcatalyst.function.Dispatcher;

import java.util.List;
import java.util.concurrent.CopyOnWriteArrayList;

/// A [Dispatcher] test double: records every job and answers `job_recorded_{n}`;
/// [#failNext] makes the next [#dispatch] throw (once).
public final class RecordingDispatcher implements Dispatcher {

    private final List<DispatchJob> dispatched = new CopyOnWriteArrayList<>();
    private volatile DispatchException nextFailure;

    /// Makes the next [#dispatch] (only) throw `failure`; the job is not recorded.
    public void failNext(DispatchException failure) {
        this.nextFailure = failure;
    }

    /// The jobs accepted so far, in order.
    public List<DispatchJob> dispatched() {
        return dispatched;
    }

    @Override
    public String dispatch(DispatchJob job) {
        DispatchException pending = nextFailure;
        if (pending != null) {
            nextFailure = null;
            throw pending;
        }
        dispatched.add(job);
        return "job_recorded_" + dispatched.size();
    }
}
