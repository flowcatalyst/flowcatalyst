package io.flowcatalyst.sdk.resources;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpServer;
import io.flowcatalyst.sdk.FlowCatalystClient;
import io.flowcatalyst.sdk.error.FlowCatalystException;
import io.flowcatalyst.sdk.generated.model.PutFieldPolicyRequest;
import java.io.IOException;
import java.net.InetSocketAddress;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.List;
import java.util.function.Function;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

/**
 * Field-worker photos against a stub that keeps request bodies as bytes (the shared stub reads them as
 * text) and can answer bytes and a 304.
 */
class FieldPhotosTest {

    private static final String WORKER = "{\"id\":\"fwu_1\",\"clientId\":\"clt_1\",\"workerCode\":\"W001\","
            + "\"name\":\"Ann\",\"status\":\"ACTIVE\",\"apps\":[],\"photo\":{\"version\":\"0123456789abcdef\","
            + "\"updatedAt\":\"2026-11-20T10:00:00Z\"}}";
    private static final byte[] JPEG = {(byte) 0xFF, (byte) 0xD8, (byte) 0xFF, (byte) 0xE0, 0, 0x10, 'J', 'F', 'I', 'F', 0,
        (byte) 0xFF, (byte) 0xD9};

    record Seen(String method, String path, String query, String contentType, String accept, String ifNoneMatch, byte[] body) {}

    record Answer(int status, String contentType, String etag, byte[] body) {}

    private HttpServer server;
    private final List<Seen> seen = new CopyOnWriteArrayList<>();
    private Function<Seen, Answer> answer = s -> new Answer(200, "application/json", null, WORKER.getBytes(StandardCharsets.UTF_8));

    @BeforeEach
    void setUp() throws IOException {
        server = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
        server.createContext("/", this::handle);
        server.start();
    }

    @AfterEach
    void tearDown() {
        server.stop(0);
    }

    private void handle(HttpExchange ex) throws IOException {
        byte[] body = ex.getRequestBody().readAllBytes();
        if (ex.getRequestURI().getPath().equals("/oauth/token")) {
            reply(ex, new Answer(200, "application/json", null,
                    "{\"access_token\":\"tok\",\"token_type\":\"Bearer\",\"expires_in\":3600}".getBytes(StandardCharsets.UTF_8)));
            return;
        }
        Seen s = new Seen(ex.getRequestMethod(), ex.getRequestURI().getPath(), ex.getRequestURI().getRawQuery(),
                ex.getRequestHeaders().getFirst("Content-Type"), ex.getRequestHeaders().getFirst("Accept"),
                ex.getRequestHeaders().getFirst("If-None-Match"), body);
        seen.add(s);
        reply(ex, answer.apply(s));
    }

    private static void reply(HttpExchange ex, Answer a) throws IOException {
        if (a.contentType() != null) ex.getResponseHeaders().set("Content-Type", a.contentType());
        if (a.etag() != null) ex.getResponseHeaders().set("ETag", a.etag());
        if (a.status() == 304) {
            ex.sendResponseHeaders(304, -1);
        } else {
            ex.sendResponseHeaders(a.status(), a.body().length == 0 ? -1 : a.body().length);
            if (a.body().length > 0) ex.getResponseBody().write(a.body());
        }
        ex.close();
    }

    private FlowCatalystClient client() {
        return FlowCatalystClient.builder()
                .baseUrl("http://127.0.0.1:" + server.getAddress().getPort())
                .clientCredentials("id", "secret")
                .retryDelay(Duration.ofMillis(1))
                .build();
    }

    private Seen last() {
        return seen.getLast();
    }

    @Test
    void setPhotoSendsTheBytesAsTheyAre() {
        var w = client().fieldWorkers().setPhoto("fwu_1", JPEG, "image/jpeg");
        assertThat(last().method()).isEqualTo("PUT");
        assertThat(last().path()).isEqualTo("/api/field-workers/fwu_1/photo");
        assertThat(last().body()).isEqualTo(JPEG);
        assertThat(last().contentType()).isEqualTo("image/jpeg");
        assertThat(w.getPhoto().getVersion()).isEqualTo("0123456789abcdef");

        client().fieldWorkers().setPhoto("fwu_1", JPEG, null);
        assertThat(last().contentType()).isEqualTo("application/octet-stream");
        assertThat(last().body()).isEqualTo(JPEG);
        // Bytes that are not text survive: 0xFF is not valid UTF-8.
        assertThat(new String(JPEG, StandardCharsets.UTF_8).getBytes(StandardCharsets.UTF_8)).isNotEqualTo(JPEG);
    }

    @Test
    void setPhotoRefusalCarriesTheCode() {
        answer = s -> new Answer(415, "application/json", null,
                "{\"error\":\"PHOTO_UNSUPPORTED_TYPE\",\"message\":\"send a JPEG\"}".getBytes(StandardCharsets.UTF_8));
        assertThatThrownBy(() -> client().fieldWorkers().setPhoto("fwu_1", JPEG, "image/gif"))
                .isInstanceOf(FlowCatalystException.class)
                .satisfies(e -> assertThat(((FlowCatalystException) e).error().toString()).contains("PHOTO_UNSUPPORTED_TYPE", "415"));
        assertThat(seen).hasSize(1);
    }

    @Test
    void getPhotoReturnsBytesAndTheValidator() {
        answer = s -> new Answer(200, "image/jpeg", "\"0123456789abcdef-thumb\"", JPEG);
        var p = client().fieldWorkers().getPhoto("fwu_1", null, null);
        assertThat(last().method()).isEqualTo("GET");
        assertThat(last().path()).isEqualTo("/api/field-workers/fwu_1/photo");
        assertThat(last().query()).isNull();
        assertThat(last().accept()).contains("image/jpeg");
        assertThat(last().ifNoneMatch()).isNull();
        assertThat(p.bytes()).isEqualTo(JPEG);
        assertThat(p.contentType()).isEqualTo("image/jpeg");
        assertThat(p.etag()).isEqualTo("\"0123456789abcdef-thumb\"");
        assertThat(p.notModified()).isFalse();
        client().fieldWorkers().getPhoto("fwu_1", FieldWorkersResource.FULL, null);
        assertThat(last().query()).isEqualTo("size=full");
    }

    @Test
    void getPhotoNotModifiedIsASuccess() {
        answer = s -> new Answer(304, null, "\"0123456789abcdef-thumb\"", new byte[0]);
        var p = client().fieldWorkers().getPhoto("fwu_1", FieldWorkersResource.THUMB, "\"0123456789abcdef-thumb\"");
        assertThat(last().ifNoneMatch()).isEqualTo("\"0123456789abcdef-thumb\"");
        assertThat(p.notModified()).isTrue();
        assertThat(p.bytes()).isNull();
        assertThat(p.etag()).isEqualTo("\"0123456789abcdef-thumb\"");
    }

    @Test
    void getPhotoOfAWorkerWithNoneIsNotFound() {
        answer = s -> new Answer(404, "application/json", null,
                "{\"error\":\"PHOTO_NOT_FOUND\",\"message\":\"this worker has no photo\"}".getBytes(StandardCharsets.UTF_8));
        assertThatThrownBy(() -> client().fieldWorkers().getPhoto("fwu_1", null, null))
                .isInstanceOf(FlowCatalystException.class)
                .hasMessageContaining("no photo");
    }

    @Test
    void removePhotoAndDeleteAllPhotos() {
        client().fieldWorkers().removePhoto("fwu_1");
        assertThat(last().method()).isEqualTo("DELETE");
        assertThat(last().path()).isEqualTo("/api/field-workers/fwu_1/photo");
        answer = s -> new Answer(200, "application/json", null, "{\"removed\":3}".getBytes(StandardCharsets.UTF_8));
        var r = client().fieldPolicies().deletePhotos("clt_1");
        assertThat(last().method()).isEqualTo("DELETE");
        assertThat(last().path()).isEqualTo("/api/field-policies/clt_1/photos");
        assertThat(r.getRemoved()).isEqualTo(3);
    }

    @Test
    void thePolicyCarriesTheTwoPhotoSettings() {
        answer = s -> new Answer(200, "application/json", null,
                "{\"clientId\":\"clt_1\",\"photosEnabled\":true,\"workerMaySetOwnPhoto\":false}".getBytes(StandardCharsets.UTF_8));
        var p = client().fieldPolicies().get("clt_1");
        assertThat(p.getPhotosEnabled()).isTrue();
        assertThat(p.getWorkerMaySetOwnPhoto()).isFalse();
        client().fieldPolicies().put("clt_1", new PutFieldPolicyRequest().requireAttestation(false).photosEnabled(false).workerMaySetOwnPhoto(true));
        assertThat(new String(last().body(), StandardCharsets.UTF_8)).contains("\"photosEnabled\":false", "\"workerMaySetOwnPhoto\":true");
        client().fieldPolicies().put("clt_1", new PutFieldPolicyRequest().requireAttestation(false));
        assertThat(new String(last().body(), StandardCharsets.UTF_8)).doesNotContain("photosEnabled").doesNotContain("workerMaySetOwnPhoto");
    }
}
