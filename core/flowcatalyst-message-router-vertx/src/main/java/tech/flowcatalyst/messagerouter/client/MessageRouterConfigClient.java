package tech.flowcatalyst.messagerouter.client;

import jakarta.ws.rs.GET;
import jakarta.ws.rs.Path;
import jakarta.ws.rs.Produces;
import jakarta.ws.rs.core.MediaType;
import org.eclipse.microprofile.rest.client.inject.RegisterRestClient;
import tech.flowcatalyst.messagerouter.config.MessageRouterConfig;

@RegisterRestClient(configKey = "message-router-config")
public interface MessageRouterConfigClient {

    @GET
    @Produces(MediaType.APPLICATION_JSON)
    MessageRouterConfig getQueueConfig();
}
