package io.flowcatalyst.ingestor.functions.context;

import io.flowcatalyst.function.EmitResult;
import io.flowcatalyst.function.Events;
import io.flowcatalyst.function.OutboundEvent;
import io.flowcatalyst.ingestor.functions.control.Control;
import io.flowcatalyst.ingestor.functions.control.ControlPlane;
import java.util.Base64;
import java.util.List;
import java.util.Objects;

/// [Events] over [ControlPlane#emit]: one `POST /control/functions/events` per [#emit] call, with this loaded
/// version's function id and number so the platform can check the type against the version's own describe.
/// As the Go runner does (`capabilities.emit`): a type not in the version's `emits` is refused here without a
/// call (`NOT_DECLARED`), the source defaults to `function:<address>`, and nothing else is defaulted (the
/// function sets its own correlation and causation ids).
final class ControlPlaneEvents implements Events {

    private final ControlPlane controlPlane;
    private final String functionId;
    private final String address;
    private final int version;
    private final List<String> emits;

    ControlPlaneEvents(ControlPlane controlPlane, String functionId, String address, int version, List<String> emits) {
        this.controlPlane = Objects.requireNonNull(controlPlane, "controlPlane");
        this.functionId = Objects.requireNonNull(functionId, "functionId");
        this.address = Objects.requireNonNull(address, "address");
        this.version = version;
        this.emits = List.copyOf(emits);
    }

    @Override
    public EmitResult emit(OutboundEvent event) {
        Objects.requireNonNull(event, "event");
        if (!emits.contains(event.type())) {
            return new EmitResult.Refused("NOT_DECLARED", 403, "event type " + event.type() + " is not in this version's emits");
        }
        String source = event.source() == null || event.source().isBlank() ? "function:" + address : event.source();
        Control.EmitEvent body = new Control.EmitEvent(
                event.type(), source, event.subject(), event.dedupId(), event.correlationId(), event.causationId(),
                event.messageGroup(), event.dataContentType());
        String data = Base64.getEncoder().encodeToString(event.data());
        return controlPlane.emit(new Control.EmitRequest(functionId, version, body, data));
    }
}
