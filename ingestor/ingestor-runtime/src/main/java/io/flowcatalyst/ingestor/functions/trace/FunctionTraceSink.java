package io.flowcatalyst.ingestor.functions.trace;

import io.flowcatalyst.ingestor.functions.control.Control;

/// Where a finished trace record goes. An implementation never blocks and never throws: tracing may not slow or fail
/// a call.
@FunctionalInterface
public interface FunctionTraceSink {

    /// Takes nothing: the sink of a host that does not ship traces.
    FunctionTraceSink DISCARD = record -> {
    };

    /// Takes one record.
    void offer(Control.TraceRecord record);
}
