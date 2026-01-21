package tech.flowcatalyst.controlplane.api;

import jakarta.ws.rs.GET;
import jakarta.ws.rs.Path;
import jakarta.ws.rs.Produces;
import jakarta.ws.rs.core.MediaType;

@Path("/api/stats")
public class StatsResource {

    @GET
    @Produces(MediaType.APPLICATION_JSON)
    public PlatformStats getStats() {
        // TODO: Implement actual stats retrieval
        return new PlatformStats(0, 0, 0);
    }

    public record PlatformStats(
        int eventTypes,
        int subscriptions,
        long messagesProcessed
    ) {}
}
