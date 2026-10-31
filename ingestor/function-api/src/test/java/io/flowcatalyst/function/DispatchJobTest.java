package io.flowcatalyst.function;

import java.nio.charset.StandardCharsets;
import java.util.HashMap;
import java.util.Map;

import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

/// `DispatchJob`'s input shape mirrors `clients/fn-ts/src/job.ts` `DispatchInput`:
/// target, path, payload, contentType, code, subject, metadata (sorted by key),
/// messageGroup, mode, correlationId, causationId, descriptor, maxRetries,
/// timeoutSeconds, dataOnly. `DispatchException` mirrors `FnError`.
class DispatchJobTest {

    @Test
    void onlyTargetAndPathAreRequired() {
        DispatchJob job = DispatchJob.to("orders.fulfil", "/fulfil").build();
        assertThat(job.target()).isEqualTo("orders.fulfil");
        assertThat(job.path()).isEqualTo("/fulfil");
        assertThat(job.payload()).isNull();
        assertThat(job.contentType()).isNull();
        assertThat(job.metadata()).isEmpty();
        assertThat(job.maxRetries()).isNull();
        assertThat(job.timeoutSeconds()).isNull();
        assertThat(job.dataOnly()).isNull();
        assertThatThrownBy(() -> DispatchJob.to(null, "/x").build()).isInstanceOf(NullPointerException.class);
        assertThatThrownBy(() -> DispatchJob.to("a.b", null).build()).isInstanceOf(NullPointerException.class);
    }

    @Test
    void everyFieldIsCarried() {
        DispatchJob job = DispatchJob.to("orders.fulfil", "/fulfil")
                .payload("hello").contentType("text/plain").code("orders:function:fulfil:dispatch")
                .subject("order-1").metadata("b", "2").metadata("a", "1").messageGroup("g")
                .mode("BLOCK_ON_ERROR").correlationId("c1").causationId("e1").descriptor("Fulfil order 1")
                .maxRetries(3).timeoutSeconds(30).dataOnly(false).build();
        assertThat(new String(job.payload(), StandardCharsets.UTF_8)).isEqualTo("hello");
        assertThat(job.contentType()).isEqualTo("text/plain");
        assertThat(job.code()).isEqualTo("orders:function:fulfil:dispatch");
        assertThat(job.subject()).isEqualTo("order-1");
        assertThat(job.messageGroup()).isEqualTo("g");
        assertThat(job.mode()).isEqualTo("BLOCK_ON_ERROR");
        assertThat(job.correlationId()).isEqualTo("c1");
        assertThat(job.causationId()).isEqualTo("e1");
        assertThat(job.descriptor()).isEqualTo("Fulfil order 1");
        assertThat(job.maxRetries()).isEqualTo(3);
        assertThat(job.timeoutSeconds()).isEqualTo(30);
        assertThat(job.dataOnly()).isFalse();
    }

    @Test
    void metadataIsSortedByKeyAndUnmodifiable() {
        DispatchJob job = DispatchJob.to("a.b", "/x").metadata("zeta", "1").metadata("alpha", "2").build();
        assertThat(job.metadata().keySet()).containsExactly("alpha", "zeta");
        assertThatThrownBy(() -> job.metadata().put("k", "v")).isInstanceOf(UnsupportedOperationException.class);
        Map<String, String> source = new HashMap<>(Map.of("k", "v"));
        DispatchJob viaConstructor = new DispatchJob("a.b", "/x", null, null, null, null, source, null, null, null,
                null, null, null, null, null);
        source.put("other", "x");
        assertThat(viaConstructor.metadata()).containsOnlyKeys("k");
    }

    @Test
    void payloadJsonSetsTheContentTypeUnlessOneWasSetExplicitly() {
        assertThat(DispatchJob.to("a.b", "/x").payloadJson("{\"a\":1}").build().contentType())
                .isEqualTo("application/json");
        assertThat(DispatchJob.to("a.b", "/x").contentType("application/vnd.x+json").payloadJson("{}").build()
                .contentType()).isEqualTo("application/vnd.x+json");
    }

    @Test
    void aStringPayloadSetsNoContentType() {
        // fn-ts: a string payload is UTF-8 encoded and the content type left alone.
        assertThat(DispatchJob.to("a.b", "/x").payload("plain").build().contentType()).isNull();
    }

    @Test
    void payloadIsDefensivelyCopiedBothWays() {
        byte[] bytes = {1, 2, 3};
        DispatchJob job = DispatchJob.to("a.b", "/x").payload(bytes).build();
        bytes[0] = 99;
        assertThat(job.payload()).containsExactly(1, 2, 3);
        job.payload()[0] = 42;
        assertThat(job.payload()).containsExactly(1, 2, 3);
    }

    @Test
    void equalityIsByContentAndToStringReportsPayloadLengthOnly() {
        DispatchJob a = DispatchJob.to("a.b", "/x").payload(new byte[] {1, 2}).build();
        DispatchJob b = DispatchJob.to("a.b", "/x").payload(new byte[] {1, 2}).build();
        DispatchJob c = DispatchJob.to("a.b", "/x").payload(new byte[] {1, 3}).build();
        assertThat(a).isEqualTo(b).hasSameHashCodeAs(b).isNotEqualTo(c);
        assertThat(a.toString()).contains("payload.length=2").doesNotContain("[1, 2]");
    }

    @Test
    void onlyUnavailableIsRetryable() {
        assertThat(new DispatchException("UNAVAILABLE", "down").retryable()).isTrue();
        for (String code : new String[] {"NOT_DECLARED", "NOT_ALLOWED", "BAD_REQUEST", "DEADLINE", "TOO_LARGE"}) {
            assertThat(new DispatchException(code, "x").retryable()).as(code).isFalse();
        }
    }

    @Test
    void exceptionMessageFollowsFnError() {
        assertThat(new DispatchException("NOT_DECLARED", "no such target").getMessage())
                .isEqualTo("fn: NOT_DECLARED: no such target");
        assertThat(new DispatchException("UNAVAILABLE", "").getMessage()).isEqualTo("fn: UNAVAILABLE");
        assertThat(new DispatchException("BAD_REQUEST", "x").code()).isEqualTo("BAD_REQUEST");
    }
}
