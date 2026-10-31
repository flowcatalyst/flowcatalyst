package io.flowcatalyst.ingestor.functions.context;

import io.flowcatalyst.framework.httpclient.HttpClients;
import io.flowcatalyst.framework.httpclient.RetryPolicy;
import io.flowcatalyst.framework.httpclient.ServiceClient;
import io.flowcatalyst.framework.httpclient.ServiceOptions;
import io.flowcatalyst.function.HttpResponseTooLargeException;
import java.net.URI;
import java.time.Duration;
import java.util.concurrent.ConcurrentHashMap;

/// The framework [ServiceClient]s a function's outbound calls go through: one per origin
/// (`scheme://host:port`), built on the application's one [HttpClients] engine, so the calls share its
/// connection pools and carry its capture hook ([io.flowcatalyst.framework.httpclient.HttpCapture]) — an outbound call is
/// traceable like any other the ingestor makes. Not retried (a function decides about repeating its own
/// calls), redirects are never followed (the framework's client does not), and a response over
/// [HttpResponseTooLargeException#MAX_BYTES] fails the call.
public final class OutboundClients {

    /// The name the clients carry in logs, metrics and captured exchanges.
    public static final String SERVICE_NAME = "fn-http";

    /// More origins than this are served by clients that are not kept.
    private static final int MAX_KEPT = 512;

    private final HttpClients engine;
    private final ConcurrentHashMap<String, ServiceClient> byOrigin = new ConcurrentHashMap<>();

    public OutboundClients(HttpClients engine) {
        this.engine = engine;
    }

    public ServiceClient forOrigin(URI origin) {
        String key = origin.getScheme().toLowerCase(java.util.Locale.ROOT) + "://" + origin.getAuthority().toLowerCase(java.util.Locale.ROOT);
        ServiceClient kept = byOrigin.get(key);
        if (kept != null) {
            return kept;
        }
        ServiceClient created = engine.service(ServiceOptions.builder(SERVICE_NAME, URI.create(key))
                .connectTimeout(Duration.ofSeconds(10))
                .requestTimeout(Duration.ofSeconds(30))
                .maxResponseBytes(HttpResponseTooLargeException.MAX_BYTES)
                .retry(RetryPolicy.none())
                .build());
        if (byOrigin.size() < MAX_KEPT) {
            ServiceClient raced = byOrigin.putIfAbsent(key, created);
            return raced == null ? created : raced;
        }
        return created;
    }
}
