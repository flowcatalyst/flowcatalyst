package io.flowcatalyst.sdk.push;

import static org.junit.jupiter.api.Assertions.assertArrayEquals;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import io.flowcatalyst.sdk.push.TicketKey.PublicJwk;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.security.KeyFactory;
import java.security.PublicKey;
import java.security.Signature;
import java.security.spec.EdECPoint;
import java.security.spec.EdECPublicKeySpec;
import java.security.spec.NamedParameterSpec;
import java.math.BigInteger;
import java.time.Instant;
import java.util.ArrayList;
import java.util.Base64;
import java.util.List;
import org.junit.jupiter.api.Assumptions;
import org.junit.jupiter.api.Test;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;
import tools.jackson.databind.json.JsonMapper;

/**
 * The ticket vector of the application framework ({@code conformance/push/tickets.json}, pinned
 * copy in {@code src/test/resources/conformance}). The keys in it are test keys, published on
 * purpose; EdDSA is deterministic, so the helper must reproduce a valid ticket byte for byte, and
 * its output must verify against the vector's own expectations.
 */
class TicketMinterTest {

    private static final ObjectMapper JSON = JsonMapper.builder().build();
    private static final Path VECTOR = Path.of("src", "test", "resources", "conformance", "tickets.json");
    private static final Base64.Decoder B64U = Base64.getUrlDecoder();

    private static final JsonNode V;
    private static final String APP;
    private static final long NOW;
    private static final String CURRENT;
    private static final String PREVIOUS;

    static {
        try {
            V = JSON.readTree(Files.readString(VECTOR));
        } catch (IOException e) {
            throw new IllegalStateException(e);
        }
        APP = V.get("application").asString();
        NOW = V.get("now").asLong();
        CURRENT = V.get("keys").get("current").get("kid").asString() + ":" + V.get("keys").get("current").get("seed").asString();
        PREVIOUS = V.get("keys").get("previous").get("kid").asString() + ":" + V.get("keys").get("previous").get("seed").asString();
    }

    @Test
    void theVectorIsTheFrameworksCopy() throws IOException {
        Path framework = io.flowcatalyst.sdk.outbox.PushOutboxConformanceTestAccess.frameworkRoot();
        Assumptions.assumeTrue(framework != null, "framework checkout not found");
        assertArrayEquals(Files.readAllBytes(framework.resolve("conformance").resolve("push").resolve("tickets.json")),
                Files.readAllBytes(VECTOR), "src/test/resources/conformance/tickets.json differs from the framework's");
    }

    @Test
    void theJwksFromTheConfiguredKeysIsTheVectorsCurrentThenPrevious() {
        TicketMinter m = TicketMinter.fromConfig(APP, CURRENT, List.of(PREVIOUS));
        assertEquals(jwks(V.get("jwks")), m.jwks());
        // a public JWK is enough for a previous key
        TicketMinter m2 = new TicketMinter(APP, TicketKey.parse(CURRENT), List.of(jwks(V.get("jwks")).keys().get(1)));
        assertEquals(jwks(V.get("jwks")), m2.jwks());
        // the public x derived from every seed in the vector is the vector's
        for (String who : List.of("current", "previous", "stranger")) {
            JsonNode k = V.get("keys").get(who);
            assertEquals(k.get("x").asString(), TicketKey.fromSeed(k.get("kid").asString(), k.get("seed").asString())
                    .toPublicJwk().x(), who);
        }
    }

    // ── a checker per push contract 6.4, validated on the vector ────────

    private record Decoded(JsonNode header, JsonNode claims, String signingInput, byte[] sig) {}

    private static Decoded decode(String token) {
        String[] parts = token.split("\\.", -1);
        if (parts.length != 3) return null;
        try {
            return new Decoded(JSON.readTree(new String(B64U.decode(parts[0]), StandardCharsets.UTF_8)),
                    JSON.readTree(new String(B64U.decode(parts[1]), StandardCharsets.UTF_8)),
                    parts[0] + "." + parts[1], B64U.decode(parts[2]));
        } catch (RuntimeException e) {
            return null;
        }
    }

    private static final class StreamClient {
        String value;
    }

    private static boolean verifies(PublicJwk jwk, Decoded t) {
        try {
            byte[] raw = B64U.decode(jwk.x());
            boolean xOdd = (raw[31] & 0x80) != 0;
            raw[31] &= 0x7f;
            byte[] be = new byte[32];
            for (int i = 0; i < 32; i++) be[i] = raw[31 - i];
            PublicKey key = KeyFactory.getInstance("Ed25519")
                    .generatePublic(new EdECPublicKeySpec(NamedParameterSpec.ED25519, new EdECPoint(xOdd, new BigInteger(1, be))));
            Signature s = Signature.getInstance("Ed25519");
            s.initVerify(key);
            s.update(t.signingInput().getBytes(StandardCharsets.US_ASCII));
            return s.verify(t.sig());
        } catch (Exception e) {
            return false;
        }
    }

    /** The vector's "ticketReasons" words. */
    private static String reasonOf(String token, TicketMinter.Jwks jwks, String ctxClient, String proofKey, StreamClient stream) {
        Decoded t = decode(token);
        if (t == null) return "malformed";
        if (!"EdDSA".equals(text(t.header(), "alg"))) return "alg";
        if (!APP.equals(text(t.claims(), "iss"))) return "iss";
        PublicJwk jwk = jwks.keys().stream().filter(k -> k.kid().equals(text(t.header(), "kid"))).findFirst().orElse(null);
        if (jwk == null) return "kid";
        if (!verifies(jwk, t)) return "signature";
        if (!"fw-push".equals(text(t.claims(), "aud"))) return "aud";
        long iat = t.claims().get("iat").asLong();
        long exp = t.claims().get("exp").asLong();
        if (exp <= iat || exp - iat > 300) return "times";
        if (exp <= NOW) return "expired";
        for (JsonNode p : t.claims().get("topics")) {
            if (!p.asString().split(":", -1)[0].equals(APP)) return "topic-outside-iss";
        }
        JsonNode cnf = t.claims().get("cnf");
        if (cnf != null && cnf.hasNonNull("jkt") && !cnf.get("jkt").asString().equals(proofKey)) return "cnf";
        String client = text(t.claims(), "client");
        String want = ctxClient != null ? ctxClient : stream.value;
        if (want != null && !client.equals(want)) return "client-mismatch";
        if (stream.value == null) stream.value = client;
        return null;
    }

    private static String text(JsonNode n, String f) {
        JsonNode v = n.get(f);
        return v == null || v.isNull() ? null : v.asString();
    }

    private static TicketMinter.Jwks jwks(JsonNode node) {
        List<PublicJwk> keys = new ArrayList<>();
        for (JsonNode k : node.get("keys")) {
            keys.add(new PublicJwk(k.get("kty").asString(), k.get("crv").asString(), k.get("x").asString(),
                    k.get("kid").asString(), k.get("use").asString(), k.get("alg").asString()));
        }
        return new TicketMinter.Jwks(keys);
    }

    @Test
    void theVectorsTicketsAreJudgedAsTheVectorSays() {
        TicketMinter.Jwks jwks = jwks(V.get("jwks"));
        int checked = 0;
        for (JsonNode c : V.get("cases")) {
            JsonNode reasons = c.get("expect").get("ticketReasons");
            if (reasons == null) continue;
            StreamClient stream = new StreamClient();
            JsonNode ctx = c.get("context");
            String client = ctx == null || ctx.isNull() ? null : text(ctx, "client");
            String proof = ctx == null || ctx.isNull() ? null : text(ctx, "proofKey");
            int i = 0;
            for (JsonNode tk : c.get("tickets")) {
                assertEquals(text(reasons, i), reasonOf(tk.asString(), jwks, client, proof, stream), c.get("name").asString());
                i++;
            }
            checked++;
        }
        assertTrue(checked >= 15, "checked " + checked + " cases");
    }

    private static String text(JsonNode arr, int i) {
        JsonNode v = arr.get(i);
        return v == null || v.isNull() ? null : v.asString();
    }

    @Test
    void theHelperReproducesTheVectorsTicketsByteForByteAndRefusesWhatItMustNotMint() {
        int reproduced = 0;
        List<String> refused = new ArrayList<>();
        for (JsonNode c : V.get("cases")) {
            JsonNode reasons = c.get("expect").get("ticketReasons");
            int i = -1;
            for (JsonNode tk : c.get("tickets")) {
                i++;
                String token = tk.asString();
                Decoded t = decode(token);
                if (t == null || !"EdDSA".equals(text(t.header(), "alg"))) continue;
                String reason = reasons == null ? null : text(reasons, i);
                JsonNode cl = t.claims();
                List<String> topics = new ArrayList<>();
                for (JsonNode p : cl.get("topics")) topics.add(p.asString());
                String minted = null;
                for (String who : List.of("current", "previous", "stranger")) {
                    // the key bound to the ticket's kid, whichever seed signed it
                    TicketKey key = TicketKey.fromSeed(text(t.header(), "kid"), V.get("keys").get(who).get("seed").asString());
                    try {
                        TicketMinter.Request req = TicketMinter.Request.of(text(cl, "client"), topics)
                                .withNow(Instant.ofEpochSecond(cl.get("iat").asLong()))
                                .withTtlSeconds((int) (cl.get("exp").asLong() - cl.get("iat").asLong()));
                        if (cl.hasNonNull("sub")) req = req.withSub(text(cl, "sub"));
                        if (cl.hasNonNull("cnf")) req = req.withJkt(text(cl.get("cnf"), "jkt"));
                        if (cl.hasNonNull("stream-exp")) req = req.withStreamExp(Instant.ofEpochSecond(cl.get("stream-exp").asLong()));
                        String out = new TicketMinter(text(cl, "iss"), key).mint(req);
                        if (out.equals(token)) {
                            minted = out;
                            break;
                        }
                    } catch (TicketException e) {
                        refused.add(reason == null ? "?" : reason);
                        break;
                    }
                }
                if (reason == null) {
                    assertEquals(token, minted, c.get("name").asString() + ": a valid ticket is reproduced");
                }
                if (minted != null) {
                    reproduced++;
                } else {
                    assertNotEquals(null, reason, c.get("name").asString() + ": only an invalid ticket is out of the helper's reach");
                }
            }
        }
        assertTrue(reproduced >= 12, "reproduced " + reproduced);
        assertTrue(refused.contains("times"), refused.toString());
        assertTrue(refused.contains("topic-outside-iss"), refused.toString());
    }

    @Test
    void ticketsMintedAtTheVectorsTimeVerifyWithTheVectorsExpectations() {
        TicketMinter m = TicketMinter.fromConfig(APP, CURRENT, List.of(PREVIOUS));
        TicketMinter.Jwks jwks = m.jwks();
        Instant before = Instant.ofEpochSecond(NOW - 10);
        List<String> t = List.of(APP + ":order:{id}");

        String t1 = m.mint(TicketMinter.Request.of("acme", t).withSub("prn_1").withNow(before));
        StreamClient a = new StreamClient();
        assertNull(reasonOf(t1, jwks, null, null, a));
        assertEquals("acme", a.value, "the first valid ticket names the stream's client");
        assertEquals("client-mismatch", reasonOf(t1, jwks, "other", null, new StreamClient()));

        String t2 = m.mint(TicketMinter.Request.of("acme", t).withJkt("jkt-A").withNow(before));
        assertNull(reasonOf(t2, jwks, "acme", "jkt-A", new StreamClient()));
        assertEquals("cnf", reasonOf(t2, jwks, "acme", "jkt-B", new StreamClient()));

        String t3 = m.mint(TicketMinter.Request.of("_platform", t).withNow(before));
        StreamClient d = new StreamClient();
        assertNull(reasonOf(t3, jwks, null, null, d));
        assertEquals("_platform", d.value);

        String t4 = m.mint(TicketMinter.Request.of("acme", t).withTtlSeconds(60).withNow(Instant.ofEpochSecond(NOW - 100)));
        assertEquals("expired", reasonOf(t4, jwks, "acme", null, new StreamClient()));

        TicketMinter old = TicketMinter.fromConfig(APP, PREVIOUS, List.of());
        String t5 = old.mint(TicketMinter.Request.of("acme", t).withNow(Instant.ofEpochSecond(NOW)));
        assertNull(reasonOf(t5, jwks, "acme", null, new StreamClient()), "signed with the previous key");
        assertEquals("kid", reasonOf(t5, TicketMinter.fromConfig(APP, CURRENT, List.of()).jwks(), "acme", null, new StreamClient()),
                "the previous key dropped from the JWKS: unknown kid");

        // header and claims order are the contract's (the vector's tickets decode alike)
        Decoded d1 = decode(t1);
        assertEquals(List.of("alg", "kid"), d1.header().propertyNames().stream().toList());
        assertEquals(List.of("iss", "aud", "client", "topics", "sub", "iat", "exp"), d1.claims().propertyNames().stream().toList());
        Decoded d2 = decode(m.mint(TicketMinter.Request.of("acme", List.of(APP + ":a:{x}")).withSub("p").withJkt("k")
                .withStreamExp(Instant.ofEpochSecond(NOW + 60)).withNow(Instant.ofEpochSecond(NOW))));
        assertEquals(List.of("iss", "aud", "client", "topics", "sub", "cnf", "iat", "exp", "stream-exp"),
                d2.claims().propertyNames().stream().toList());
        assertEquals(NOW + 60, d2.claims().get("stream-exp").asLong());
    }

    // ── the helper's own rules ──────────────────────────────────────────

    private final TicketMinter shop = TicketMinter.fromConfig("shop", "k1:" + V.get("keys").get("current").get("seed").asString(), List.of());

    @Test
    void refusesATopicOfAnotherApplicationNoTopicsTooManyTopicsABadTtlAndABadClient() {
        List<String> ok = List.of("shop:orders:{id}");
        assertThrows(TicketException.class, () -> shop.mint("acme", List.of("orders:order:{id}")));
        assertThrows(TicketException.class, () -> shop.mint("acme", List.of("shop")));
        assertThrows(TicketException.class, () -> shop.mint("acme", List.of()));
        List<String> many = new ArrayList<>();
        for (int i = 0; i < 101; i++) many.add("shop:t:" + i);
        assertThrows(TicketException.class, () -> shop.mint("acme", many));
        many.removeLast();
        shop.mint("acme", many);
        assertThrows(TicketException.class, () -> shop.mint(TicketMinter.Request.of("acme", ok).withTtlSeconds(301)));
        assertThrows(TicketException.class, () -> shop.mint(TicketMinter.Request.of("acme", ok).withTtlSeconds(0)));
        shop.mint(TicketMinter.Request.of("acme", ok).withTtlSeconds(300));
        assertThrows(TicketException.class, () -> shop.mint("a b", ok));
    }

    @Test
    void keyConfigurationIsKidColonSeedWithA32ByteSeed() {
        String seed = V.get("keys").get("current").get("seed").asString();
        assertThrows(TicketException.class, () -> TicketKey.parse("noseed"));
        assertThrows(TicketException.class, () -> TicketKey.parse(":abc"));
        assertThrows(TicketException.class, () -> TicketKey.parse("k:AAAA"));
        assertThrows(TicketException.class, () -> TicketKey.parse("k".repeat(65) + ":" + seed));
        assertThrows(TicketException.class, () -> new TicketMinter("Shop", TicketKey.parse("k:" + seed)));
    }

    @Test
    void theJwksCarriesNoPrivateMemberAndDuplicateKidsAreRefused() {
        PublicJwk k = shop.jwks().keys().getFirst();
        assertEquals(List.of("kty", "crv", "x", "kid", "use", "alg"),
                JSON.valueToTree(k).propertyNames().stream().toList());
        TicketKey same = TicketKey.parse("k1:" + V.get("keys").get("previous").get("seed").asString());
        assertThrows(TicketException.class, () -> new TicketMinter("shop", same, List.of(k)).jwks());
    }

    @Test
    void theDefaultLifetimeIs60Seconds() {
        Decoded t = decode(shop.mint(TicketMinter.Request.of("acme", List.of("shop:a:b")).withNow(Instant.ofEpochSecond(1000))));
        assertEquals(60, t.claims().get("exp").asLong() - t.claims().get("iat").asLong());
    }
}
