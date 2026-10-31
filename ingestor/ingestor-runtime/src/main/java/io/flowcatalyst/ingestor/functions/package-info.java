/// The ingestor runtime's function role: it hosts JVM functions (docs/jvm-functions-in-ingestor.md) next to
/// ingestion. It fetches the platform's desired state for its pool, loads each `jvm` version's jar into an
/// isolated class loader, serves `/fn/{address}[@{alias}|@v{n}]/{path}` on the framework's HTTP server with the
/// invocation path of the Go function runner, and reports what it holds in heartbeats.
///
/// Copied from flowcatalyst-javalin function-host at 43e2fc48c33000c3b3d09d11b6e69b940389db5a: the
/// engine (`load`, `reconcile`, `context`, the invocation runner and pinned versions) is javalin's; the platform
/// edges (control protocol, listener, auth, describe document) follow this platform's contract
/// (`internal/functions/control`, `internal/functions/runner`).
@NullMarked
package io.flowcatalyst.ingestor.functions;

import org.jspecify.annotations.NullMarked;
