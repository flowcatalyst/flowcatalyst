package tech.flowcatalyst.messagerouter.factory;

import tech.flowcatalyst.messagerouter.mediator.Mediator;
import tech.flowcatalyst.messagerouter.model.MediationType;

public interface MediatorFactory {

    /**
     * Creates a mediator based on the mediation type
     *
     * @param mediationType the type of mediator to create
     * @return a mediator instance
     * @throws IllegalArgumentException if mediationType is not supported
     */
    Mediator createMediator(MediationType mediationType);
}
