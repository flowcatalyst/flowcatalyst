package tech.flowcatalyst.messagerouter.benchmark;

import jakarta.ws.rs.*;
import jakarta.ws.rs.core.MediaType;
import jakarta.ws.rs.core.Response;
import org.jboss.logging.Logger;

import java.util.Map;
import java.util.concurrent.atomic.AtomicLong;

/**
 * Mock endpoint for benchmarking message router throughput.
 * Returns immediate 200 OK responses to measure pure routing performance.
 */
@Path("/api/benchmark")
public class BenchmarkResource {

    private static final Logger LOG = Logger.getLogger(BenchmarkResource.class);

    private final AtomicLong requestCount = new AtomicLong(0);
    private final AtomicLong startTime = new AtomicLong(0);

    /**
     * Mock processing endpoint - immediately returns 200 OK
     */
    @POST
    @Path("/process")
    @Consumes(MediaType.APPLICATION_JSON)
    @Produces(MediaType.APPLICATION_JSON)
    public Response process(Map<String, Object> payload) {
        long count = requestCount.incrementAndGet();

        // Initialize start time on first request
        if (count == 1) {
            startTime.set(System.currentTimeMillis());
        }

        // Log every 100 requests
        if (count % 100 == 0) {
            long elapsed = System.currentTimeMillis() - startTime.get();
            double throughput = count / (elapsed / 1000.0);
            LOG.infof("Processed %d requests (%.2f req/s)", count, throughput);
        }

        return Response.ok(Map.of(
            "status", "ok",
            "requestId", count,
            "timestamp", System.currentTimeMillis()
        )).build();
    }

    /**
     * Mock endpoint with simulated processing delay
     */
    @POST
    @Path("/process-slow")
    @Consumes(MediaType.APPLICATION_JSON)
    @Produces(MediaType.APPLICATION_JSON)
    public Response processSlow(
            @QueryParam("delayMs") @DefaultValue("100") int delayMs,
            Map<String, Object> payload) throws InterruptedException {

        Thread.sleep(delayMs);
        long count = requestCount.incrementAndGet();

        return Response.ok(Map.of(
            "status", "ok",
            "requestId", count,
            "delayMs", delayMs,
            "timestamp", System.currentTimeMillis()
        )).build();
    }

    /**
     * Get current benchmark stats
     */
    @GET
    @Path("/stats")
    @Produces(MediaType.APPLICATION_JSON)
    public Response stats() {
        long count = requestCount.get();
        long start = startTime.get();
        long elapsed = start > 0 ? System.currentTimeMillis() - start : 0;
        double throughput = start > 0 ? count / (elapsed / 1000.0) : 0;

        return Response.ok(Map.of(
            "totalRequests", count,
            "elapsedMs", elapsed,
            "throughputPerSecond", throughput
        )).build();
    }

    /**
     * Reset benchmark stats
     */
    @POST
    @Path("/reset")
    public Response reset() {
        requestCount.set(0);
        startTime.set(0);
        LOG.info("Benchmark stats reset");
        return Response.ok(Map.of("status", "reset")).build();
    }
}
