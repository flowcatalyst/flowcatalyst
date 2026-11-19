package io.flowcatalyst.sdk.field;

import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.sdk.http.Json;
import java.io.IOException;
import java.math.BigInteger;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.security.GeneralSecurityException;
import java.security.KeyPair;
import java.security.KeyPairGenerator;
import java.security.MessageDigest;
import java.security.Signature;
import java.security.interfaces.ECPublicKey;
import java.security.interfaces.RSAPublicKey;
import java.security.spec.ECGenParameterSpec;
import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Base64;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.DynamicTest;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.TestFactory;
import tools.jackson.databind.JsonNode;

/**
 * A field token and its DPoP proof verify only in the spelling they were issued in
 * (docs/wire-contract.md, Authentication, "Token spelling"): the issued value re-spelled
 * (another last character with the same significant bits, '=' padding, the standard
 * alphabet, a line break, whitespace around a proof) and the issued value changed are both
 * refused, and the issued value still verifies afterwards.
 */
class CanonicalSpellingTest {

    private static final String ALPHABET = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_";
    private static final Base64.Encoder ENC = Base64.getUrlEncoder().withoutPadding();
    private static final Path DIR = Path.of("..", "..", "api", "field-fixtures");
    private static final String ISSUER = "https://fc.example.test";
    private static final String APP = "canonical-app";
    private static final String URL = "https://app.example.test/api/orders";
    private static final Instant NOW = Instant.parse("2026-11-19T12:00:00Z");

    private static KeyPair rsa;
    private static KeyPair device;
    private static String jwks;
    private static String deviceJwk;
    private static String jkt;

    @BeforeAll
    static void keys() throws GeneralSecurityException {
        KeyPairGenerator rg = KeyPairGenerator.getInstance("RSA");
        rg.initialize(2048); // a 256-byte signature: 342 characters, 4 spare bits
        rsa = rg.generateKeyPair();
        RSAPublicKey pub = (RSAPublicKey) rsa.getPublic();
        jwks = "{\"keys\":[{\"kty\":\"RSA\",\"kid\":\"k1\",\"use\":\"sig\",\"alg\":\"RS256\",\"n\":\""
                + ENC.encodeToString(unsigned(pub.getModulus())) + "\",\"e\":\""
                + ENC.encodeToString(unsigned(pub.getPublicExponent())) + "\"}]}";
        KeyPairGenerator eg = KeyPairGenerator.getInstance("EC");
        eg.initialize(new ECGenParameterSpec("secp256r1"));
        device = eg.generateKeyPair();
        ECPublicKey ec = (ECPublicKey) device.getPublic();
        String x = ENC.encodeToString(fixed(ec.getW().getAffineX()));
        String y = ENC.encodeToString(fixed(ec.getW().getAffineY()));
        deviceJwk = "{\"kty\":\"EC\",\"crv\":\"P-256\",\"x\":\"" + x + "\",\"y\":\"" + y + "\"}";
        jkt = ENC.encodeToString(sha256("{\"crv\":\"P-256\",\"kty\":\"EC\",\"x\":\"" + x + "\",\"y\":\"" + y + "\"}"));
    }

    private static byte[] unsigned(BigInteger v) {
        byte[] b = v.toByteArray();
        return b[0] == 0 ? Arrays.copyOfRange(b, 1, b.length) : b;
    }

    private static byte[] fixed(BigInteger v) {
        byte[] b = unsigned(v);
        byte[] out = new byte[32];
        System.arraycopy(b, 0, out, 32 - b.length, b.length);
        return out;
    }

    private static byte[] sha256(String s) {
        try {
            return MessageDigest.getInstance("SHA-256").digest(s.getBytes(StandardCharsets.UTF_8));
        } catch (GeneralSecurityException e) {
            throw new IllegalStateException(e);
        }
    }

    private static String b64(String json) {
        return ENC.encodeToString(json.getBytes(StandardCharsets.UTF_8));
    }

    private static String sign(String alg, java.security.PrivateKey key, String input) {
        try {
            Signature s = Signature.getInstance(alg);
            s.initSign(key);
            s.update(input.getBytes(StandardCharsets.US_ASCII));
            return input + "." + ENC.encodeToString(s.sign());
        } catch (GeneralSecurityException e) {
            throw new IllegalStateException(e);
        }
    }

    private static String token(String sub) {
        long now = NOW.getEpochSecond();
        return sign("SHA256withRSA", rsa.getPrivate(),
                b64("{\"alg\":\"RS256\",\"typ\":\"fc-field+jwt\",\"kid\":\"k1\"}") + "."
                        + b64("{\"iss\":\"" + ISSUER + "\",\"sub\":\"" + sub + "\",\"aud\":\"" + APP + "\",\"exp\":"
                                + (now + 600) + ",\"iat\":" + (now - 60) + ",\"jti\":\"j1\",\"token_use\":\"field\","
                                + "\"device_mode\":\"PERSONAL\",\"amr\":[\"pin\"],\"cnf\":{\"jkt\":\"" + jkt + "\"}}"));
    }

    /** A proof for {@code token}: its ath is the hash of the token's exact text. */
    private static String proof(String token, String jti) {
        return sign("SHA256withECDSAinP1363Format", device.getPrivate(),
                b64("{\"alg\":\"ES256\",\"typ\":\"dpop+jwt\",\"jwk\":" + deviceJwk + "}") + "."
                        + b64("{\"htm\":\"POST\",\"htu\":\"" + URL + "\",\"iat\":" + NOW.getEpochSecond() + ",\"jti\":\""
                                + jti + "\",\"ath\":\"" + ENC.encodeToString(sha256(token)) + "\"}"));
    }

    private static FieldTokenVerifier verifier(String keys, Instant now) {
        return FieldTokenVerifier.builder()
                .issuer(ISSUER)
                .appCode(APP)
                .jwksUrl("https://keys.example.test/jwks")
                .fetcher(u -> keys)
                .clock(Clock.fixed(now, ZoneOffset.UTC))
                .build();
    }

    // ── the re-spellings and the tamper cases ─────────────────────────

    /** Spare low bits of the last character of an unpadded base64url string of n characters. */
    static int spareBits(int n) {
        return switch (n % 4) {
            case 2 -> 4;
            case 3 -> 2;
            default -> 0;
        };
    }

    /** Every other last character of seg that a lenient decoder reads as the same bytes. */
    static List<String> nonCanonicalSpellings(String seg) {
        int mask = (1 << spareBits(seg.length())) - 1;
        int last = ALPHABET.indexOf(seg.charAt(seg.length() - 1));
        List<String> out = new ArrayList<>();
        for (int low = 1; low <= mask; low++) {
            out.add(seg.substring(0, seg.length() - 1) + ALPHABET.charAt((last & ~mask) | low));
        }
        return out;
    }

    /** The issued value spelled other ways: same bytes to a lenient decoder. */
    static Map<String, String> respellings(String issued, boolean withSurroundingWhitespace) {
        String[] p = issued.split("\\.", -1);
        String sig = p[2];
        String head = p[0] + "." + p[1] + ".";
        Map<String, String> out = new LinkedHashMap<>();
        byte[] bytes = Base64.getUrlDecoder().decode(sig);
        for (String v : nonCanonicalSpellings(sig)) {
            assertThat(Base64.getUrlDecoder().decode(v)).as("test bug: %s is another spelling", v).isEqualTo(bytes);
            out.put("signature ends in " + v.charAt(v.length() - 1), head + v);
        }
        String pad = "=".repeat((4 - sig.length() % 4) % 4);
        if (!pad.isEmpty()) out.put("signature padded", head + sig + pad);
        if (pad.length() == 2) out.put("signature padded once", head + sig + "=");
        String std = sig.replace('-', '+').replace('_', '/');
        if (!std.equals(sig)) out.put("signature in the standard alphabet", head + std);
        int mid = sig.length() / 2;
        out.put("line feed inside the signature", head + sig.substring(0, mid) + "\n" + sig.substring(mid));
        out.put("carriage return inside the signature", head + sig.substring(0, mid) + "\r" + sig.substring(mid));
        out.put("CRLF inside the signature", head + sig.substring(0, mid) + "\r\n" + sig.substring(mid));
        if (withSurroundingWhitespace) {
            out.put("leading space", " " + issued);
            out.put("trailing space", issued + " ");
            out.put("CRLF after it", issued + "\r\n");
        }
        return out;
    }

    /** The issued value changed in ways that certainly alter its content or signature bytes. */
    static Map<String, String> tampered(String issued, String claim, String otherClaim) {
        String[] p = issued.split("\\.", -1);
        byte[] sig = Base64.getUrlDecoder().decode(p[2]);
        String payload = new String(Base64.getUrlDecoder().decode(p[1]), StandardCharsets.UTF_8);
        assertThat(payload).as("test bug: the claim must occur in the payload").contains(claim);
        byte[] first = sig.clone();
        first[0] ^= (byte) 0x80;
        byte[] last = sig.clone();
        last[last.length - 1] ^= 0x01;
        char swapped = p[1].charAt(0) == 'A' ? 'B' : 'A';
        Map<String, String> out = new LinkedHashMap<>();
        out.put("first signature bit flipped", p[0] + "." + p[1] + "." + ENC.encodeToString(first));
        out.put("last signature bit flipped", p[0] + "." + p[1] + "." + ENC.encodeToString(last));
        out.put("payload claim changed", p[0] + "." + b64(payload.replace(claim, otherClaim)) + "." + p[2]);
        out.put("payload character changed", p[0] + "." + swapped + p[1].substring(1) + "." + p[2]);
        out.put("signature truncated by one byte",
                p[0] + "." + p[1] + "." + ENC.encodeToString(Arrays.copyOf(sig, sig.length - 1)));
        out.put("truncated by one character", issued.substring(0, issued.length() - 1));
        out.put("signature removed", p[0] + "." + p[1] + ".");
        out.put("signature segment removed", p[0] + "." + p[1]);
        return out;
    }

    private static String reason(FieldVerification r) {
        return r instanceof FieldVerification.Invalid i ? i.reason() : r.getClass().getSimpleName();
    }

    @Test
    void aFieldTokenVerifiesOnlyAsIssued() {
        FieldTokenVerifier v = verifier(jwks, NOW);
        String tok = token("fwu_canonical");
        assertThat(v.verify("DPoP " + tok, proof(tok, "p1"), "POST", URL)).isInstanceOf(FieldVerification.Valid.class);
        // Whitespace around the token is the Authorization header's framing, not the token.
        Map<String, String> cases = respellings(tok, false);
        assertThat(cases).hasSize(15 + 6);
        cases.forEach((name, bad) -> assertThat(reason(v.verify("DPoP " + bad, proof(bad, "p1"), "POST", URL)))
                .as(name)
                .isEqualTo(FieldReason.MALFORMED));
        tampered(tok, "fwu_canonical", "fwu_somebody").forEach((name, bad) -> {
            assertThat(bad).isNotEqualTo(tok);
            assertThat(v.verify("DPoP " + bad, proof(bad, "p1"), "POST", URL)).as(name).isInstanceOf(FieldVerification.Invalid.class);
        });
        assertThat(v.verify("DPoP " + tok, proof(tok, "p1"), "POST", URL)).isInstanceOf(FieldVerification.Valid.class);
    }

    @Test
    void aProofVerifiesOnlyAsIssued() {
        FieldTokenVerifier v = verifier(jwks, NOW);
        String tok = token("fwu_canonical");
        String pr = proof(tok, "proof-jti-1");
        assertThat(v.verify("DPoP " + tok, pr, "POST", URL)).isInstanceOf(FieldVerification.Valid.class);
        Map<String, String> cases = respellings(pr, true); // a 64-byte signature: 86 characters, 4 spare bits
        assertThat(cases).hasSize(15 + 6 + 3);
        cases.forEach((name, bad) -> assertThat(reason(v.verify("DPoP " + tok, bad, "POST", URL)))
                .as(name)
                .isEqualTo(FieldReason.INVALID_PROOF));
        tampered(pr, "proof-jti-1", "proof-jti-2").forEach((name, bad) -> assertThat(
                        v.verify("DPoP " + tok, bad, "POST", URL))
                .as(name)
                .isInstanceOf(FieldVerification.Invalid.class));
        assertThat(v.verify("DPoP " + tok, pr, "POST", URL)).isInstanceOf(FieldVerification.Valid.class);
    }

    @Test
    void compactJwsAcceptsTheEncodersOutputAndNothingElse() {
        for (int n = 0; n < 10; n++) {
            byte[] b = new byte[n];
            for (int i = 0; i < n; i++) b[i] = (byte) (0xff - i);
            String s = ENC.encodeToString(b);
            assertThat(CompactJws.decodeSegment(s)).as("length %d", n).isEqualTo(b);
        }
        String sig = ENC.encodeToString(new byte[] {(byte) 0xfb, (byte) 0xff, (byte) 0xbf, (byte) 0xfe}); // "-_-_-g"
        List<String> bad = new ArrayList<>(nonCanonicalSpellings(sig));
        assertThat(bad).hasSize(15);
        bad.addAll(List.of(sig + "==", "+/+/+g", sig.substring(0, 2) + "\n" + sig.substring(2), sig + "\r\n", " " + sig, "A"));
        for (String s : bad) {
            assertThat(CompactJws.isCanonical(s)).as("%s", s).isFalse();
        }
        assertThat(CompactJws.isCanonicalCompact("eyJhbGciOiJIUzI1NiJ9.e30.AAAAAA")).isTrue();
        for (String s : new String[] {"", "a.b", "a.b.c.d", "eyJhbGciOiJIUzI1NiJ9.e30.AAAAAB", "eyJhbGciOiJIUzI1NiJ9.e30=.AAAAAA"}) {
            assertThat(CompactJws.isCanonicalCompact(s)).as("%s", s).isFalse();
        }
    }

    /** The shared vectors: the valid fixture's token and proof re-spelled, each refused. */
    @TestFactory
    List<DynamicTest> everyNonCanonicalFixtureIsRefused() throws IOException {
        Path file = DIR.resolve("noncanonical.json");
        if (!Files.exists(file)) {
            return List.of(DynamicTest.dynamicTest(
                    "SKIPPED: field fixtures not present at " + DIR.toAbsolutePath() + " (standalone SDK checkout)",
                    () -> org.junit.jupiter.api.Assumptions.abort("field fixtures absent")));
        }
        JsonNode root = Json.newMapper().readTree(Files.readString(file));
        String keys = Files.readString(DIR.resolve("jwks.json"));
        List<DynamicTest> tests = new ArrayList<>();
        for (JsonNode c : root.get("cases")) {
            tests.add(DynamicTest.dynamicTest(c.get("name").asString(), () -> {
                FieldVerification r = FieldTokenVerifier.builder()
                        .issuer(root.get("issuer").asString())
                        .appCode(root.get("appCode").asString())
                        .jwksUrl("https://keys.example.test/jwks")
                        .fetcher(u -> keys)
                        .clock(Clock.fixed(Instant.parse(c.get("now").asString()), ZoneOffset.UTC))
                        .build()
                        .verify("DPoP " + c.get("accessToken").asString(), c.get("dpopProof").asString(),
                                c.get("method").asString(), c.get("url").asString());
                assertThat(c.get("expect").asString()).isNotEqualTo("valid");
                assertThat(reason(r)).isEqualTo(c.get("expect").asString());
            }));
        }
        assertThat(tests).hasSizeGreaterThan(50);
        return tests;
    }
}
