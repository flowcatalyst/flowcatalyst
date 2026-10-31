package io.flowcatalyst.ingestor.functions.control;

import java.util.List;
import java.util.Map;
import org.jspecify.annotations.Nullable;
import tools.jackson.databind.JsonNode;

/// The wire contract with the platform's function control plane, as the Go runner speaks it
/// (`internal/functions/control/control.go`): the desired-state document, the heartbeat, the emit request and
/// the dispatch request. Field names are the JSON names. The desired document contains secrets: it is
/// never logged.
public final class Control {

    private Control() {
    }

    /// Routes under the platform's base URL; all need a bearer token whose principal holds `platform:function-runner`.
    public static final String PATH_DESIRED = "/control/functions/desired";
    public static final String PATH_HEARTBEAT = "/control/functions/heartbeat";
    public static final String PATH_ARTIFACTS = "/control/functions/artifacts";
    public static final String PATH_EVENTS = "/control/functions/events";
    public static final String PATH_DISPATCH_JOBS = "/control/functions/dispatch-jobs";

    /// The runtime tag this host serves.
    public static final String RUNTIME_JVM = "jvm";

    /// `control.ProtocolV1`.
    public static final int PROTOCOL_V1 = 1;

    /// `control.MaxWait`: the longest long-poll the platform holds.
    public static final int MAX_WAIT_SECONDS = 60;

    /// Roles of a version in the desired document.
    public static final String ROLE_LIVE = "live";
    public static final String ROLE_CANDIDATE = "candidate";
    public static final String ROLE_ALIAS_PREFIX = "alias:";

    /// Version states a runner reports.
    public static final String STATE_LOADED = "LOADED";
    public static final String STATE_FAILED = "FAILED";
    public static final String STATE_RETRYING = "RETRYING";

    /// Everything a pool's runners should hold.
    public record Desired(
            @Nullable String pool, long revision, @Nullable TokenAuth auth, List<FunctionDoc> functions) {
        public Desired {
            functions = functions == null ? List.of() : List.copyOf(functions);
        }
    }

    /// The issuer and audience of the platform's access tokens (unused here: tokens are verified with the
    /// ingestor's own `PlatformTokens`).
    public record TokenAuth(@Nullable String issuer, @Nullable String audience) {
    }

    /// One function the pool runs.
    public record FunctionDoc(
            String id,
            String address,
            @Nullable String applicationId,
            @Nullable String clientId,
            @Nullable Limits limits,
            boolean warm,
            @Nullable String webhookSecret,
            Map<String, String> config,
            Map<String, String> secrets,
            Map<String, String> db,
            List<VersionDoc> versions) {
        public FunctionDoc {
            config = config == null ? Map.of() : Map.copyOf(config);
            secrets = secrets == null ? Map.of() : Map.copyOf(secrets);
            db = db == null ? Map.of() : Map.copyOf(db);
            versions = versions == null ? List.of() : List.copyOf(versions);
        }

        /// The limits with the runner's defaults for what the platform did not send (`limitsOf`).
        public Limits effectiveLimits() {
            return limits == null ? Limits.DEFAULTS : limits.withDefaults();
        }
    }

    /// A function's resource limits.
    public record Limits(int memoryMb, int maxConcurrency, long timeoutMs, long maxBodyBytes) {

        public static final int DEFAULT_MEMORY_MB = 64;
        public static final int DEFAULT_MAX_CONCURRENCY = 16;
        public static final long DEFAULT_TIMEOUT_MS = 30_000;
        public static final long DEFAULT_MAX_BODY_BYTES = 1 << 20;

        public static final Limits DEFAULTS =
                new Limits(DEFAULT_MEMORY_MB, DEFAULT_MAX_CONCURRENCY, DEFAULT_TIMEOUT_MS, DEFAULT_MAX_BODY_BYTES);

        public Limits withDefaults() {
            return new Limits(
                    memoryMb <= 0 ? DEFAULT_MEMORY_MB : memoryMb,
                    maxConcurrency <= 0 ? DEFAULT_MAX_CONCURRENCY : maxConcurrency,
                    timeoutMs <= 0 ? DEFAULT_TIMEOUT_MS : timeoutMs,
                    maxBodyBytes <= 0 ? DEFAULT_MAX_BODY_BYTES : maxBodyBytes);
        }
    }

    /// One version the runner should hold, and why. `describe` stays raw JSON until the host reads it.
    public record VersionDoc(
            int number, String digest, @Nullable String runtime, int abi, @Nullable JsonNode describe, List<String> roles) {
        public VersionDoc {
            roles = roles == null ? List.of() : List.copyOf(roles);
        }
    }

    /// A runner's report to the platform.
    public record Heartbeat(
            String runnerId,
            String pool,
            String startedAt,
            long revision,
            BudgetReport budget,
            List<VersionReport> versions,
            RunnerCapabilities capabilities) {
    }

    /// The runner's memory picture (the JVM's, here).
    public record BudgetReport(
            long limitBytes,
            long reserveBytes,
            long budgetBytes,
            long linearBytes,
            long refused,
            long compiledBytes,
            long rssBytes,
            int loaded,
            long evictions) {
    }

    /// What a runner can run.
    public record RunnerCapabilities(
            int protocol, List<String> runtimes, String runnerKind, String version, Map<String, Object> extra) {
    }

    /// One version's state on the runner.
    public record VersionReport(String functionId, int number, String state, @Nullable String reason) {
    }

    /// `control.EmitRequest`: `data` is base64 on the wire.
    public record EmitRequest(String functionId, int version, EmitEvent event, String data) {
    }

    /// `abi.Event`.
    public record EmitEvent(
            String type,
            @Nullable String source,
            @Nullable String subject,
            String dedupId,
            @Nullable String correlationId,
            @Nullable String causationId,
            @Nullable String messageGroup,
            @Nullable String contentType) {
    }

    /// `control.EmitResponse`.
    public record EmitResponse(@Nullable String eventId) {
    }

    /// `control.DispatchRequest`: `payload` is base64 on the wire.
    public record DispatchRequest(
            String functionId,
            int version,
            @Nullable String invocationId,
            @Nullable String requestCorrelationId,
            DispatchBody dispatch,
            String payload) {
    }

    /// `abi.Dispatch`.
    public record DispatchBody(
            String target,
            String path,
            @Nullable String code,
            @Nullable String subject,
            @Nullable String messageGroup,
            @Nullable String mode,
            @Nullable String correlationId,
            @Nullable String causationId,
            @Nullable String contentType,
            @Nullable String descriptor,
            @Nullable Integer maxRetries,
            @Nullable Integer timeoutSeconds,
            @Nullable Boolean dataOnly,
            List<Pair> metadata) {
    }

    /// `abi.KV`: one dispatch-job metadata entry.
    public record Pair(String key, String value) {
    }

    /// `control.DispatchResponse`.
    public record DispatchResponse(@Nullable String jobId) {
    }
}
