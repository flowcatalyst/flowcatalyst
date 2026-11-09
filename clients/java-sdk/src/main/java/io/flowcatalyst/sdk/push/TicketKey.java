package io.flowcatalyst.sdk.push;

import java.security.GeneralSecurityException;
import java.security.KeyFactory;
import java.security.PrivateKey;
import java.security.Signature;
import java.security.spec.PKCS8EncodedKeySpec;
import java.util.Arrays;
import java.util.Base64;

/**
 * An Ed25519 ticket signing key: a {@code kid} plus the 32-byte seed. Configure it as
 * {@code kid:base64url-seed} ({@link #parse}); the seed is a secret, the derived public JWK
 * ({@link #toPublicJwk()}) is what the push gateway needs.
 */
public final class TicketKey {

    /** An Ed25519 public key as a JWK, the shape the gateway stores and {@code /push/keys} serves. */
    public record PublicJwk(String kty, String crv, String x, String kid, String use, String alg) {
        public PublicJwk {
            if (kty == null || crv == null || x == null || kid == null || use == null || alg == null) {
                throw new TicketException("a ticket public key needs kty, crv, x, kid, use and alg");
            }
        }

        /** An Ed25519 signing key's JWK with the given {@code x} (base64url) and {@code kid}. */
        public static PublicJwk ed25519(String kid, String x) {
            return new PublicJwk("OKP", "Ed25519", x, kid, "sig", "EdDSA");
        }
    }

    // PKCS#8 prefix of a raw Ed25519 seed (RFC 8410).
    private static final byte[] PKCS8_ED25519_PREFIX = hex("302e020100300506032b657004220420");

    private final String kid;
    private final PrivateKey privateKey;
    private final PublicJwk publicJwk;

    private TicketKey(String kid, PrivateKey privateKey, PublicJwk publicJwk) {
        this.kid = kid;
        this.privateKey = privateKey;
        this.publicJwk = publicJwk;
    }

    /** From a {@code kid:base64url-seed} string (the seed is the 32 raw bytes, base64url without padding). */
    public static TicketKey parse(String config) {
        int i = config.indexOf(':');
        if (i <= 0 || i == config.length() - 1) {
            throw new TicketException("a ticket key is kid:base64url-seed");
        }
        return fromSeed(config.substring(0, i).strip(), config.substring(i + 1).strip());
    }

    public static TicketKey fromSeed(String kid, String seedBase64Url) {
        if (kid == null || kid.isEmpty() || kid.length() > 64) {
            throw new TicketException("a ticket key id is 1 to 64 characters");
        }
        byte[] seed;
        String stripped = seedBase64Url.replaceAll("=+$", "");
        try {
            seed = Base64.getUrlDecoder().decode(stripped);
        } catch (IllegalArgumentException e) {
            throw new TicketException("a ticket key seed is 32 bytes, base64url");
        }
        if (seed.length != 32 || !Base64.getUrlEncoder().withoutPadding().encodeToString(seed).equals(stripped)) {
            throw new TicketException("a ticket key seed is 32 bytes, base64url");
        }
        try {
            byte[] pkcs8 = new byte[PKCS8_ED25519_PREFIX.length + 32];
            System.arraycopy(PKCS8_ED25519_PREFIX, 0, pkcs8, 0, PKCS8_ED25519_PREFIX.length);
            System.arraycopy(seed, 0, pkcs8, PKCS8_ED25519_PREFIX.length, 32);
            PrivateKey key = KeyFactory.getInstance("Ed25519").generatePrivate(new PKCS8EncodedKeySpec(pkcs8));
            String x = Base64.getUrlEncoder().withoutPadding().encodeToString(Ed25519Keys.publicKey(seed));
            return new TicketKey(kid, key, PublicJwk.ed25519(kid, x));
        } catch (GeneralSecurityException e) {
            throw new TicketException("could not load the ticket key: " + e.getMessage());
        } finally {
            Arrays.fill(seed, (byte) 0);
        }
    }

    public String kid() {
        return kid;
    }

    /** The public key as a JWK (no private member). */
    public PublicJwk toPublicJwk() {
        return publicJwk;
    }

    /** The Ed25519 signature (64 bytes) of {@code data}. */
    byte[] sign(byte[] data) {
        try {
            Signature signature = Signature.getInstance("Ed25519");
            signature.initSign(privateKey);
            signature.update(data);
            return signature.sign();
        } catch (GeneralSecurityException e) {
            throw new TicketException("could not sign the ticket: " + e.getMessage());
        }
    }

    @Override
    public String toString() {
        return "TicketKey[kid=" + kid + "]";
    }

    private static byte[] hex(String s) {
        byte[] out = new byte[s.length() / 2];
        for (int i = 0; i < out.length; i++) {
            out[i] = (byte) Integer.parseInt(s.substring(2 * i, 2 * i + 2), 16);
        }
        return out;
    }
}
