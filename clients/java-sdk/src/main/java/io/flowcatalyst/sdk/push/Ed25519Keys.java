package io.flowcatalyst.sdk.push;

import java.math.BigInteger;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;

/**
 * Derives an Ed25519 public key from its 32-byte seed (RFC 8032 section 5.1.5). The JDK signs and
 * verifies Ed25519 but has no way to ask a private key for its public half, and a ticket key is
 * configured as the seed alone.
 */
final class Ed25519Keys {

    private static final BigInteger P = BigInteger.ONE.shiftLeft(255).subtract(BigInteger.valueOf(19));
    private static final BigInteger D = BigInteger.valueOf(-121665)
            .multiply(BigInteger.valueOf(121666).modInverse(P)).mod(P);
    private static final BigInteger SQRT_M1 =
            BigInteger.TWO.modPow(P.subtract(BigInteger.ONE).shiftRight(2), P);
    private static final BigInteger[] BASE = basePoint();

    private Ed25519Keys() {}

    /** The 32-byte compressed public key of {@code seed}. */
    static byte[] publicKey(byte[] seed) {
        byte[] h;
        try {
            h = MessageDigest.getInstance("SHA-512").digest(seed);
        } catch (NoSuchAlgorithmException e) {
            throw new IllegalStateException(e);
        }
        byte[] lower = new byte[32];
        System.arraycopy(h, 0, lower, 0, 32);
        lower[0] &= (byte) 248;
        lower[31] &= 127;
        lower[31] |= 64;
        BigInteger a = new BigInteger(1, reverse(lower));

        BigInteger[] result = {BigInteger.ZERO, BigInteger.ONE}; // the neutral point
        BigInteger[] addend = BASE;
        for (int i = 0; i < 255; i++) {
            if (a.testBit(i)) {
                result = add(result, addend);
            }
            addend = add(addend, addend);
        }

        byte[] y = reverse(toFixed(result[1], 32));
        if (result[0].testBit(0)) {
            y[31] |= (byte) 0x80;
        }
        return y;
    }

    /** Edwards addition on the affine curve -x^2 + y^2 = 1 + d x^2 y^2. */
    private static BigInteger[] add(BigInteger[] p, BigInteger[] q) {
        BigInteger x1y2 = p[0].multiply(q[1]);
        BigInteger x2y1 = q[0].multiply(p[1]);
        BigInteger y1y2 = p[1].multiply(q[1]);
        BigInteger x1x2 = p[0].multiply(q[0]);
        BigInteger dxy = D.multiply(x1x2).multiply(y1y2).mod(P);
        BigInteger x3 = x1y2.add(x2y1).multiply(BigInteger.ONE.add(dxy).modInverse(P)).mod(P);
        BigInteger y3 = y1y2.add(x1x2).multiply(BigInteger.ONE.subtract(dxy).mod(P).modInverse(P)).mod(P);
        return new BigInteger[] {x3, y3};
    }

    private static BigInteger[] basePoint() {
        BigInteger y = BigInteger.valueOf(4).multiply(BigInteger.valueOf(5).modInverse(P)).mod(P);
        BigInteger x2 = y.multiply(y).subtract(BigInteger.ONE).mod(P)
                .multiply(D.multiply(y).multiply(y).add(BigInteger.ONE).mod(P).modInverse(P)).mod(P);
        BigInteger x = x2.modPow(P.add(BigInteger.valueOf(3)).shiftRight(3), P);
        if (!x.multiply(x).subtract(x2).mod(P).equals(BigInteger.ZERO)) {
            x = x.multiply(SQRT_M1).mod(P);
        }
        if (x.testBit(0)) { // the base point's x is even
            x = P.subtract(x);
        }
        return new BigInteger[] {x, y};
    }

    private static byte[] toFixed(BigInteger v, int length) {
        byte[] raw = v.toByteArray();
        byte[] out = new byte[length];
        int copy = Math.min(raw.length, length);
        System.arraycopy(raw, raw.length - copy, out, length - copy, copy);
        return out;
    }

    private static byte[] reverse(byte[] in) {
        byte[] out = new byte[in.length];
        for (int i = 0; i < in.length; i++) {
            out[i] = in[in.length - 1 - i];
        }
        return out;
    }
}
