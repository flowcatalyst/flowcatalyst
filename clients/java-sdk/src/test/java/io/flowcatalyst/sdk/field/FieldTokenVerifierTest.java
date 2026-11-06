package io.flowcatalyst.sdk.field;

import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.sdk.http.Json;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.time.Clock;
import java.time.Duration;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.ArrayList;
import java.util.Base64;
import java.util.List;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.function.Function;
import org.junit.jupiter.api.DynamicTest;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.TestFactory;
import tools.jackson.databind.JsonNode;

/** Runs the shared field-token vectors (api/field-fixtures) and the verifier's own guards. */
class FieldTokenVerifierTest {

    /** Sits outside this module: a standalone checkout of the SDK does not have it. */
    private static final Path DIR = Path.of("..", "..", "api", "field-fixtures");

    private record Case(
            String name, String token, String proof, String method, String url, Instant now, String expect, JsonNode principal) {}

    private record Fixtures(String issuer, String appCode, String jwks, List<Case> cases) {}

    private static Fixtures load() throws IOException {
        Path cases = DIR.resolve("cases.json");
        if (!Files.exists(cases)) {
            return null;
        }
        JsonNode root = Json.newMapper().readTree(Files.readString(cases));
        List<Case> out = new ArrayList<>();
        for (JsonNode c : root.get("cases")) {
            out.add(new Case(
                    c.get("name").asString(),
                    c.get("accessToken").asString(),
                    c.get("dpopProof").asString(),
                    c.get("method").asString(),
                    c.get("url").asString(),
                    Instant.parse(c.get("now").asString()),
                    c.get("expect").asString(),
                    c.get("principal")));
        }
        return new Fixtures(
                root.get("issuer").asString(),
                root.get("appCode").asString(),
                Files.readString(DIR.resolve("jwks.json")),
                out);
    }

    private static FieldTokenVerifier verifier(Fixtures f, Instant now, Function<String, String> fetcher) {
        return FieldTokenVerifier.builder()
                .issuer(f.issuer())
                .appCode(f.appCode())
                .jwksUrl("https://keys.example.test/jwks")
                .fetcher(fetcher)
                .clock(Clock.fixed(now, ZoneOffset.UTC))
                .build();
    }

    private static Fixtures requireFixtures() throws IOException {
        Fixtures f = load();
        org.junit.jupiter.api.Assumptions.assumeTrue(
                f != null, "field fixtures not present at " + DIR.toAbsolutePath() + " (standalone SDK checkout)");
        return f;
    }

    @TestFactory
    List<DynamicTest> everyFixtureCase() throws IOException {
        Fixtures f = load();
        if (f == null) {
            return List.of(DynamicTest.dynamicTest(
                    "SKIPPED: field fixtures not present at " + DIR.toAbsolutePath() + " (standalone SDK checkout)",
                    () -> org.junit.jupiter.api.Assumptions.abort("field fixtures absent")));
        }
        assertThat(f.cases()).isNotEmpty();
        assertThat(f.cases().stream().filter(c -> c.principal() != null).map(Case::name))
                .contains("valid_unenrolled", "valid_impersonated_supervisor", "valid_impersonated_principal", "valid_test");
        List<DynamicTest> tests = new ArrayList<>();
        for (Case c : f.cases()) {
            tests.add(DynamicTest.dynamicTest(c.name(), () -> {
                FieldVerification r = verifier(f, c.now(), u -> f.jwks())
                        .verify("DPoP " + c.token(), c.proof(), c.method(), c.url());
                if (c.expect().equals("valid")) {
                    assertThat(r).isInstanceOf(FieldVerification.Valid.class);
                    FieldPrincipal p = ((FieldVerification.Valid) r).principal();
                    assertThat(p.workerId()).isEqualTo("fwu_0FIXTUREWORKER");
                    assertThat(p.clientIdentifier()).isEqualTo("fixture-client");
                    assertThat(p.workerCode()).isEqualTo("W001");
                    assertThat(p.appCode()).isEqualTo(f.appCode());
                    assertThat(p.jkt()).isNotBlank();
                    JsonNode want = c.principal();
                    if (want == null) {
                        assertThat(p.deviceId()).isEqualTo("fwd_0FIXTUREDEVICE");
                        assertThat(p.amr()).containsExactly("pin");
                        assertThat(p.actor()).isNull();
                        assertThat(p.test()).isFalse();
                    } else {
                        assertThat(p.deviceId()).isEqualTo(want.get("deviceId").asString());
                        assertThat(p.deviceMode()).isEqualTo(want.get("deviceMode").asString());
                        List<String> amr = new ArrayList<>();
                        want.get("amr").forEach(n -> amr.add(n.asString()));
                        assertThat(p.amr()).isEqualTo(amr);
                        assertThat(p.test()).isEqualTo(want.get("test").asBoolean());
                        JsonNode actor = want.get("actor");
                        if (actor == null) {
                            assertThat(p.actor()).isNull();
                        } else {
                            assertThat(p.actor())
                                    .isEqualTo(new FieldActor(
                                            actor.get("id").asString(),
                                            actor.get("type").asString(),
                                            actor.get("name").asString()));
                        }
                    }
                } else {
                    assertThat(r).isInstanceOf(FieldVerification.Invalid.class);
                    assertThat(((FieldVerification.Invalid) r).reason()).isEqualTo(c.expect());
                }
            }));
        }
        return tests;
    }

    @Test
    void bearerSchemeIsRefusedBecauseItWouldSkipTheProof() throws IOException {
        Fixtures f = requireFixtures();
        Case c = f.cases().get(0);
        FieldVerification r = verifier(f, c.now(), u -> f.jwks())
                .verify("Bearer " + c.token(), c.proof(), c.method(), c.url());
        assertThat(r).isEqualTo(new FieldVerification.Invalid(FieldReason.MALFORMED, "Authorization must be \"DPoP <token>\""));
    }

    @Test
    void missingOrGarbageAuthorizationIsMalformed() throws IOException {
        Fixtures f = requireFixtures();
        Case c = f.cases().get(0);
        FieldTokenVerifier v = verifier(f, c.now(), u -> f.jwks());
        for (String h : new String[] {null, "", "DPoP", "DPoP ", "DPoP not-a-jws", "DPoP a.b.c"}) {
            assertThat(v.verify(h, c.proof(), c.method(), c.url()))
                    .asInstanceOf(org.assertj.core.api.InstanceOfAssertFactories.type(FieldVerification.Invalid.class))
                    .extracting(FieldVerification.Invalid::reason)
                    .isEqualTo(FieldReason.MALFORMED);
        }
    }

    @Test
    void accessTokenAlgIsPinnedToRs256() throws IOException {
        Fixtures f = requireFixtures();
        Case c = f.cases().get(0);
        FieldTokenVerifier v = verifier(f, c.now(), u -> f.jwks());
        String[] parts = c.token().split("\\.");
        for (String alg : new String[] {"HS256", "none", "ES256", "RS512", "rs256"}) {
            String hdr = b64("{\"alg\":\"" + alg + "\",\"typ\":\"fc-field+jwt\",\"kid\":\"fixture-key\"}");
            FieldVerification r = v.verify("DPoP " + hdr + "." + parts[1] + "." + parts[2], c.proof(), c.method(), c.url());
            assertThat(r).as(alg).isInstanceOf(FieldVerification.Invalid.class);
            FieldVerification.Invalid inv = (FieldVerification.Invalid) r;
            assertThat(inv.reason()).as(alg).isEqualTo(FieldReason.INVALID_TOKEN);
            assertThat(inv.detail()).as(alg).contains("RS256");
        }
        // Even with an empty signature, as an "alg: none" attacker would send.
        String none = b64("{\"alg\":\"none\",\"typ\":\"fc-field+jwt\"}");
        assertThat(v.verify("DPoP " + none + "." + parts[1] + ".", c.proof(), c.method(), c.url()))
                .isInstanceOf(FieldVerification.Invalid.class);
    }

    @Test
    void proofAlgIsPinnedToEs256() throws IOException {
        Fixtures f = requireFixtures();
        Case c = f.cases().get(0);
        FieldTokenVerifier v = verifier(f, c.now(), u -> f.jwks());
        String[] p = c.proof().split("\\.");
        for (String alg : new String[] {"RS256", "HS256", "none"}) {
            JsonNode hdr = Json.newMapper().readTree(Base64.getUrlDecoder().decode(p[0]));
            String forged = b64(hdr.toString().replace("\"ES256\"", "\"" + alg + "\""));
            FieldVerification r = v.verify("DPoP " + c.token(), forged + "." + p[1] + "." + p[2], c.method(), c.url());
            assertThat(r).as(alg).isEqualTo(
                    new FieldVerification.Invalid(FieldReason.INVALID_PROOF, "typ must be dpop+jwt and alg ES256"));
        }
    }

    @Test
    void proofMaxAgeCanBeWidenedForBadClocks() throws IOException {
        Fixtures f = requireFixtures();
        Case stale = f.cases().stream().filter(c -> c.name().equals("stale_proof")).findFirst().orElseThrow();
        FieldVerification narrow = verifier(f, stale.now(), u -> f.jwks())
                .verify("DPoP " + stale.token(), stale.proof(), stale.method(), stale.url());
        assertThat(narrow).isInstanceOf(FieldVerification.Invalid.class);
        FieldTokenVerifier wide = FieldTokenVerifier.builder()
                .issuer(f.issuer())
                .appCode(f.appCode())
                .jwksUrl("https://keys.example.test/jwks")
                .fetcher(u -> f.jwks())
                .clock(Clock.fixed(stale.now(), ZoneOffset.UTC))
                .proofMaxAge(Duration.ofDays(2))
                .build();
        assertThat(wide.verify("DPoP " + stale.token(), stale.proof(), stale.method(), stale.url()))
                .isInstanceOf(FieldVerification.Valid.class);
    }

    @Test
    void htuComparesSchemeHostAndPathNotQueryAndNeverAHostHeader() {
        assertThat(FieldTokenVerifier.sameUrl("https://app.example.test/a/b", "HTTPS://APP.example.test/a/b?x=1")).isTrue();
        assertThat(FieldTokenVerifier.sameUrl("https://app.example.test/a/b", "http://app.example.test/a/b")).isFalse();
        assertThat(FieldTokenVerifier.sameUrl("https://app.example.test/a/b", "https://evil.example.test/a/b")).isFalse();
        assertThat(FieldTokenVerifier.sameUrl("https://app.example.test/a/b", "https://app.example.test:8443/a/b")).isFalse();
        assertThat(FieldTokenVerifier.sameUrl("https://app.example.test/a/b", "https://app.example.test/a/c")).isFalse();
        assertThat(FieldTokenVerifier.sameUrl("https://app.example.test/a", "not a url")).isFalse();
    }

    @Test
    void keysAreCachedAndAnUnknownKidRefetchesAtMostOncePerMinute() throws IOException {
        Fixtures f = requireFixtures();
        Case c = f.cases().get(0);
        AtomicInteger fetches = new AtomicInteger();
        MutableClock clock = new MutableClock(c.now());
        FieldTokenVerifier v = FieldTokenVerifier.builder()
                .issuer(f.issuer())
                .appCode(f.appCode())
                .jwksUrl("https://keys.example.test/jwks")
                .fetcher(u -> {
                    fetches.incrementAndGet();
                    return f.jwks();
                })
                .clock(clock)
                .build();

        assertThat(v.verify("DPoP " + c.token(), c.proof(), c.method(), c.url())).isInstanceOf(FieldVerification.Valid.class);
        assertThat(v.verify("DPoP " + c.token(), c.proof(), c.method(), c.url())).isInstanceOf(FieldVerification.Valid.class);
        assertThat(fetches).hasValue(1);

        // A token naming a kid the set does not hold (signature irrelevant: no key to try).
        String[] parts = c.token().split("\\.");
        String ghost = b64("{\"alg\":\"RS256\",\"typ\":\"fc-field+jwt\",\"kid\":\"rotated-key\"}")
                + "." + parts[1] + "." + parts[2];
        // Within a minute of the last fetch: no refetch.
        clock.advance(Duration.ofSeconds(30));
        assertThat(v.verify("DPoP " + ghost, c.proof(), c.method(), c.url())).isInstanceOf(FieldVerification.Invalid.class);
        assertThat(fetches).hasValue(1);
        // After a minute: exactly one refetch, however many bad tokens arrive.
        clock.advance(Duration.ofSeconds(31));
        v.verify("DPoP " + ghost, c.proof(), c.method(), c.url());
        v.verify("DPoP " + ghost, c.proof(), c.method(), c.url());
        assertThat(fetches).hasValue(2);
    }

    @Test
    void keysThatCannotBeFetchedAreUnavailableNotInvalid() throws IOException {
        Fixtures f = requireFixtures();
        Case c = f.cases().get(0);
        FieldTokenVerifier v = verifier(f, c.now(), u -> {
            throw new IllegalStateException("connection refused");
        });
        assertThat(v.verify("DPoP " + c.token(), c.proof(), c.method(), c.url()))
                .isInstanceOf(FieldVerification.Unavailable.class);
    }

    @Test
    void discoveryFindsTheJwksUri() throws IOException {
        Fixtures f = requireFixtures();
        Case c = f.cases().get(0);
        List<String> urls = new ArrayList<>();
        FieldTokenVerifier v = FieldTokenVerifier.builder()
                .issuer(f.issuer() + "/")
                .appCode(f.appCode())
                .fetcher(u -> {
                    urls.add(u);
                    return u.endsWith("openid-configuration") ? "{\"jwks_uri\":\"https://keys.example.test/jwks\"}" : f.jwks();
                })
                .clock(Clock.fixed(c.now(), ZoneOffset.UTC))
                .build();
        assertThat(v.verify("DPoP " + c.token(), c.proof(), c.method(), c.url())).isInstanceOf(FieldVerification.Valid.class);
        assertThat(urls).containsExactly(f.issuer() + "/.well-known/openid-configuration", "https://keys.example.test/jwks");
    }

    private static String b64(String s) {
        return Base64.getUrlEncoder().withoutPadding().encodeToString(s.getBytes(StandardCharsets.UTF_8));
    }

    private static final class MutableClock extends Clock {
        private Instant now;

        MutableClock(Instant now) {
            this.now = now;
        }

        void advance(Duration d) {
            now = now.plus(d);
        }

        @Override
        public ZoneOffset getZone() {
            return ZoneOffset.UTC;
        }

        @Override
        public Clock withZone(java.time.ZoneId zone) {
            return this;
        }

        @Override
        public Instant instant() {
            return now;
        }
    }
}
