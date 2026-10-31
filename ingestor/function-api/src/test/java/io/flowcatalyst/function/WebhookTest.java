package io.flowcatalyst.function;

import java.nio.charset.StandardCharsets;
import java.time.Instant;
import java.util.Map;

import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

/// `Webhook.event` / `dataOnly` / `schedule` against the wire shapes the
/// platform sends:
///
/// - delivery envelope: `internal/platform/dispatchjob/processing`
///   `buildPayload`: `{id, type, attemptNumber, time?, dataContentType?, source?,
///   subject?, correlationId?, messageGroup?, clientId?, clientCode?, data?}`.
/// - `dataOnly` delivery: the payload as the body; `X-Dispatch-Job-Id`,
///   `X-Event-Type`, `X-FlowCatalyst-Client` headers.
/// - scheduled-job firing: `internal/platform/scheduledjob/scheduler`
///   `webhookEnvelope`: `{jobId, jobCode, instanceId, scheduledFor?, firedAt,
///   triggerKind, correlationId?, payload?, tracksCompletion, timeoutSeconds?, concurrent}`.
class WebhookTest {

    @Test
    void parsesAFullEventEnvelope() {
        String body = """
                {"id":"djb_123","type":"billing:invoices:invoice:created","attemptNumber":2,
                 "source":"billing-service","subject":"inv_1","correlationId":"corr-1",
                 "messageGroup":"group-1","clientId":"cli_1","clientCode":"acme",
                 "time":"2026-09-19T10:15:30.123Z","dataContentType":"application/json",
                 "data":{"invoiceId":"inv_1","amount":100}}
                """;
        Event event = Webhook.event(request(body));

        assertThat(event.id()).isEqualTo("djb_123");
        assertThat(event.type()).isEqualTo("billing:invoices:invoice:created");
        assertThat(event.attemptNumber()).isEqualTo(2);
        assertThat(event.source()).isEqualTo("billing-service");
        assertThat(event.subject()).isEqualTo("inv_1");
        assertThat(event.correlationId()).isEqualTo("corr-1");
        assertThat(event.messageGroup()).isEqualTo("group-1");
        assertThat(event.clientId()).isEqualTo("cli_1");
        assertThat(event.clientCode()).isEqualTo("acme");
        assertThat(event.time()).isEqualTo(Instant.parse("2026-09-19T10:15:30.123Z"));
        assertThat(event.dataContentType()).isEqualTo("application/json");
        assertThat(event.dataJson()).isEqualTo("{\"invoiceId\":\"inv_1\",\"amount\":100}");
    }

    @Test
    void parsesTheEnvelopeExactlyAsTheGoPlatformMarshalsIt() {
        // Key order and millisecond UTC time as buildPayload writes them (encoding/json sorts map keys).
        String body = "{\"attemptNumber\":1,\"clientCode\":\"acme\",\"clientId\":\"clt_1\","
                + "\"data\":{\"k\":\"v\"},\"dataContentType\":\"application/json\",\"id\":\"djb_1\","
                + "\"time\":\"2026-10-31T08:00:00.000Z\",\"type\":\"orders:order:created\"}";
        Event event = Webhook.event(request(body));
        assertThat(event.id()).isEqualTo("djb_1");
        assertThat(event.type()).isEqualTo("orders:order:created");
        assertThat(event.attemptNumber()).isEqualTo(1);
        assertThat(event.clientId()).isEqualTo("clt_1");
        assertThat(event.clientCode()).isEqualTo("acme");
        assertThat(event.time()).isEqualTo(Instant.parse("2026-10-31T08:00:00.000Z"));
        assertThat(event.dataJson()).isEqualTo("{\"k\":\"v\"}");
    }

    @Test
    void eventRejectsAMalformedTime() {
        assertThatThrownBy(() -> Webhook.event(request(
                "{\"id\":\"i\",\"type\":\"t\",\"attemptNumber\":1,\"time\":\"yesterday\"}")))
                .isInstanceOf(WebhookFormatException.class);
    }

    // ---- dataOnly ----

    @Test
    void dataOnlyReadsTheHeadersAndKeepsTheBodyVerbatim() {
        Request in = new Request("inv-1", FunctionAddress.parse("billing.api"), 1, "POST", "/events", "", 
                Map.of("x-dispatch-job-id", java.util.List.of("djb_9"), "X-Event-Type", java.util.List.of("orders:order:created"),
                        "X-FlowCatalyst-Client", java.util.List.of("clt_1:acme"),
                        "Content-Type", java.util.List.of("text/csv")),
                "", Map.of(), Caller.Platform.INSTANCE, Instant.parse("2099-01-01T00:00:00Z"), "", "", false,
                "a,b\n1,2".getBytes(StandardCharsets.UTF_8));
        DataOnly d = Webhook.dataOnly(in);
        assertThat(d.jobId()).isEqualTo("djb_9");
        assertThat(d.eventType()).isEqualTo("orders:order:created");
        assertThat(d.clientId()).isEqualTo("clt_1");
        assertThat(d.clientCode()).isEqualTo("acme");
        assertThat(d.contentType()).isEqualTo("text/csv");
        assertThat(new String(d.body(), StandardCharsets.UTF_8)).isEqualTo("a,b\n1,2");
    }

    @Test
    void dataOnlyWithNoHeadersGivesNullFieldsAndAnyBodyIsAPayload() {
        DataOnly d = Webhook.dataOnly(request("not json at all"));
        assertThat(d.jobId()).isNull();
        assertThat(d.eventType()).isNull();
        assertThat(d.clientId()).isNull();
        assertThat(d.clientCode()).isNull();
        assertThat(d.contentType()).isNull();
        assertThat(new String(d.body(), StandardCharsets.UTF_8)).isEqualTo("not json at all");
    }

    @Test
    void dataOnlyIgnoresAHalfPairClientHeader() {
        Request in = new Request("inv-1", FunctionAddress.parse("billing.api"), 1, "POST", "/e", "",
                Map.of("X-FlowCatalyst-Client", java.util.List.of("clt_1")), "", Map.of(), Caller.Platform.INSTANCE,
                Instant.parse("2099-01-01T00:00:00Z"), "", "", false, new byte[0]);
        DataOnly d = Webhook.dataOnly(in);
        assertThat(d.clientId()).isNull();
        assertThat(d.clientCode()).isNull();
    }

    @Test
    void dataOnlyBodyIsDefensivelyCopied() {
        DataOnly d = Webhook.dataOnly(request("abc"));
        d.body()[0] = 'z';
        assertThat(new String(d.body(), StandardCharsets.UTF_8)).isEqualTo("abc");
    }

    @Test
    void everyOptionalEventFieldMayBeAbsentEntirely() {
        // NON_ABSENT drops null fields — the key is missing, not present with a null value
        // (dispatch-seam spec §5, DeliveryPayload's Envelope over Json.MAPPER).
        String body = "{\"id\":\"djb_1\",\"type\":\"t\",\"attemptNumber\":1}";
        Event event = Webhook.event(request(body));

        assertThat(event.source()).isNull();
        assertThat(event.subject()).isNull();
        assertThat(event.correlationId()).isNull();
        assertThat(event.messageGroup()).isNull();
        assertThat(event.clientId()).isNull();
        assertThat(event.clientCode()).isNull();
        assertThat(event.dataJson()).isNull();
        assertThat(event.time()).isNull();
        assertThat(event.dataContentType()).isNull();
    }

    @Test
    void nonJsonPayloadDataPassesThroughAsALiteralJsonString() {
        // DeliveryPayload.dataValue: a non-JSON payload string is embedded as the literal
        // string value of `data`, not dropped.
        String body = "{\"id\":\"djb_1\",\"type\":\"t\",\"attemptNumber\":1,\"data\":\"not json {\"}";
        Event event = Webhook.event(request(body));
        assertThat(event.dataJson()).isEqualTo("\"not json {\"");
    }

    @Test
    void eventRequiresIdAndType() {
        assertThatThrownBy(() -> Webhook.event(request("{\"type\":\"t\",\"attemptNumber\":1}")))
                .isInstanceOf(WebhookFormatException.class);
        assertThatThrownBy(() -> Webhook.event(request("{\"id\":\"i\",\"attemptNumber\":1}")))
                .isInstanceOf(WebhookFormatException.class);
        assertThatThrownBy(() -> Webhook.event(request("{\"id\":\"i\",\"type\":\"t\"}")))
                .isInstanceOf(WebhookFormatException.class);
    }

    @Test
    void eventBodyThatIsNotAJsonObjectIsRejected() {
        // A dataOnly:true subscription sends the raw payload, not this envelope —
        // Webhook.event is only for dataOnly:false; a bare array/string/number is refused.
        assertThatThrownBy(() -> Webhook.event(request("[1,2,3]"))).isInstanceOf(WebhookFormatException.class);
        assertThatThrownBy(() -> Webhook.event(request("not json"))).isInstanceOf(WebhookFormatException.class);
        assertThatThrownBy(() -> Webhook.event(request(""))).isInstanceOf(WebhookFormatException.class);
    }

    @Test
    void parsesAFullScheduleEnvelope() {
        String body = """
                {"jobId":"sjb_1","jobCode":"nightly-report","instanceId":"sji_1",
                 "scheduledFor":"2026-09-19T00:00:00.000000Z","firedAt":"2026-09-19T00:00:01.500000Z",
                 "triggerKind":"CRON","correlationId":"corr-9","payload":{"n":1},
                 "tracksCompletion":true,"timeoutSeconds":30,"concurrent":false}
                """;
        Schedule schedule = Webhook.schedule(request(body));

        assertThat(schedule.jobId()).isEqualTo("sjb_1");
        assertThat(schedule.jobCode()).isEqualTo("nightly-report");
        assertThat(schedule.instanceId()).isEqualTo("sji_1");
        assertThat(schedule.scheduledFor()).isEqualTo(Instant.parse("2026-09-19T00:00:00.000000Z"));
        assertThat(schedule.firedAt()).isEqualTo(Instant.parse("2026-09-19T00:00:01.500000Z"));
        assertThat(schedule.triggerKind()).isEqualTo("CRON");
        assertThat(schedule.correlationId()).isEqualTo("corr-9");
        assertThat(schedule.payloadJson()).isEqualTo("{\"n\":1}");
        assertThat(schedule.tracksCompletion()).isTrue();
        assertThat(schedule.timeoutSeconds()).isEqualTo(30);
        assertThat(schedule.concurrent()).isFalse();
    }

    @Test
    void everyOptionalScheduleFieldMayBeAbsentEntirely() {
        String body = """
                {"jobId":"sjb_1","jobCode":"nightly-report","instanceId":"sji_1",
                 "firedAt":"2026-09-19T00:00:01Z","triggerKind":"MANUAL",
                 "tracksCompletion":false,"concurrent":true}
                """;
        Schedule schedule = Webhook.schedule(request(body));

        assertThat(schedule.scheduledFor()).isNull();
        assertThat(schedule.correlationId()).isNull();
        assertThat(schedule.payloadJson()).isNull();
        assertThat(schedule.timeoutSeconds()).isNull();
    }

    @Test
    void scheduleRequiresItsMandatoryFields() {
        assertThatThrownBy(() -> Webhook.schedule(request(
                "{\"jobCode\":\"c\",\"instanceId\":\"i\",\"firedAt\":\"2026-09-19T00:00:00Z\","
                        + "\"triggerKind\":\"CRON\",\"tracksCompletion\":true,\"concurrent\":false}")))
                .isInstanceOf(WebhookFormatException.class);
        assertThatThrownBy(() -> Webhook.schedule(request(
                "{\"jobId\":\"j\",\"jobCode\":\"c\",\"instanceId\":\"i\","
                        + "\"triggerKind\":\"CRON\",\"tracksCompletion\":true,\"concurrent\":false}")))
                .isInstanceOf(WebhookFormatException.class);
    }

    @Test
    void scheduleRejectsAMalformedTimestamp() {
        assertThatThrownBy(() -> Webhook.schedule(request(
                "{\"jobId\":\"j\",\"jobCode\":\"c\",\"instanceId\":\"i\",\"firedAt\":\"not-a-time\","
                        + "\"triggerKind\":\"CRON\",\"tracksCompletion\":true,\"concurrent\":false}")))
                .isInstanceOf(WebhookFormatException.class);
    }

    @Test
    void parsesAScheduleExactlyAsTheGoPlatformMarshalsIt() {
        // time.Time marshals as RFC 3339 with nanoseconds trimmed, or a numeric offset for a non-UTC zone.
        String body = "{\"jobId\":\"sjb_1\",\"jobCode\":\"nightly\",\"instanceId\":\"sji_1\","
                + "\"scheduledFor\":\"2026-10-31T02:00:00+02:00\",\"firedAt\":\"2026-10-31T00:00:00.123456789Z\","
                + "\"triggerKind\":\"BACKFILL\",\"tracksCompletion\":false,\"concurrent\":true}";
        Schedule schedule = Webhook.schedule(request(body));
        assertThat(schedule.scheduledFor()).isEqualTo(Instant.parse("2026-10-31T00:00:00Z"));
        assertThat(schedule.firedAt()).isEqualTo(Instant.parse("2026-10-31T00:00:00.123456789Z"));
        assertThat(schedule.triggerKind()).isEqualTo("BACKFILL");
    }

    @Test
    void scheduleBodyThatIsNotAnObjectIsRejected() {
        assertThatThrownBy(() -> Webhook.schedule(request("[]"))).isInstanceOf(WebhookFormatException.class);
    }

    private static Request request(String body) {
        return new Request("invocation-1", FunctionAddress.parse("billing.api"), 1, "POST",
                "/events/invoice-created", "", Map.of(), "", Map.of(), Caller.Platform.INSTANCE,
                Instant.parse("2099-01-01T00:00:00Z"), "", "10.0.0.1", false, body.getBytes(StandardCharsets.UTF_8));
    }
}
