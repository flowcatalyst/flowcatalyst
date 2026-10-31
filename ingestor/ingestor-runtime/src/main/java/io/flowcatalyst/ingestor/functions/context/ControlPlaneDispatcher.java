package io.flowcatalyst.ingestor.functions.context;

import io.flowcatalyst.function.DispatchException;
import io.flowcatalyst.function.DispatchJob;
import io.flowcatalyst.function.Dispatcher;
import io.flowcatalyst.ingestor.functions.control.Control;
import io.flowcatalyst.ingestor.functions.control.ControlPlane;
import io.flowcatalyst.ingestor.functions.control.Describe;
import java.util.ArrayList;
import java.util.Base64;
import java.util.List;
import java.util.Map;
import java.util.Objects;

/// [Dispatcher] over [ControlPlane#dispatch]: one `POST /control/functions/dispatch-jobs` per call. As the Go
/// runner does (`capabilities.dispatch`): the `(target, path)` must be one of this version's declared
/// `dispatches` (else `NOT_DECLARED`, no call), the payload is at most 256 KiB (`TOO_LARGE`), and the
/// platform does everything else (ownership, target resolution, URL, signing identity, the job rules): nothing
/// the function sends decides where a job goes. The calling invocation's id and the inbound request's
/// `X-Correlation-Id` travel with it ([InvocationInfo]).
final class ControlPlaneDispatcher implements Dispatcher {

    /// `maxDispatchPayloadBytes`.
    static final int MAX_PAYLOAD_BYTES = 256 << 10;

    private final ControlPlane controlPlane;
    private final String functionId;
    private final int version;
    private final List<Describe.DispatchTarget> declared;

    ControlPlaneDispatcher(ControlPlane controlPlane, String functionId, int version, List<Describe.DispatchTarget> declared) {
        this.controlPlane = Objects.requireNonNull(controlPlane, "controlPlane");
        this.functionId = Objects.requireNonNull(functionId, "functionId");
        this.version = version;
        this.declared = List.copyOf(declared);
    }

    @Override
    public String dispatch(DispatchJob job) {
        Objects.requireNonNull(job, "job");
        boolean ok = declared.stream().anyMatch(t -> t.target().equals(job.target()) && t.path().equals(job.path()));
        if (!ok) {
            throw new DispatchException("NOT_DECLARED",
                    "dispatch target " + job.target() + job.path() + " is not in this version's dispatches");
        }
        byte[] payload = job.payload();
        if (payload.length > MAX_PAYLOAD_BYTES) {
            throw new DispatchException("TOO_LARGE", "the dispatch payload is over 256 KiB");
        }
        List<Control.Pair> metadata = new ArrayList<>();
        for (Map.Entry<String, String> e : job.metadata().entrySet()) {
            metadata.add(new Control.Pair(e.getKey(), e.getValue()));
        }
        Control.DispatchBody body = new Control.DispatchBody(
                job.target(), job.path(), job.code(), job.subject(), job.messageGroup(), job.mode(), job.correlationId(),
                job.causationId(), job.contentType(), job.descriptor(), job.maxRetries(), job.timeoutSeconds(), job.dataOnly(),
                metadata);
        InvocationInfo info = InvocationInfo.current();
        return controlPlane.dispatch(new Control.DispatchRequest(
                functionId, version, info.invocationId(), info.requestCorrelationId(), body,
                Base64.getEncoder().encodeToString(payload)));
    }
}
