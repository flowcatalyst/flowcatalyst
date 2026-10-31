package io.flowcatalyst.function;

import java.time.Instant;
import java.util.Objects;

/// A scheduled-job firing's envelope, parsed by [Webhook#schedule(Request)]
/// from a `webhook`-endpoint [Request]'s body. Field-for-field the wire
/// shape the platform sends when a scheduled job fires
/// (`internal/platform/scheduledjob/scheduler`, `webhookEnvelope`):
///
/// ```
/// {jobId, jobCode, instanceId, scheduledFor?, firedAt, triggerKind,
///  correlationId?, payload?, tracksCompletion, timeoutSeconds?, concurrent}
/// ```
///
/// `triggerKind` is the platform's `CRON` / `MANUAL` / `BACKFILL` constant,
/// carried as its raw wire string — this jar has no enum for the platform's
/// trigger kinds. `payloadJson` is the raw JSON text of `payload`
/// exactly as it appeared on the wire, `null` when absent — never parsed
/// further here, for the same reason as [Event#dataJson].
///
/// @param jobId            the scheduled job's own id
/// @param jobCode          the scheduled job's code
/// @param instanceId       this firing's instance id
/// @param scheduledFor     when this firing was due, `null` when absent
/// @param firedAt          when this firing actually ran
/// @param triggerKind      `CRON`, `MANUAL` or `BACKFILL`
/// @param correlationId    links this firing to the flow that caused it, `null` when absent
/// @param payloadJson      the raw JSON text of `payload`, `null` when absent
/// @param tracksCompletion whether the scheduler expects this firing to report completion
/// @param timeoutSeconds   the firing's own timeout, `null` when the job has none
/// @param concurrent       whether this job allows overlapping firings
public record Schedule(
        String jobId,
        String jobCode,
        String instanceId,
        Instant scheduledFor,
        Instant firedAt,
        String triggerKind,
        String correlationId,
        String payloadJson,
        boolean tracksCompletion,
        Integer timeoutSeconds,
        boolean concurrent) {

    public Schedule {
        Objects.requireNonNull(jobId, "jobId");
        Objects.requireNonNull(jobCode, "jobCode");
        Objects.requireNonNull(instanceId, "instanceId");
        Objects.requireNonNull(firedAt, "firedAt");
        Objects.requireNonNull(triggerKind, "triggerKind");
    }
}
