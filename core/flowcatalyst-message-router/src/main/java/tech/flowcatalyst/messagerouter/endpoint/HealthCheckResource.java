package tech.flowcatalyst.messagerouter.endpoint;

import jakarta.inject.Inject;
import jakarta.ws.rs.GET;
import jakarta.ws.rs.Path;
import jakarta.ws.rs.Produces;
import jakarta.ws.rs.core.MediaType;
import jakarta.ws.rs.core.Response;
import org.eclipse.microprofile.openapi.annotations.Operation;
import org.eclipse.microprofile.openapi.annotations.tags.Tag;
import tech.flowcatalyst.messagerouter.health.InfrastructureHealthService;

/**
 * Infrastructure health check endpoint for load balancers and orchestrators.
 * Returns HTTP 200 if infrastructure is operational, HTTP 503 if compromised.
 */
@Path("/health")
@Tag(name = "Health", description = "Infrastructure health check for load balancers")
public class HealthCheckResource {

    @Inject
    InfrastructureHealthService infrastructureHealthService;

    @GET
    @Produces(MediaType.APPLICATION_JSON)
    @Operation(
        summary = "Infrastructure health check",
        description = "Returns 200 if message router infrastructure is operational, 503 if compromised. " +
                     "Does not fail for downstream service issues (circuit breakers, mediation failures)."
    )
    public Response healthCheck() {
        InfrastructureHealthService.InfrastructureHealth health = infrastructureHealthService.checkHealth();

        if (health.healthy()) {
            return Response.ok(health).build();
        } else {
            return Response.status(Response.Status.SERVICE_UNAVAILABLE)
                .entity(health)
                .build();
        }
    }
}
