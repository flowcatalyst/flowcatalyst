package io.flowcatalyst.sdk.field;

import io.flowcatalyst.sdk.http.Json;
import java.io.IOException;
import java.math.BigInteger;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.charset.StandardCharsets;
import java.security.GeneralSecurityException;
import java.security.KeyFactory;
import java.security.MessageDigest;
import java.security.PublicKey;
import java.security.Signature;
import java.security.spec.RSAPublicKeySpec;
import java.time.Clock;
import java.time.Duration;
import java.time.Instant;
import java.util.ArrayList;
import java.util.Base64;
import java.util.List;
import java.util.Objects;
import java.util.function.Function;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;

/**
 * Verifies field-worker access tokens and their DPoP proofs, per request.
 *
 * <p>A field worker signs in from an enrolled device with a PIN at the platform's {@code
 * /field/*}; the platform issues a short RS256 access token (JOSE {@code typ: fc-field+jwt},
 * {@code token_use: "field"}, {@code aud} = your application code) bound to the device key
 * ({@code cnf.jkt}). The device presents it to your backend as {@code Authorization: DPoP
 * <token>} plus {@code DPoP: <proof>} (an ES256 JWS signed by the same key). PINs never reach
 * your backend: this class only verifies.
 *
 * <pre>{@code
 * var verifier = FieldTokenVerifier.builder()
 *         .issuer("https://auth.flowcatalyst.io")
 *         .appCode("my-app")
 *         .build();
 *
 * switch (verifier.verify(authorization, dpop, "POST", "https://app.example.com/api/orders")) {
 *     case FieldVerification.Valid v -> handle(v.principal());
 *     case FieldVerification.Invalid i -> respond(401, i.reason());
 *     case FieldVerification.Unavailable u -> respond(503, "unavailable");
 * }
 * }</pre>
 *
 * <p>The request URL is the full public URL you pass in (scheme, host, path; the query is
 * ignored); it is never taken from a Host header. Nonce checking is not done here; an app that
 * needs replay protection beyond the proof's {@code iat} window brings its own. A PIN-change-only
 * ({@code pcr}) token is refused: it is valid on the platform's {@code /field/auth/pin} only.
 * Access tokens must be RS256 and proofs ES256; any other algorithm is refused.
 *
 * <p>Thread-safe. Signing keys are fetched from the issuer's JWKS, cached, and refetched at
 * most once a minute when a token names an unknown {@code kid} (and every
 * {@code jwksRefreshInterval} regardless).
 */
public final class FieldTokenVerifier {

    private static final Base64.Decoder B64 = Base64.getUrlDecoder();
    private static final Base64.Encoder B64E = Base64.getUrlEncoder().withoutPadding();
    private static final Duration UNKNOWN_KID_REFETCH = Duration.ofMinutes(1);

    private final ObjectMapper mapper = Json.newMapper();
    private final String issuer;
    private final String appCode;
    private final String configuredJwksUrl;
    private final Function<String, String> fetcher;
    private final Duration jwksRefreshInterval;
    private final Duration clockSkew;
    private final Duration proofMaxAge;
    private final Clock clock;

    private final Object keyLock = new Object();
    private String jwksUrl; // guarded by keyLock
    private List<Jwk> keys = List.of(); // guarded by keyLock
    private Instant fetchedAt; // guarded by keyLock; null until the first successful fetch

    private record Jwk(String kid, PublicKey key) {}

    /** Thrown inside the verifier when the JWKS cannot be obtained. */
    private static final class KeysUnavailable extends Exception {
        private static final long serialVersionUID = 1L;

        KeysUnavailable(String message) {
            super(message);
        }
    }

    private FieldTokenVerifier(Builder b) {
        this.issuer = Objects.requireNonNull(b.issuer, "issuer is required").replaceAll("/+$", "");
        this.appCode = Objects.requireNonNull(b.appCode, "appCode is required");
        this.configuredJwksUrl = b.jwksUrl;
        this.jwksRefreshInterval = b.jwksRefreshInterval;
        this.clockSkew = b.clockSkew;
        this.proofMaxAge = b.proofMaxAge;
        this.clock = b.clock;
        this.fetcher = b.fetcher != null ? b.fetcher : defaultFetcher(b.httpClient);
    }

    public static Builder builder() {
        return new Builder();
    }

    /** Configuration. {@code issuer} and {@code appCode} are required. */
    public static final class Builder {
        private String issuer;
        private String appCode;
        private String jwksUrl;
        private HttpClient httpClient;
        private Function<String, String> fetcher;
        private Duration jwksRefreshInterval = Duration.ofHours(1);
        private Duration clockSkew = Duration.ofSeconds(60);
        private Duration proofMaxAge = Duration.ofMinutes(5);
        private Clock clock = Clock.systemUTC();

        /** The FlowCatalyst base URL and the tokens' {@code iss} claim. */
        public Builder issuer(String issuer) {
            this.issuer = issuer;
            return this;
        }

        /** This application's code; tokens must be addressed to it ({@code aud}). */
        public Builder appCode(String appCode) {
            this.appCode = appCode;
            return this;
        }

        /** Skips OIDC discovery and fetches keys from this URL. */
        public Builder jwksUrl(String jwksUrl) {
            this.jwksUrl = jwksUrl;
            return this;
        }

        /** The HTTP client for discovery and JWKS (default: a 15 s timeout client). */
        public Builder httpClient(HttpClient httpClient) {
            this.httpClient = httpClient;
            return this;
        }

        /**
         * Replaces HTTP altogether: given a URL, return the document body or throw. Discovery
         * and JWKS both go through it. For tests and unusual networks.
         */
        public Builder fetcher(Function<String, String> fetcher) {
            this.fetcher = fetcher;
            return this;
        }

        /** How long fetched keys are trusted before a refetch (default 1 hour). */
        public Builder jwksRefreshInterval(Duration d) {
            this.jwksRefreshInterval = d;
            return this;
        }

        /** Tolerance for the token's exp/nbf/iat (default 60 s). */
        public Builder clockSkew(Duration d) {
            this.clockSkew = d;
            return this;
        }

        /**
         * How far a proof's {@code iat} may be from now, either way (default 5 minutes). Widen
         * it for devices with bad clocks.
         */
        public Builder proofMaxAge(Duration d) {
            this.proofMaxAge = d;
            return this;
        }

        /** Overrides the clock (tests, fixtures). */
        public Builder clock(Clock clock) {
            this.clock = clock;
            return this;
        }

        public FieldTokenVerifier build() {
            return new FieldTokenVerifier(this);
        }
    }

    private static Function<String, String> defaultFetcher(HttpClient supplied) {
        HttpClient http = supplied != null
                ? supplied
                : HttpClient.newBuilder().connectTimeout(Duration.ofSeconds(15)).build();
        return url -> {
            try {
                HttpRequest req = HttpRequest.newBuilder(URI.create(url))
                        .timeout(Duration.ofSeconds(15))
                        .header("Accept", "application/json")
                        .GET()
                        .build();
                HttpResponse<String> resp = http.send(req, HttpResponse.BodyHandlers.ofString());
                if (resp.statusCode() < 200 || resp.statusCode() >= 300) {
                    throw new IllegalStateException("GET " + url + ": HTTP " + resp.statusCode());
                }
                return resp.body();
            } catch (IOException e) {
                throw new IllegalStateException("GET " + url + ": " + e.getMessage(), e);
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                throw new IllegalStateException("GET " + url + ": interrupted", e);
            }
        };
    }

    // ── verification ────────────────────────────────────────────────

    /**
     * Checks a request's credentials.
     *
     * @param authorizationHeader the {@code Authorization} header value ({@code DPoP <token>})
     * @param dpopHeader the {@code DPoP} proof header value
     * @param method the request's HTTP method
     * @param requestUrl the request's full public URL (scheme, host, path; any query is ignored)
     */
    public FieldVerification verify(
            String authorizationHeader, String dpopHeader, String method, String requestUrl) {
        String auth = authorizationHeader == null ? "" : authorizationHeader.strip();
        int sp = auth.indexOf(' ');
        String scheme = sp < 0 ? auth : auth.substring(0, sp);
        String token = sp < 0 ? "" : auth.substring(sp + 1).strip();
        if (!scheme.equalsIgnoreCase("DPoP") || token.isEmpty()) {
            return invalid(FieldReason.MALFORMED, "Authorization must be \"DPoP <token>\"");
        }
        String[] parts = token.split("\\.", -1);
        if (parts.length != 3) {
            return invalid(FieldReason.MALFORMED, "not a compact JWS");
        }
        JsonNode header;
        JsonNode claims;
        byte[] signature;
        try {
            header = mapper.readTree(B64.decode(parts[0]));
            claims = mapper.readTree(B64.decode(parts[1]));
            signature = B64.decode(parts[2]);
        } catch (RuntimeException e) {
            return invalid(FieldReason.MALFORMED, "not a compact JWS");
        }
        if (!header.isObject() || !claims.isObject()) {
            return invalid(FieldReason.MALFORMED, "not a compact JWS");
        }
        String typ = text(header, "typ");
        if (!"fc-field+jwt".equals(typ)) {
            return invalid(FieldReason.WRONG_TYP, "typ is " + typ);
        }
        // The platform signs field access tokens RS256 only; refuse any other alg (HS256,
        // none, ES256, ...) before touching the keys.
        String alg = text(header, "alg");
        if (!"RS256".equals(alg)) {
            return invalid(FieldReason.INVALID_TOKEN, "alg must be RS256, got " + alg);
        }

        byte[] signingInput = (parts[0] + "." + parts[1]).getBytes(StandardCharsets.US_ASCII);
        String kid = header.hasNonNull("kid") ? text(header, "kid") : null;
        List<PublicKey> candidates;
        try {
            candidates = candidateKeys(kid);
        } catch (KeysUnavailable e) {
            return new FieldVerification.Unavailable(e.getMessage());
        }
        boolean verified = false;
        for (PublicKey k : candidates) {
            if (rs256(k, signingInput, signature)) {
                verified = true;
                break;
            }
        }
        if (!verified) {
            return invalid(FieldReason.INVALID_TOKEN, "signature does not verify");
        }

        Instant now = clock.instant();
        if (!issuer.equals(text(claims, "iss"))) {
            return invalid(FieldReason.INVALID_TOKEN, "iss is " + text(claims, "iss"));
        }
        Long exp = epoch(claims, "exp");
        if (exp == null) {
            // The platform always sets exp; a token without one is never accepted.
            return invalid(FieldReason.INVALID_TOKEN, "exp is missing");
        }
        Long iat = epoch(claims, "iat");
        if (iat != null && Instant.ofEpochSecond(iat).isAfter(now.plus(clockSkew))) {
            return invalid(FieldReason.INVALID_TOKEN, "iat is in the future");
        }
        if (now.minus(clockSkew).isAfter(Instant.ofEpochSecond(exp))) {
            return invalid(FieldReason.EXPIRED, "");
        }
        Long nbf = epoch(claims, "nbf");
        if (nbf != null && Instant.ofEpochSecond(nbf).isAfter(now.plus(clockSkew))) {
            return invalid(FieldReason.INVALID_TOKEN, "token is not valid yet");
        }

        String tokenUse = text(claims, "token_use");
        if (!"field".equals(tokenUse)) {
            return invalid(FieldReason.WRONG_TOKEN_USE, "token_use is " + tokenUse);
        }
        String aud = audience(claims.get("aud"));
        if (!appCode.equals(aud)) {
            return invalid(FieldReason.WRONG_AUD, "aud is " + aud);
        }
        if (claims.path("pcr").asBoolean(false)) {
            return invalid(FieldReason.PCR, "the token may only change the PIN");
        }
        String cnfJkt = text(claims.path("cnf"), "jkt");
        if (cnfJkt.isEmpty()) {
            return invalid(FieldReason.JKT_MISMATCH, "the token carries no cnf.jkt");
        }
        if (dpopHeader == null || dpopHeader.isBlank()) {
            return invalid(FieldReason.MISSING_PROOF, "");
        }
        Proof proof;
        try {
            proof = parseProof(dpopHeader);
        } catch (ProofError e) {
            return invalid(FieldReason.INVALID_PROOF, e.getMessage());
        }
        if (!proof.jkt.equals(cnfJkt)) {
            return invalid(FieldReason.JKT_MISMATCH, "");
        }
        if (!proof.htm.equals(method)) {
            return invalid(FieldReason.HTM_MISMATCH, "");
        }
        if (!sameUrl(proof.htu, requestUrl)) {
            return invalid(FieldReason.HTU_MISMATCH, "");
        }
        if (!MessageDigest.isEqual(
                proof.ath.getBytes(StandardCharsets.UTF_8),
                B64E.encode(sha256(token.getBytes(StandardCharsets.UTF_8))))) {
            return invalid(FieldReason.ATH_MISMATCH, "");
        }
        Duration age = Duration.between(Instant.ofEpochSecond(proof.iat), now);
        if (age.compareTo(proofMaxAge) > 0 || age.compareTo(proofMaxAge.negated()) < 0) {
            return invalid(FieldReason.STALE_PROOF, "");
        }

        List<String> amr = new ArrayList<>();
        claims.path("amr").forEach(n -> amr.add(n.asString()));
        Long authTime = epoch(claims, "auth_time");
        JsonNode act = claims.get("act");
        FieldActor actor = act != null && act.isObject() && !text(act, "sub").isEmpty()
                ? new FieldActor(text(act, "sub"), text(act, "typ"), text(act, "name"))
                : null;
        boolean test = claims.path("test").isBoolean() && claims.path("test").asBoolean();
        return new FieldVerification.Valid(new FieldPrincipal(
                text(claims, "sub"),
                text(claims, "client_id"),
                text(claims, "client_identifier"),
                text(claims, "worker_code"),
                text(claims, "name"),
                aud,
                text(claims, "device_id"),
                text(claims, "device_mode"),
                text(claims, "sid"),
                List.copyOf(amr),
                Instant.ofEpochSecond(authTime == null ? 0 : authTime),
                Instant.ofEpochSecond(exp),
                proof.jkt,
                text(claims, "jti"),
                actor,
                test,
                text(claims, "linked_principal_id")));
    }

    private static FieldVerification invalid(String reason, String detail) {
        return new FieldVerification.Invalid(reason, detail);
    }

    private static String text(JsonNode node, String field) {
        JsonNode v = node.get(field);
        return v == null || !v.isString() ? "" : v.asString();
    }

    private static Long epoch(JsonNode node, String field) {
        JsonNode v = node.get(field);
        return v != null && v.isNumber() ? v.asLong() : null;
    }

    /** A string, or the first entry of an array of strings (as the Go verifier does). */
    private static String audience(JsonNode aud) {
        if (aud == null) return "";
        if (aud.isString()) return aud.asString();
        if (aud.isArray() && aud.size() > 0 && aud.get(0).isString()) return aud.get(0).asString();
        return "";
    }

    private static boolean rs256(PublicKey key, byte[] input, byte[] sig) {
        try {
            Signature s = Signature.getInstance("SHA256withRSA");
            s.initVerify(key);
            s.update(input);
            return s.verify(sig);
        } catch (GeneralSecurityException e) {
            return false;
        }
    }

    private static byte[] sha256(byte[] in) {
        try {
            return MessageDigest.getInstance("SHA-256").digest(in);
        } catch (GeneralSecurityException e) {
            throw new IllegalStateException(e);
        }
    }

    // ── the proof ───────────────────────────────────────────────────

    private record Proof(String jkt, String htm, String htu, String ath, long iat) {}

    private static final class ProofError extends Exception {
        private static final long serialVersionUID = 1L;

        ProofError(String message) {
            super(message);
        }
    }

    private Proof parseProof(String compact) throws ProofError {
        String[] parts = compact.strip().split("\\.", -1);
        if (parts.length != 3) throw new ProofError("not a compact JWS");
        JsonNode hdr;
        try {
            hdr = mapper.readTree(B64.decode(parts[0]));
        } catch (RuntimeException e) {
            throw new ProofError("header is not base64url JSON");
        }
        if (!"dpop+jwt".equals(text(hdr, "typ")) || !"ES256".equals(text(hdr, "alg"))) {
            throw new ProofError("typ must be dpop+jwt and alg ES256");
        }
        JsonNode jwk = hdr.path("jwk");
        if (!"EC".equals(text(jwk, "kty")) || !"P-256".equals(text(jwk, "crv")) || jwk.has("d")) {
            throw new ProofError("jwk must be a public EC P-256 key");
        }
        byte[] x;
        byte[] y;
        try {
            x = B64.decode(text(jwk, "x"));
            y = B64.decode(text(jwk, "y"));
        } catch (IllegalArgumentException e) {
            throw new ProofError("jwk coordinates are malformed");
        }
        if (x.length != 32 || y.length != 32) throw new ProofError("jwk coordinates are malformed");
        PublicKey pub = Es256.publicKey(x, y);
        if (pub == null) throw new ProofError("jwk point is not on the curve");
        byte[] sig;
        try {
            sig = B64.decode(parts[2]);
        } catch (IllegalArgumentException e) {
            throw new ProofError("signature is malformed");
        }
        if (sig.length != 64) throw new ProofError("signature is malformed");
        if (!Es256.verify(pub, (parts[0] + "." + parts[1]).getBytes(StandardCharsets.US_ASCII), sig)) {
            throw new ProofError("signature does not verify");
        }
        JsonNode c;
        try {
            c = mapper.readTree(B64.decode(parts[1]));
        } catch (RuntimeException e) {
            throw new ProofError("claims are not base64url JSON");
        }
        JsonNode iat = c.get("iat");
        if (iat == null || !iat.isIntegralNumber() || text(c, "jti").isEmpty()) {
            throw new ProofError("claims need iat and jti");
        }
        // RFC 7638: required members only, lexicographic order, no whitespace.
        String canonical = "{\"crv\":\"P-256\",\"kty\":\"EC\",\"x\":\"" + B64E.encodeToString(x)
                + "\",\"y\":\"" + B64E.encodeToString(y) + "\"}";
        String jkt = B64E.encodeToString(sha256(canonical.getBytes(StandardCharsets.UTF_8)));
        return new Proof(jkt, text(c, "htm"), text(c, "htu"), text(c, "ath"), iat.asLong());
    }

    /** Scheme and host (case-insensitive, port included) and decoded path; query and fragment ignored. */
    static boolean sameUrl(String a, String b) {
        try {
            URI ua = new URI(a);
            URI ub = new URI(b);
            return ua.getScheme() != null
                    && ub.getScheme() != null
                    && ua.getScheme().equalsIgnoreCase(ub.getScheme())
                    && hostPort(ua).equalsIgnoreCase(hostPort(ub))
                    && java.util.Objects.equals(ua.getPath(), ub.getPath());
        } catch (java.net.URISyntaxException | RuntimeException e) {
            return false;
        }
    }

    private static String hostPort(URI u) {
        if (u.getHost() == null) {
            return u.getAuthority() == null ? "" : u.getAuthority();
        }
        return u.getPort() < 0 ? u.getHost() : u.getHost() + ":" + u.getPort();
    }

    // ── keys ────────────────────────────────────────────────────────

    /**
     * The keys that may have signed a token with this {@code kid}. A named kid selects that key
     * (refetching at most once a minute when it is unknown); no kid tries every key.
     */
    private List<PublicKey> candidateKeys(String kid) throws KeysUnavailable {
        synchronized (keyLock) {
            Instant now = clock.instant();
            if (fetchedAt == null || now.isAfter(fetchedAt.plus(jwksRefreshInterval))) {
                refresh(now, fetchedAt != null);
            }
            List<PublicKey> match = select(kid);
            if (match.isEmpty() && kid != null && now.isAfter(fetchedAt.plus(UNKNOWN_KID_REFETCH))) {
                refresh(now, true);
                match = select(kid);
            }
            return match;
        }
    }

    private List<PublicKey> select(String kid) {
        List<PublicKey> out = new ArrayList<>();
        for (Jwk k : keys) {
            if (kid == null || kid.equals(k.kid)) out.add(k.key);
        }
        return out;
    }

    /**
     * Fetches the key set. With {@code keepStale}, a failure keeps the keys already held (the
     * attempt counts, so a down platform is not hammered); without any keys it is Unavailable.
     */
    private void refresh(Instant now, boolean keepStale) throws KeysUnavailable {
        try {
            if (jwksUrl == null) {
                jwksUrl = configuredJwksUrl != null ? configuredJwksUrl : discover();
            }
            keys = parseJwks(fetcher.apply(jwksUrl));
            fetchedAt = now;
        } catch (KeysUnavailable e) {
            if (!keepStale) throw e;
            fetchedAt = now;
        } catch (RuntimeException e) {
            if (!keepStale) throw new KeysUnavailable("fetch JWKS: " + e.getMessage());
            fetchedAt = now;
        }
    }

    private String discover() throws KeysUnavailable {
        String url = issuer + "/.well-known/openid-configuration";
        try {
            JsonNode doc = mapper.readTree(fetcher.apply(url));
            String uri = text(doc, "jwks_uri");
            if (uri.isEmpty()) throw new KeysUnavailable("discovery doc missing jwks_uri");
            return uri;
        } catch (KeysUnavailable e) {
            throw e;
        } catch (RuntimeException e) {
            throw new KeysUnavailable("discovery " + url + ": " + e.getMessage());
        }
    }

    private List<Jwk> parseJwks(String body) throws KeysUnavailable {
        List<Jwk> out = new ArrayList<>();
        try {
            for (JsonNode k : mapper.readTree(body).path("keys")) {
                if (!"RSA".equals(text(k, "kty"))) continue;
                String use = text(k, "use");
                String alg = text(k, "alg");
                if ((!use.isEmpty() && !use.equals("sig")) || (!alg.isEmpty() && !alg.equals("RS256"))) {
                    continue;
                }
                BigInteger n = new BigInteger(1, B64.decode(text(k, "n")));
                BigInteger e = new BigInteger(1, B64.decode(text(k, "e")));
                PublicKey pk = KeyFactory.getInstance("RSA").generatePublic(new RSAPublicKeySpec(n, e));
                out.add(new Jwk(text(k, "kid"), pk));
            }
        } catch (GeneralSecurityException | RuntimeException e) {
            throw new KeysUnavailable("parse JWKS: " + e.getMessage());
        }
        if (out.isEmpty()) throw new KeysUnavailable("JWKS holds no RS256 signing keys");
        return List.copyOf(out);
    }
}
