package io.flowcatalyst.sdk.resources;

import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.sdk.FlowCatalystClient;
import io.flowcatalyst.sdk.StubServer;
import io.flowcatalyst.sdk.generated.model.PutFieldPolicyRequest;
import java.time.Duration;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

/** Unlock keys: the two admin calls, the device's key, the policy's settings and the get-then-put round trip. */
class FieldUnlockKeysTest {

    private static final String KEY = "{\"id\":\"fuk_1\",\"workerId\":\"fwu_1\",\"deviceId\":\"fdv_1\",\"status\":\"ACTIVE\","
            + "\"createdAt\":\"2026-01-01T00:00:00Z\",\"pinVerifiedAt\":\"2026-01-01T00:00:00Z\",\"liveSessions\":2,"
            + "\"attestation\":{\"format\":\"ANDROID_KEY\",\"verdict\":\"TRUSTED\",\"securityLevel\":\"STRONGBOX\","
            + "\"challengeMatches\":true,\"authPerUse\":true,\"biometricOnly\":true,\"signP256\":true,"
            + "\"meetsRequirement\":true}}";

    private static final String POLICY = "{\"clientId\":\"clt_1\",\"pinMinLength\":8,\"softLockThreshold\":4,"
            + "\"softLockSecs\":600,\"hardLockThreshold\":12,\"sharedIdleSecs\":300,\"personalIdleSecs\":3600,"
            + "\"shiftMaxSecs\":43200,\"requireAttestation\":true,\"pinReauthDays\":0,\"requireEnrolledDevice\":true,"
            + "\"unenrolledIdleSecs\":120,\"badgeOnlyLogin\":true,\"impersonationMaxSecs\":1800,\"workerCodeDigits\":5,"
            + "\"photosEnabled\":false,\"workerMaySetOwnPhoto\":true,\"unlockKeysEnabled\":true,"
            + "\"unlockKeyAttestation\":\"REQUIRED\",\"isDefault\":false}";

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
    void listsAndRevokesUnlockKeys() {
        server.on("GET", "/api/field-workers/fwu_1/unlock-keys", 200, "{\"unlockKeys\":[" + KEY + "]}");
        var list = client().fieldWorkers().listUnlockKeys("fwu_1");
        assertThat(last().pathAndQuery()).isEqualTo("/api/field-workers/fwu_1/unlock-keys");
        var key = list.getUnlockKeys().getFirst();
        assertThat(key.getId()).isEqualTo("fuk_1");
        assertThat(key.getLiveSessions()).isEqualTo(2);
        assertThat(key.getAttestation().getVerdict().getValue()).isEqualTo("TRUSTED");
        assertThat(key.getAttestation().getMeetsRequirement()).isTrue();

        server.on("POST", "/api/field-workers/fwu_1/unlock-keys/fuk_1/revoke", 200,
                KEY.replace("\"ACTIVE\"", "\"REVOKED\"").replace("\"liveSessions\":2", "\"liveSessions\":0,\"revokedReason\":\"ADMIN\""));
        var revoked = client().fieldWorkers().revokeUnlockKey("fwu_1", "fuk_1");
        assertThat(last().method()).isEqualTo("POST");
        assertThat(last().pathAndQuery()).isEqualTo("/api/field-workers/fwu_1/unlock-keys/fuk_1/revoke");
        assertThat(revoked.getStatus().getValue()).isEqualTo("REVOKED");
        assertThat(revoked.getRevokedReason().getValue()).isEqualTo("ADMIN");
        assertThat(revoked.getLiveSessions()).isZero();
    }

    @Test
    void aDeviceCarriesItsUnlockKeyOrNull() {
        String device = "{\"id\":\"fdv_1\",\"clientId\":\"clt_1\",\"mode\":\"PERSONAL\",\"jkt\":\"j\",\"attestationLevel\":\"TEE\","
                + "\"status\":\"ACTIVE\",\"apps\":[],\"activeSessions\":0,\"enrolledAt\":\"2026-01-01T00:00:00Z\",\"unlockKey\":";
        server.on("GET", "/api/field-devices/fdv_1", 200, device + KEY + "}");
        assertThat(client().fieldDevices().get("fdv_1").getUnlockKey().getId()).isEqualTo("fuk_1");
        server.on("GET", "/api/field-devices/fdv_2", 200, device.replace("fdv_1", "fdv_2") + "null}");
        assertThat(client().fieldDevices().get("fdv_2").getUnlockKey()).isNull();
    }

    @Test
    void toRequestCarriesEverySetting() {
        server.on("GET", "/api/field-policies/clt_1", 200, POLICY);
        server.on("PUT", "/api/field-policies/clt_1", 200, POLICY);
        var policy = client().fieldPolicies().get("clt_1");
        client().fieldPolicies().put("clt_1", FieldPoliciesResource.toRequest(policy));
        assertThat(last().body()).contains("\"unlockKeysEnabled\":true", "\"unlockKeyAttestation\":\"REQUIRED\"",
                "\"pinReauthDays\":0", "\"workerMaySetOwnPhoto\":true", "\"photosEnabled\":false",
                "\"requireEnrolledDevice\":true", "\"pinMinLength\":8", "\"workerCodeDigits\":5")
                .doesNotContain("clientId").doesNotContain("isDefault");
    }

    @Test
    void updateReadsChangesOneSettingAndKeepsUnlockKeysOn() {
        server.on("GET", "/api/field-policies/clt_1", 200, POLICY);
        server.on("PUT", "/api/field-policies/clt_1", 200, POLICY);
        client().fieldPolicies().update("clt_1", r -> r.pinMinLength(10L));
        assertThat(server.requests.stream().map(StubServer.Recorded::method).toList().reversed().subList(0, 2))
                .containsExactly("PUT", "GET");
        assertThat(last().body()).contains("\"pinMinLength\":10", "\"unlockKeysEnabled\":true",
                "\"unlockKeyAttestation\":\"REQUIRED\"", "\"pinReauthDays\":0");
    }

    @Test
    void aBarePutDoesNotSendTheUnlockKeySettings() {
        server.on("PUT", "/api/field-policies/clt_1", 200, POLICY);
        client().fieldPolicies().put("clt_1", new PutFieldPolicyRequest().requireAttestation(true));
        assertThat(last().body()).doesNotContain("unlockKeysEnabled").doesNotContain("unlockKeyAttestation");
    }
}
