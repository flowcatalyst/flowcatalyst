package io.flowcatalyst.sdk.field;

import java.util.Base64;

/**
 * The one spelling of a compact JWS that verifies (docs/wire-contract.md, Authentication,
 * "Token spelling"): three segments of RFC 7515 base64url, which is the URL-safe alphabet with
 * no {@code =} padding, no {@code +} or {@code /}, no whitespace or line break, and (RFC 4648
 * §3.5) the spare low bits of the last character zero. A standard encoder writes exactly that,
 * so a token the platform issued and a proof the field libraries signed are always canonical.
 *
 * <p>{@link Base64#getUrlDecoder()} alone is not enough: it accepts {@code =} padding and
 * ignores the spare bits, so a signature would have several spellings that all verify. A
 * segment is accepted here only when it is of the URL-safe alphabet and the encoder writes it
 * back exactly from the bytes it decodes to.
 */
final class CompactJws {

    private static final Base64.Decoder DECODER = Base64.getUrlDecoder();
    private static final Base64.Encoder ENCODER = Base64.getUrlEncoder().withoutPadding();

    private CompactJws() {}

    /** Whether {@code seg} is one segment of canonical base64url. */
    static boolean isCanonical(String seg) {
        if (seg == null || seg.length() % 4 == 1) {
            return false;
        }
        for (int i = 0; i < seg.length(); i++) {
            char c = seg.charAt(i);
            boolean ok = c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_';
            if (!ok) {
                return false;
            }
        }
        // The alphabet is right; the spare bits are zero exactly when re-encoding gives it back.
        return ENCODER.encodeToString(DECODER.decode(seg)).equals(seg);
    }

    /** Whether {@code token} is three dot-separated segments, each canonical. */
    static boolean isCanonicalCompact(String token) {
        if (token == null) {
            return false;
        }
        String[] parts = token.split("\\.", -1);
        if (parts.length != 3) {
            return false;
        }
        for (String p : parts) {
            if (!isCanonical(p)) {
                return false;
            }
        }
        return true;
    }

    /**
     * Decodes one canonical segment.
     *
     * @throws IllegalArgumentException when {@code seg} is not canonical base64url
     */
    static byte[] decodeSegment(String seg) {
        if (!isCanonical(seg)) {
            throw new IllegalArgumentException("not canonical base64url");
        }
        return DECODER.decode(seg);
    }
}
