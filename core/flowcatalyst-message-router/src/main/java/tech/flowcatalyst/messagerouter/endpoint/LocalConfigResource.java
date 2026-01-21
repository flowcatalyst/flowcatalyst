package tech.flowcatalyst.messagerouter.endpoint;

import io.quarkus.arc.profile.IfBuildProfile;
import jakarta.ws.rs.GET;
import jakarta.ws.rs.Path;
import jakarta.ws.rs.Produces;
import jakarta.ws.rs.core.MediaType;
import org.eclipse.microprofile.openapi.annotations.Operation;
import org.eclipse.microprofile.openapi.annotations.tags.Tag;
import tech.flowcatalyst.messagerouter.config.MessageRouterConfig;
import tech.flowcatalyst.messagerouter.config.ProcessingPool;
import tech.flowcatalyst.messagerouter.config.QueueConfig;
import tech.flowcatalyst.messagerouter.security.Protected;

import java.util.List;

@Path("/api")
@Tag(name = "Local Configuration", description = "Local configuration endpoint for development and testing")
@Protected("Config endpoint requiring authentication")
public class LocalConfigResource {

    @GET
    @Path("/config")
    @Produces(MediaType.APPLICATION_JSON)
    @Operation(summary = "Get queue configuration", description = "Returns the default queue configuration for local development")
    public MessageRouterConfig getQueueConfig() {
        return new MessageRouterConfig(
            List.of(
                new QueueConfig(null, "http://localhost:4566/000000000000/flow-catalyst-high-priority.fifo", null),
                new QueueConfig(null, "http://localhost:4566/000000000000/flow-catalyst-medium-priority.fifo", null),
                new QueueConfig(null, "http://localhost:4566/000000000000/flow-catalyst-low-priority.fifo", null),
                new QueueConfig(null, "http://localhost:4566/000000000000/flow-catalyst-dispatch.fifo", null)
            ),
            1,
            List.of(
                new ProcessingPool("POOL-HIGH", 10, null),
                new ProcessingPool("POOL-MEDIUM", 5, null),
                new ProcessingPool("POOL-LOW", 2, null),
                new ProcessingPool("DISPATCH-POOL", 5, null)
            )
        );
    }
}
