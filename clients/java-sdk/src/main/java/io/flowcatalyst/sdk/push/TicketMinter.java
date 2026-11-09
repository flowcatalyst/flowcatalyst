package io.flowcatalyst.sdk.push;

import io.flowcatalyst.sdk.http.Json;
import io.flowcatalyst.sdk.push.TicketKey.PublicJwk;
import java.nio.charset.StandardCharsets;
import java.time.Instant;
import java.util.ArrayList;
import java.util.Base64;
import java.util.HashSet;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.regex.Pattern;
import tools.jackson.core.JacksonException;
import tools.jackson.databind.ObjectMapper;

/**
 * Push tickets: short-lived grants to listen on topics, minted by the application after its own
 * access check (push contract section 6.4; docs/push-gateway-spec.md section 9).
 *
 * <p>A ticket is a JWS compact token, {@code alg} {@code EdDSA} (Ed25519), header {@code kid}. The
 * gateway verifies it with the public key the application synced ({@link #jwks()} through
 * {@code DefinitionSet#withPush}). A minter mints for one application with its current key, and
 * exports the JWKS (the current key then the previous ones during a rotation) that sync sends.
 *
 * <pre>{@code
 * var minter = TicketMinter.fromConfig("shop", System.getenv("PUSH_TICKET_KEY"), List.of());
 * String ticket = minter.mint(TicketMinter.Request.of("acme", List.of("shop:orders:{id}"))
 *         .withSub("prn_1"));
 * }</pre>
 */
public final class TicketMinter {

    /** The audience every ticket carries. */
    public static final String AUDIENCE = "fw-push";
    /** A ticket's {@code exp} is at most this many seconds after its {@code iat}. */
    public static final int MAX_TTL_SECONDS = 300;
    /** The default lifetime of a minted ticket. */
    public static final int DEFAULT_TTL_SECONDS = 60;
    /** A ticket names at most this many topic patterns. */
    public static final int MAX_TOPICS = 100;

    /** A JWKS of ticket keys: what sync sends and {@code GET /push/keys} answers. */
    public record Jwks(List<PublicJwk> keys) {
        public Jwks {
            keys = List.copyOf(keys);
        }
    }

    /**
     * What a ticket grants. Immutable; every {@code with*} returns a new instance.
     *
     * @param client the stream's client ({@code _platform} for platform-wide)
     * @param topics topic patterns of the application (placeholders {@code {x}} match one
     *     segment), at most 100
     * @param sub the principal, recorded in the stream token for logs (null: none)
     * @param jkt bind the ticket to a DPoP key: the subscribe must prove it ({@code cnf.jkt})
     * @param ttlSeconds lifetime in seconds, 1 to 300 (null: 60)
     * @param streamExp the latest the stream may live (null: the ticket's {@code exp} plus the
     *     gateway's maximum lifetime)
     * @param now issued-at (null: the minter's clock); for tests
     */
    public record Request(
            String client, List<String> topics, String sub, String jkt, Integer ttlSeconds,
            Instant streamExp, Instant now) {

        public Request {
            topics = List.copyOf(topics);
        }

        public static Request of(String client, List<String> topics) {
            return new Request(client, topics, null, null, null, null, null);
        }

        public Request withSub(String sub) {
            return new Request(client, topics, sub, jkt, ttlSeconds, streamExp, now);
        }

        public Request withJkt(String jkt) {
            return new Request(client, topics, sub, jkt, ttlSeconds, streamExp, now);
        }

        public Request withTtlSeconds(int ttlSeconds) {
            return new Request(client, topics, sub, jkt, ttlSeconds, streamExp, now);
        }

        public Request withStreamExp(Instant streamExp) {
            return new Request(client, topics, sub, jkt, ttlSeconds, streamExp, now);
        }

        public Request withNow(Instant now) {
            return new Request(client, topics, sub, jkt, ttlSeconds, streamExp, now);
        }
    }

    private static final Pattern APPLICATION = Pattern.compile("[a-z][a-z0-9-]*");
    private static final Pattern CLIENT = Pattern.compile("[A-Za-z0-9._-]{1,100}");
    private static final ObjectMapper MAPPER = Json.newMapper();
    private static final Base64.Encoder B64U = Base64.getUrlEncoder().withoutPadding();

    private final String applicationCode;
    private final TicketKey current;
    private final List<PublicJwk> previous;

    /**
     * @param applicationCode the {@code iss} of every ticket; each topic's first segment must
     *     equal it
     * @param current the key tickets are signed with
     * @param previous the keys of a rotation in progress: published in the JWKS so tickets signed
     *     with them keep verifying, never used to sign (at most 3)
     */
    public TicketMinter(String applicationCode, TicketKey current, List<PublicJwk> previous) {
        if (applicationCode == null || !APPLICATION.matcher(applicationCode).matches()) {
            throw new TicketException("not an application code: \"" + applicationCode + "\"");
        }
        if (previous.size() > 3) {
            throw new TicketException("at most 4 ticket keys (the current and 3 previous)");
        }
        this.applicationCode = applicationCode;
        this.current = current;
        this.previous = List.copyOf(previous);
    }

    public TicketMinter(String applicationCode, TicketKey current) {
        this(applicationCode, current, List.of());
    }

    /**
     * From configuration: the current key as {@code kid:base64url-seed}, and the previous ones
     * (during a rotation) as {@code kid:base64url-seed} strings, for example split from a
     * comma-separated setting. Blank entries are ignored.
     */
    public static TicketMinter fromConfig(String applicationCode, String current, List<String> previous) {
        List<PublicJwk> prev = new ArrayList<>();
        for (String p : previous) {
            if (p != null && !p.isBlank()) {
                prev.add(TicketKey.parse(p).toPublicJwk());
            }
        }
        return new TicketMinter(applicationCode, TicketKey.parse(current), prev);
    }

    public String applicationCode() {
        return applicationCode;
    }

    /** The JWKS to sync: the current key, then the previous ones. */
    public Jwks jwks() {
        List<PublicJwk> keys = new ArrayList<>();
        keys.add(current.toPublicJwk());
        keys.addAll(previous);
        Set<String> kids = new HashSet<>();
        for (PublicJwk k : keys) {
            kids.add(k.kid());
        }
        if (kids.size() != keys.size()) {
            throw new TicketException("duplicate ticket key ids");
        }
        return new Jwks(keys);
    }

    /** Mint a ticket granting {@code client} the topics, with the default lifetime. */
    public String mint(String client, List<String> topics) {
        return mint(Request.of(client, topics));
    }

    /**
     * Mint a ticket. Claims, in order: {@code iss}, {@code aud}, {@code client}, {@code topics},
     * {@code sub}, {@code cnf}, {@code iat}, {@code exp}, {@code stream-exp}.
     *
     * @throws TicketException when the request cannot be a valid ticket
     */
    public String mint(Request request) {
        if (request.client() == null || !CLIENT.matcher(request.client()).matches()) {
            throw new TicketException("client is [A-Za-z0-9._-]{1,100}");
        }
        List<String> topics = request.topics();
        if (topics.isEmpty() || topics.size() > MAX_TOPICS) {
            throw new TicketException("a ticket names 1 to " + MAX_TOPICS + " topics");
        }
        for (String t : topics) {
            if (t == null || !t.contains(":") || !t.substring(0, t.indexOf(':')).equals(applicationCode)) {
                throw new TicketException(
                        "the topic \"" + t + "\" is not a topic of the application \"" + applicationCode + "\"");
            }
        }
        int ttl = request.ttlSeconds() == null ? DEFAULT_TTL_SECONDS : request.ttlSeconds();
        if (ttl < 1 || ttl > MAX_TTL_SECONDS) {
            throw new TicketException("ttlSeconds is 1 to " + MAX_TTL_SECONDS);
        }
        long iat = (request.now() == null ? Instant.now() : request.now()).getEpochSecond();

        Map<String, Object> claims = new LinkedHashMap<>();
        claims.put("iss", applicationCode);
        claims.put("aud", AUDIENCE);
        claims.put("client", request.client());
        claims.put("topics", topics);
        if (request.sub() != null && !request.sub().isEmpty()) {
            claims.put("sub", request.sub());
        }
        if (request.jkt() != null && !request.jkt().isEmpty()) {
            claims.put("cnf", Map.of("jkt", request.jkt()));
        }
        claims.put("iat", iat);
        claims.put("exp", iat + ttl);
        if (request.streamExp() != null) {
            claims.put("stream-exp", request.streamExp().getEpochSecond());
        }

        Map<String, Object> header = new LinkedHashMap<>();
        header.put("alg", "EdDSA");
        header.put("kid", current.kid());
        try {
            String signingInput = b64(MAPPER.writeValueAsString(header)) + "." + b64(MAPPER.writeValueAsString(claims));
            byte[] signature = current.sign(signingInput.getBytes(StandardCharsets.US_ASCII));
            return signingInput + "." + B64U.encodeToString(signature);
        } catch (JacksonException e) {
            throw new TicketException("could not encode the ticket: " + e.getOriginalMessage());
        }
    }

    private static String b64(String json) {
        return B64U.encodeToString(json.getBytes(StandardCharsets.UTF_8));
    }
}
