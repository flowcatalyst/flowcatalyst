package tech.flowcatalyst.dispatchjob.model;

/**
 * Interface for message mediators that process message pointers.
 *
 * NOTE: This is a copy of tech.flowcatalyst.messagerouter.mediator.Mediator
 * kept in sync for the backend's dispatch job functionality.
 */
public interface Mediator {

    /**
     * Processes a message pointer by mediating to the downstream system
     *
     * @param message the message pointer to process
     * @return the result of the mediation
     */
    MediationResult process(MessagePointer message);

    /**
     * Returns the mediation type this mediator handles
     */
    MediationType getMediationType();
}
