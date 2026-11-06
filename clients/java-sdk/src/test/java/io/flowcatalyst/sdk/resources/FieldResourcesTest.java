package io.flowcatalyst.sdk.resources;

import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.sdk.FlowCatalystClient;
import io.flowcatalyst.sdk.StubServer;
import io.flowcatalyst.sdk.generated.model.CreateEnrolmentCodeRequest;
import io.flowcatalyst.sdk.generated.model.EnsureFieldWorkerRequest;
import io.flowcatalyst.sdk.generated.model.PutFieldPolicyRequest;
import io.flowcatalyst.sdk.generated.model.ResetFieldWorkerPinRequest;
import io.flowcatalyst.sdk.generated.model.UpdateFieldWorkerRequest;
import java.time.Duration;
import java.util.List;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

/** Wire behaviour of the field-identity admin resources, against a local stub. */
class FieldResourcesTest {

    private static final String WORKER = "{\"id\":\"fwu_1\",\"clientId\":\"clt_1\",\"workerCode\":\"W001\","
            + "\"name\":\"Ann\",\"status\":\"ACTIVE\",\"hasPin\":true,\"pinChangeRequired\":false,"
            + "\"locked\":false,\"hardLocked\":false,\"failedSinceSuccess\":0,\"apps\":"
            + "[{\"id\":\"app_1\",\"code\":\"orders\",\"name\":\"Orders\",\"source\":\"API\","
            + "\"grantedAt\":\"2026-01-01T00:00:00Z\"}],"
            + "\"createdAt\":\"2026-01-01T00:00:00Z\",\"updatedAt\":\"2026-01-01T00:00:00Z\",\"created\":true}";

    private StubServer server;

    @BeforeEach
    void setUp() throws Exception {
        server = new StubServer();
        server.stubToken("tok");
    }

    @AfterEach
    void tearDown() {
        server.close();
    }

    private FlowCatalystClient client() {
        return FlowCatalystClient.builder()
                .baseUrl(server.baseUrl())
                .clientCredentials("id", "secret")
                .retryDelay(Duration.ofMillis(1))
                .build();
    }

    private StubServer.Recorded last() {
        return server.requests.getLast();
    }

    @Test
    void ensureWorkerPostsTheWireNamesAndReadsTheResult() {
        server.on("POST", "/api/field-workers", 201, WORKER);
        var w = client().fieldWorkers().ensure(new EnsureFieldWorkerRequest()
                .clientId("clt_1").workerCode("W001").name("Ann").appCodes(List.of("orders")).pin("482913"));
        assertThat(last().authorization()).isEqualTo("Bearer tok");
        assertThat(last().body()).contains("\"clientId\":\"clt_1\"", "\"workerCode\":\"W001\"",
                "\"appCodes\":[\"orders\"]", "\"pin\":\"482913\"").doesNotContain("email");
        assertThat(w.getCreated()).isTrue();
        assertThat(w.getHasPin()).isTrue();
        assertThat(w.getApps()).hasSize(1);
        assertThat(w.getApps().getFirst().getCode()).isEqualTo("orders");
    }

    @Test
    void listWorkersSendsOnlyTheFiltersGiven() {
        server.on("GET", "/api/field-workers", 200, "{\"workers\":[" + WORKER + "],\"total\":1,\"page\":0,\"size\":100}");
        var r = client().fieldWorkers().list("clt_1", new FieldWorkersResource.Filters("an", null, "ACTIVE", 0, null));
        assertThat(last().pathAndQuery()).isEqualTo("/api/field-workers?clientId=clt_1&q=an&status=ACTIVE&page=0");
        assertThat(r.getTotal()).isEqualTo(1);
        assertThat(r.getWorkers()).hasSize(1);
        client().fieldWorkers().list("clt_1", null);
        assertThat(last().pathAndQuery()).isEqualTo("/api/field-workers?clientId=clt_1");
    }

    @Test
    void badgesSignInCodesAndIdentityFlags() {
        String badge = "{\"id\":\"fbg_1\",\"workerId\":\"fwu_1\",\"status\":\"ACTIVE\",\"createdAt\":\"2026-01-01T00:00:00Z\"}";
        server.on("POST", "/api/field-workers/fwu_1/badges", 201, "{\"badge\":" + badge + ",\"token\":\"secret\"}");
        var issued = client().fieldWorkers().issueBadge("fwu_1", "Depot A");
        assertThat(last().body()).contains("\"label\":\"Depot A\"");
        assertThat(issued.getToken()).isEqualTo("secret");
        assertThat(issued.getBadge().getId()).isEqualTo("fbg_1");

        server.on("GET", "/api/field-workers/fwu_1/badges", 200, "{\"badges\":[" + badge + "]}");
        assertThat(client().fieldWorkers().listBadges("fwu_1").getBadges()).hasSize(1);

        server.on("POST", "/api/field-workers/fwu_1/badges/fbg_1/revoke", 200, badge);
        client().fieldWorkers().revokeBadge("fwu_1", "fbg_1");
        assertThat(last().pathAndQuery()).isEqualTo("/api/field-workers/fwu_1/badges/fbg_1/revoke");

        server.on("POST", "/api/field-workers/badges", 201,
                "{\"issued\":[{\"workerId\":\"fwu_1\",\"workerCode\":\"W001\",\"name\":\"Ann\",\"badge\":" + badge + ",\"token\":\"t1\"}]}");
        var bulk = client().fieldWorkers().bulkIssueBadges(List.of("fwu_1"));
        assertThat(last().body()).contains("\"workerIds\":[\"fwu_1\"]");
        assertThat(bulk.getIssued().getFirst().getToken()).isEqualTo("t1");

        server.on("POST", "/api/field-workers/fwu_1/sign-in-codes", 201,
                "{\"code\":\"ABC\",\"workerId\":\"fwu_1\",\"appCode\":\"orders\",\"expiresAt\":\"2026-01-01T00:05:00Z\"}");
        var code = client().fieldWorkers().createSignInCode("fwu_1", "orders");
        assertThat(last().body()).contains("\"appCode\":\"orders\"");
        assertThat(code.getCode()).isEqualTo("ABC");

        server.on("POST", "/api/field-workers", 201, WORKER);
        client().fieldWorkers().ensure(new EnsureFieldWorkerRequest()
                .clientId("clt_1").workerCode("W001").name("Ann").isTest(true).canImpersonate(true));
        assertThat(last().body()).contains("\"isTest\":true", "\"canImpersonate\":true");

        server.on("GET", "/api/field-workers", 200, "{\"workers\":[],\"total\":0,\"page\":0,\"size\":100}");
        client().fieldWorkers().list("clt_1", new FieldWorkersResource.Filters(null, null, null, true, null, null));
        assertThat(last().pathAndQuery()).isEqualTo("/api/field-workers?clientId=clt_1&isTest=true");
    }

    @Test
    void linkUnlinkAndFilter() {
        String linked = WORKER.substring(0, WORKER.length() - 1)
                + ",\"linkedPrincipal\":{\"id\":\"prn_1\",\"name\":\"Pat\",\"email\":\"pat@x.test\"}}";
        server.on("PUT", "/api/field-workers/fwu_1/link", 200, linked);
        var w = client().fieldWorkers().link("fwu_1", "prn_1");
        assertThat(last().method()).isEqualTo("PUT");
        assertThat(last().body()).contains("\"principalId\":\"prn_1\"");
        assertThat(w.getLinkedPrincipal().getId()).isEqualTo("prn_1");

        server.on("DELETE", "/api/field-workers/fwu_1/link", 200, WORKER);
        client().fieldWorkers().unlink("fwu_1");
        assertThat(last().method()).isEqualTo("DELETE");
        assertThat(last().pathAndQuery()).isEqualTo("/api/field-workers/fwu_1/link");

        server.on("GET", "/api/field-workers", 200, "{\"workers\":[],\"total\":0,\"page\":0,\"size\":100}");
        client().fieldWorkers().list("clt_1", new FieldWorkersResource.Filters(null, null, null, null, null, null, "prn_1"));
        assertThat(last().pathAndQuery()).isEqualTo("/api/field-workers?clientId=clt_1&linkedPrincipalId=prn_1");
    }

    @Test
    void getUpdateDeleteHitTheByIdRoutes() {
        server.on("GET", "/api/field-workers/fwu_1", 200, WORKER);
        server.on("PUT", "/api/field-workers/fwu_1", 200, WORKER);
        server.on("DELETE", "/api/field-workers/fwu_1", 200, "{\"message\":\"deleted\"}");
        assertThat(client().fieldWorkers().get("fwu_1").getWorkerCode()).isEqualTo("W001");
        client().fieldWorkers().update("fwu_1", new UpdateFieldWorkerRequest().name("Ann B").email("a@x.test"));
        assertThat(last().method()).isEqualTo("PUT");
        assertThat(last().body()).contains("\"name\":\"Ann B\"", "\"email\":\"a@x.test\"");
        client().fieldWorkers().delete("fwu_1");
        assertThat(last().method()).isEqualTo("DELETE");
        assertThat(last().pathAndQuery()).isEqualTo("/api/field-workers/fwu_1");
    }

    @Test
    void resetPinReturnsTheTemporaryPinAndAcceptsANullRequest() {
        server.on("POST", "/api/field-workers/fwu_1/pin-reset", 200, "{\"temporaryPin\":\"739104\",\"pinChangeRequired\":true}");
        var r = client().fieldWorkers().resetPin("fwu_1", null);
        assertThat(r.getTemporaryPin()).isEqualTo("739104");
        assertThat(r.getPinChangeRequired()).isTrue();
        assertThat(last().body()).isEqualTo("{}");
        client().fieldWorkers().resetPin("fwu_1", new ResetFieldWorkerPinRequest().pin("246810"));
        assertThat(last().body()).contains("\"pin\":\"246810\"");
    }

    @Test
    void stateTransitionsAndAppGrantsUseTheirRoutes() {
        for (String p : List.of("unlock", "suspend", "activate")) {
            server.on("POST", "/api/field-workers/fwu_1/" + p, 204, "");
        }
        server.on("POST", "/api/field-workers/fwu_1/apps", 200, "{}");
        server.on("DELETE", "/api/field-workers/fwu_1/apps/orders", 200, "{}");
        var w = client().fieldWorkers();
        w.unlock("fwu_1");
        w.suspend("fwu_1");
        w.activate("fwu_1");
        w.grantApp("fwu_1", "orders");
        assertThat(last().body()).isEqualTo("{\"appCode\":\"orders\"}");
        w.revokeApp("fwu_1", "orders");
        assertThat(server.requests.stream().filter(r -> !r.pathAndQuery().equals("/oauth/token"))
                .map(r -> r.method() + " " + r.pathAndQuery()))
                .containsExactly("POST /api/field-workers/fwu_1/unlock", "POST /api/field-workers/fwu_1/suspend",
                        "POST /api/field-workers/fwu_1/activate", "POST /api/field-workers/fwu_1/apps",
                        "DELETE /api/field-workers/fwu_1/apps/orders");
    }

    @Test
    void signInsReadsNewestFirstPage() {
        server.on("GET", "/api/field-workers/fwu_1/sign-ins", 200,
                "{\"signIns\":[{\"id\":\"s1\",\"outcome\":\"FAILURE\",\"failureReason\":\"BAD_PIN\","
                        + "\"authMethod\":\"PIN\",\"attemptedAt\":\"2026-01-01T00:00:00Z\"}],\"total\":1,\"page\":2,\"size\":10}");
        var r = client().fieldWorkers().signIns("fwu_1", 2, 10);
        assertThat(last().pathAndQuery()).isEqualTo("/api/field-workers/fwu_1/sign-ins?page=2&pageSize=10");
        assertThat(r.getSignIns().getFirst().getFailureReason()).isEqualTo("BAD_PIN");
    }

    @Test
    void devicesEnrolListGetAndLifecycle() {
        server.on("POST", "/api/field-devices/enrolment-codes", 201,
                "{\"code\":\"7K3MQ9XA2B\",\"qrPayload\":\"fcfield:1:https://fc.test:7K3MQ9XA2B\",\"expiresAt\":\"2026-01-01T00:10:00Z\"}");
        var code = client().fieldDevices().createEnrolmentCode(new CreateEnrolmentCodeRequest()
                .clientId("clt_1").mode(CreateEnrolmentCodeRequest.ModeEnum.SHARED).appCodes(List.of("orders")));
        assertThat(last().body()).contains("\"mode\":\"SHARED\"", "\"appCodes\":[\"orders\"]").doesNotContain("assignedWorkerId");
        assertThat(code.getCode()).isEqualTo("7K3MQ9XA2B");

        String device = "{\"id\":\"fwd_1\",\"clientId\":\"clt_1\",\"mode\":\"PERSONAL\",\"jkt\":\"abc\","
                + "\"attestationLevel\":\"SOFTWARE\",\"status\":\"ACTIVE\",\"apps\":[],\"activeSessions\":2,"
                + "\"enrolledAt\":\"2026-01-01T00:00:00Z\"}";
        server.on("GET", "/api/field-devices", 200, "{\"devices\":[" + device + "],\"total\":1,\"page\":0,\"size\":100}");
        server.on("GET", "/api/field-devices/fwd_1", 200, device);
        for (String p : List.of("suspend", "activate", "revoke")) {
            server.on("POST", "/api/field-devices/fwd_1/" + p, 204, "");
        }
        var devices = client().fieldDevices();
        assertThat(devices.list("clt_1", new FieldDevicesResource.Filters("ACTIVE", null, null, null)).getDevices()).hasSize(1);
        assertThat(last().pathAndQuery()).isEqualTo("/api/field-devices?clientId=clt_1&status=ACTIVE");
        assertThat(devices.get("fwd_1").getActiveSessions()).isEqualTo(2);
        devices.suspend("fwd_1");
        devices.activate("fwd_1");
        devices.revoke("fwd_1");
        assertThat(last().pathAndQuery()).isEqualTo("/api/field-devices/fwd_1/revoke");
    }

    @Test
    void policyGetAndPut() {
        String policy = "{\"clientId\":\"clt_1\",\"pinMinLength\":8,\"softLockThreshold\":5,\"softLockSecs\":900,"
                + "\"hardLockThreshold\":10,\"sharedIdleSecs\":900,\"personalIdleSecs\":7200,\"shiftMaxSecs\":50400,"
                + "\"requireAttestation\":false,\"pinReauthDays\":7,\"isDefault\":false}";
        server.on("GET", "/api/field-policies/clt_1", 200, policy);
        server.on("PUT", "/api/field-policies/clt_1", 200, policy);
        assertThat(client().fieldPolicies().get("clt_1").getIsDefault()).isFalse();
        var out = client().fieldPolicies().put("clt_1", new PutFieldPolicyRequest().pinMinLength(8L).requireAttestation(false));
        assertThat(last().body()).contains("\"pinMinLength\":8", "\"requireAttestation\":false").doesNotContain("softLockSecs");
        assertThat(out.getPinMinLength()).isEqualTo(8);
    }

    @Test
    void introspectFieldToken() {
        server.on("POST", "/api/field-tokens/introspect", 200,
                "{\"active\":true,\"sub\":\"fwu_1\",\"workerCode\":\"W001\",\"amr\":[\"pin\"]}");
        var r = client().introspectFieldToken("the.token.here");
        assertThat(last().body()).isEqualTo("{\"token\":\"the.token.here\"}");
        assertThat(r.getActive()).isTrue();
        assertThat(r.getWorkerCode()).isEqualTo("W001");
        server.on("POST", "/api/field-tokens/introspect", 200, "{\"active\":false}");
    }
}
