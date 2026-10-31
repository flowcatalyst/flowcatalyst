package io.flowcatalyst.function.testing;

import io.flowcatalyst.function.EmitResult;
import io.flowcatalyst.function.Events;
import io.flowcatalyst.function.OutboundEvent;

import java.util.List;
import java.util.concurrent.CopyOnWriteArrayList;

/// An [Events] test double: records every emitted event and answers
/// `evt_recorded_{n}`; [#refuseNext] makes the next [#emit] answer a refusal
/// instead (once), so a test can drive a function's refusal branch.
public final class RecordingEvents implements Events {

    private final List<OutboundEvent> emitted = new CopyOnWriteArrayList<>();
    private volatile EmitResult.Refused nextRefusal;

    /// Makes the next [#emit] (only) answer `refusal`; the event is not recorded.
    public void refuseNext(EmitResult.Refused refusal) {
        this.nextRefusal = refusal;
    }

    /// The events accepted so far, in order.
    public List<OutboundEvent> emitted() {
        return emitted;
    }

    @Override
    public EmitResult emit(OutboundEvent event) {
        EmitResult.Refused pending = nextRefusal;
        if (pending != null) {
            nextRefusal = null;
            return pending;
        }
        emitted.add(event);
        return new EmitResult.Emitted("evt_recorded_" + emitted.size());
    }
}
