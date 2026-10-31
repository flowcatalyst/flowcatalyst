package io.flowcatalyst.ingestor.functions.control;

import io.flowcatalyst.function.DispatchException;
import io.flowcatalyst.function.EmitResult;
import java.time.Duration;
import org.jspecify.annotations.Nullable;

/// The platform as the function host sees it (`internal/functions/runner` `ControlPlane`):
/// [HttpControlPlane] in production, a fake in tests.
public interface ControlPlane {

    /// `GET /control/functions/desired?pool=P&wait=Ns&runtimes=jvm`, `If-None-Match: etag` when
    /// `etag` is not null. With an etag the platform holds the request up to `wait` and answers 304
    /// ([Fetched.NotModified]) when nothing changed.
    ///
    /// @throws ControlPlaneException the request could not be completed — the caller keeps serving
    ///                                what is already loaded
    Fetched desired(String pool, @Nullable String etag, Duration wait) throws ControlPlaneException;

    /// `POST /control/functions/heartbeat`.
    void heartbeat(Control.Heartbeat heartbeat) throws ControlPlaneException;

    /// `GET /control/functions/artifacts/{digest}`: the artifact's bytes (a 302 to the presigned URL is
    /// followed), whose sha256 has been verified against `digest`.
    ///
    /// @throws ControlPlaneException the bytes could not be fetched or are not the artifact named
    byte[] artifact(String digest) throws ControlPlaneException;

    /// `POST /control/functions/events` on a function's behalf. Never throws: a refusal or an
    /// unreachable platform is a [EmitResult.Refused].
    EmitResult emit(Control.EmitRequest request);

    /// `POST /control/functions/dispatch-jobs` on a function's behalf: the job id.
    ///
    /// @throws DispatchException the platform refused the job or could not be reached
    String dispatch(Control.DispatchRequest request) throws DispatchException;

    /// The outcome of one [#desired] call.
    sealed interface Fetched permits Fetched.NotModified, Fetched.Changed {

        /// `304`: the caller's etag is still current.
        record NotModified() implements Fetched {
        }

        /// `200`, with the document's etag.
        record Changed(@Nullable String etag, Control.Desired document) implements Fetched {
        }
    }
}
