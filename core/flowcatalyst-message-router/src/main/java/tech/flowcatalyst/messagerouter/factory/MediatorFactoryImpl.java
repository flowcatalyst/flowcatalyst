package tech.flowcatalyst.messagerouter.factory;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import org.jboss.logging.Logger;
import tech.flowcatalyst.messagerouter.mediator.HttpMediator;
import tech.flowcatalyst.messagerouter.mediator.Mediator;
import tech.flowcatalyst.messagerouter.model.MediationType;

@ApplicationScoped
public class MediatorFactoryImpl implements MediatorFactory {

    private static final Logger LOG = Logger.getLogger(MediatorFactoryImpl.class);

    @Inject
    HttpMediator httpMediator;

    @Override
    public Mediator createMediator(MediationType mediationType) {
        LOG.debugf("Creating mediator for type: %s", mediationType);

        return switch (mediationType) {
            case HTTP -> httpMediator;
        };
    }
}
