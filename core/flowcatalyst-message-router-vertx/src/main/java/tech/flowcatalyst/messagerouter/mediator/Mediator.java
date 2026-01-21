package tech.flowcatalyst.messagerouter.mediator;

import tech.flowcatalyst.messagerouter.model.MediationOutcome;
import tech.flowcatalyst.messagerouter.model.MediationType;
import tech.flowcatalyst.messagerouter.model.MessagePointer;

public interface Mediator {

    /**
     * Processes a message pointer by mediating to the downstream system.
     *
     * @param message the message pointer to process
     * @return the outcome of the mediation, including result and optional delay for retries
     */
    MediationOutcome process(MessagePointer message);

    /**
     * Returns the mediation type this mediator handles.
     */
    MediationType getMediationType();
}
