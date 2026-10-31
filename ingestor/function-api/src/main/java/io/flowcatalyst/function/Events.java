package io.flowcatalyst.function;

/// Emits platform events on a function's behalf, through the host — no
/// application credential is ever handed to the function. A function may emit
/// only event types it declared in `describe.emits`, owned by its application.
public interface Events {

    /// Emits `event`, answering [EmitResult.Emitted] with the stored event's id,
    /// or [EmitResult.Refused] when the platform refused it (an unowned type, a
    /// bad or duplicate dedup id, …) or could not be reached. Never throws for
    /// either.
    EmitResult emit(OutboundEvent event);
}
