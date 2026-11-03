package io.flowcatalyst.sdk.field;

import java.math.BigInteger;
import java.security.AlgorithmParameters;
import java.security.GeneralSecurityException;
import java.security.KeyFactory;
import java.security.PublicKey;
import java.security.Signature;
import java.security.spec.ECGenParameterSpec;
import java.security.spec.ECParameterSpec;
import java.security.spec.ECPoint;
import java.security.spec.ECPublicKeySpec;
import java.util.Arrays;

/** ES256 (ECDSA P-256 / SHA-256) helpers: JWS raw r||s to DER, and public-key checks. */
final class Es256 {

    private Es256() {}

    private static final ECParameterSpec P256 = p256();

    private static ECParameterSpec p256() {
        try {
            AlgorithmParameters params = AlgorithmParameters.getInstance("EC");
            params.init(new ECGenParameterSpec("secp256r1"));
            return params.getParameterSpec(ECParameterSpec.class);
        } catch (GeneralSecurityException e) {
            throw new ExceptionInInitializerError(e);
        }
    }

    /** A P-256 public key from affine coordinates, or null if the point is not on the curve. */
    static PublicKey publicKey(byte[] x, byte[] y) {
        BigInteger bx = new BigInteger(1, x);
        BigInteger by = new BigInteger(1, y);
        BigInteger p = ((java.security.spec.ECFieldFp) P256.getCurve().getField()).getP();
        if (bx.compareTo(p) >= 0 || by.compareTo(p) >= 0) {
            return null;
        }
        // y^2 = x^3 + ax + b (mod p)
        BigInteger lhs = by.multiply(by).mod(p);
        BigInteger rhs = bx.pow(3).add(P256.getCurve().getA().multiply(bx)).add(P256.getCurve().getB()).mod(p);
        if (!lhs.equals(rhs)) {
            return null;
        }
        try {
            return KeyFactory.getInstance("EC").generatePublic(new ECPublicKeySpec(new ECPoint(bx, by), P256));
        } catch (GeneralSecurityException e) {
            return null;
        }
    }

    /** Verifies a JWS ES256 signature (64 bytes, r||s) over {@code signingInput}. */
    static boolean verify(PublicKey key, byte[] signingInput, byte[] rawSignature) {
        if (rawSignature.length != 64) {
            return false;
        }
        try {
            Signature sig = Signature.getInstance("SHA256withECDSA");
            sig.initVerify(key);
            sig.update(signingInput);
            return sig.verify(rawToDer(rawSignature));
        } catch (GeneralSecurityException e) {
            return false;
        }
    }

    /**
     * Converts the JWS form (fixed-width big-endian r||s, 32 bytes each) to the ASN.1 DER
     * {@code SEQUENCE { INTEGER r, INTEGER s }} the JDK expects. Leading zero bytes are
     * stripped, and a zero byte is prepended when the top bit is set so the INTEGER stays
     * positive. Package-visible for tests.
     */
    static byte[] rawToDer(byte[] raw) {
        int half = raw.length / 2;
        byte[] r = derInteger(Arrays.copyOfRange(raw, 0, half));
        byte[] s = derInteger(Arrays.copyOfRange(raw, half, raw.length));
        int len = r.length + s.length;
        // len <= 2 * (2 + 33) = 70 for P-256, so the short length form always applies; the
        // long form is here so the helper stays correct for any input.
        byte[] header = len < 128 ? new byte[] {0x30, (byte) len} : new byte[] {0x30, (byte) 0x81, (byte) len};
        byte[] out = new byte[header.length + len];
        System.arraycopy(header, 0, out, 0, header.length);
        System.arraycopy(r, 0, out, header.length, r.length);
        System.arraycopy(s, 0, out, header.length + r.length, s.length);
        return out;
    }

    private static byte[] derInteger(byte[] unsigned) {
        int start = 0;
        while (start < unsigned.length - 1 && unsigned[start] == 0) {
            start++;
        }
        boolean pad = (unsigned[start] & 0x80) != 0;
        int n = unsigned.length - start;
        byte[] out = new byte[2 + (pad ? 1 : 0) + n];
        out[0] = 0x02;
        out[1] = (byte) (n + (pad ? 1 : 0));
        if (pad) {
            out[2] = 0;
        }
        System.arraycopy(unsigned, start, out, 2 + (pad ? 1 : 0), n);
        return out;
    }
}
