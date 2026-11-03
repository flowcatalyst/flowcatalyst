package io.flowcatalyst.sdk.field;

import static org.assertj.core.api.Assertions.assertThat;

import java.math.BigInteger;
import java.security.KeyPair;
import java.security.KeyPairGenerator;
import java.security.Signature;
import java.security.interfaces.ECPublicKey;
import java.security.spec.ECGenParameterSpec;
import java.util.Arrays;
import org.junit.jupiter.api.Test;

class Es256Test {

    private static byte[] raw(byte[] r, byte[] s) {
        byte[] out = new byte[64];
        System.arraycopy(r, 0, out, 32 - r.length, r.length);
        System.arraycopy(s, 0, out, 64 - s.length, s.length);
        return out;
    }

    private static byte[] filled(int n, int v) {
        byte[] b = new byte[n];
        Arrays.fill(b, (byte) v);
        return b;
    }

    @Test
    void derKeepsFullWidthIntegersAndPadsWhenTheTopBitIsSet() {
        byte[] der = Es256.rawToDer(raw(filled(32, 0x7f), filled(32, 0x80)));
        // r: 32 bytes, top bit clear -> no pad; s: 32 bytes, top bit set -> 0x00 pad (33 bytes)
        assertThat(der).hasSize(2 + (2 + 32) + (2 + 33));
        assertThat(der[0]).isEqualTo((byte) 0x30);
        assertThat(der[1]).isEqualTo((byte) (34 + 35));
        assertThat(der[2]).isEqualTo((byte) 0x02);
        assertThat(der[3]).isEqualTo((byte) 32);
        assertThat(der[2 + 34]).isEqualTo((byte) 0x02);
        assertThat(der[2 + 34 + 1]).isEqualTo((byte) 33);
        assertThat(der[2 + 34 + 2]).isEqualTo((byte) 0x00);
    }

    @Test
    void derStripsLeadingZeroBytes() {
        byte[] r = new byte[32];
        r[31] = 0x05; // 31 leading zero bytes -> INTEGER 05
        byte[] s = new byte[32];
        s[2] = (byte) 0x90; // 2 leading zeros, then top bit set -> 30 bytes + 0x00 pad
        s[31] = 1;
        byte[] der = Es256.rawToDer(raw(r, s));
        assertThat(der).startsWith(0x30, 3 + 2 + 31, 0x02, 0x01, 0x05, 0x02, 31, 0x00, 0x90);
    }

    @Test
    void zeroIsTheSingleByteInteger() {
        assertThat(Es256.rawToDer(new byte[64])).containsExactly(0x30, 6, 0x02, 1, 0, 0x02, 1, 0);
    }

    @Test
    void derRoundTripsThroughBigIntegerForManyValues() {
        for (int i = 0; i < 200; i++) {
            byte[] r = new byte[32];
            byte[] s = new byte[32];
            new java.util.Random(i).nextBytes(r);
            new java.util.Random(~i).nextBytes(s);
            // force some leading zeros of both kinds
            if (i % 3 == 0) r[0] = 0;
            if (i % 5 == 0) { s[0] = 0; s[1] = 0; }
            byte[] der = Es256.rawToDer(raw(r, s));
            int p = 2;
            assertThat(der[p++]).isEqualTo((byte) 0x02);
            byte[] rd = Arrays.copyOfRange(der, p + 1, p + 1 + der[p]);
            p += 1 + der[p];
            assertThat(der[p++]).isEqualTo((byte) 0x02);
            byte[] sd = Arrays.copyOfRange(der, p + 1, p + 1 + der[p]);
            assertThat(new BigInteger(rd)).isEqualTo(new BigInteger(1, r));
            assertThat(new BigInteger(sd)).isEqualTo(new BigInteger(1, s));
            assertThat(new BigInteger(rd).signum()).isGreaterThanOrEqualTo(0);
        }
    }

    @Test
    void realSignaturesVerifyIncludingOnesWithShortRorS() throws Exception {
        KeyPairGenerator g = KeyPairGenerator.getInstance("EC");
        g.initialize(new ECGenParameterSpec("secp256r1"));
        KeyPair kp = g.generateKeyPair();
        ECPublicKey pub = (ECPublicKey) kp.getPublic();
        byte[] x = fixed(pub.getW().getAffineX());
        byte[] y = fixed(pub.getW().getAffineY());
        var key = Es256.publicKey(x, y);
        assertThat(key).isNotNull();

        int shortOnes = 0;
        for (int i = 0; i < 2000; i++) {
            byte[] msg = ("message-" + i).getBytes();
            Signature s = Signature.getInstance("SHA256withECDSAinP1363Format");
            s.initSign(kp.getPrivate());
            s.update(msg);
            byte[] sig = s.sign();
            assertThat(sig).hasSize(64);
            if (sig[0] == 0 || sig[32] == 0) shortOnes++;
            assertThat(Es256.verify(key, msg, sig)).as("sig %d", i).isTrue();
            byte[] bad = sig.clone();
            bad[10] ^= 1;
            assertThat(Es256.verify(key, msg, bad)).isFalse();
        }
        // ~1/128 of 4000 integers are a byte short; the loop must have exercised that path.
        assertThat(shortOnes).isPositive();
        assertThat(Es256.verify(key, "m".getBytes(), new byte[63])).isFalse();
    }

    @Test
    void offCurveAndOutOfRangePointsAreRefused() {
        assertThat(Es256.publicKey(filled(32, 1), filled(32, 2))).isNull();
        assertThat(Es256.publicKey(filled(32, 0xff), filled(32, 0xff))).isNull();
    }

    private static byte[] fixed(BigInteger v) {
        byte[] b = v.toByteArray();
        byte[] out = new byte[32];
        int n = Math.min(32, b.length);
        System.arraycopy(b, b.length - n, out, 32 - n, n);
        return out;
    }
}
