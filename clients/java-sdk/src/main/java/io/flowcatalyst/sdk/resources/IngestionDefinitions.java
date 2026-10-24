package io.flowcatalyst.sdk.resources;

import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonSubTypes;
import com.fasterxml.jackson.annotation.JsonTypeInfo;
import io.flowcatalyst.sdk.generated.model.Target;
import io.flowcatalyst.sdk.http.Json;
import java.time.Duration;
import java.util.List;
import java.util.Map;
import tools.jackson.databind.ObjectMapper;

/**
 * Typed forms of an ingestion definition's {@code source} and {@code split}, which the OpenAPI
 * document leaves as open objects tagged by {@code kind}. Pass a {@link Source} or {@link Split}
 * where a request takes the {@code source} or {@code split}; read a response's back with
 * {@link #sourceOf} and {@link #splitOf}.
 *
 * <p>A secret is always a {@link SecretRef}, the secret's <em>name</em>; no value is ever held. A
 * kind a runtime extension supplies (and {@code camel-route}) is a {@link Source.Custom}: its
 * {@code type} and open {@code settings}.
 */
public final class IngestionDefinitions {

    private static final ObjectMapper MAPPER = Json.newMapper();

    private IngestionDefinitions() {}

    /** How an ingestion's target delivers: what a failed record does to the ones behind it. */
    public enum Mode {
        IMMEDIATE,
        NEXT_ON_ERROR,
        BLOCK_ON_ERROR
    }

    /** The target of a definition; set {@code connection} or {@code serviceAccount} on the result if needed (not both). */
    public static Target target(String code, String targetUrl, String pool, Mode mode, long batchSize) {
        return new Target().code(code).targetUrl(targetUrl).pool(pool).mode(mode.name()).batchSize(batchSize);
    }

    /** A response's {@code source} (an open object) as the typed form. */
    public static Source sourceOf(Object wire) {
        return MAPPER.convertValue(wire, Source.class);
    }

    /** A response's {@code split} (an open object) as the typed form. */
    public static Split splitOf(Object wire) {
        return MAPPER.convertValue(wire, Split.class);
    }

    /**
     * The {@code kind} a union member is written with, from its union's {@code @JsonSubTypes}. Written
     * as a property of the member itself because a request holds {@code source} and {@code split} as
     * open objects, where Jackson adds no type tag.
     */
    private static String kindOf(Class<?> union, Class<?> member) {
        for (JsonSubTypes.Type t : union.getAnnotation(JsonSubTypes.class).value()) {
            if (t.value() == member) return t.name();
        }
        throw new IllegalStateException(member + " is not a member of " + union);
    }

    /** The name of a secret, resolved by the runtime against the ingestion's client. */
    public record SecretRef(String name) {}

    public record AwsCredential(String accessKeyId, SecretRef secretAccessKey) {}

    /** At most {@code requests} per {@code per}, counted per runtime. */
    public record RateLimit(int requests, Duration per) {}

    public record Header(String name, String value) {}

    /** What in an answer moves the next request on: {@code from} is a path such as {@code $.next}. */
    public record Cursor(String from, String param, String initial) {}

    // ── Source ──────────────────────────────────────────────────────

    @JsonTypeInfo(use = JsonTypeInfo.Id.NAME, include = JsonTypeInfo.As.EXISTING_PROPERTY, property = "kind")
    @JsonSubTypes({
        @JsonSubTypes.Type(value = Source.Http.class, name = "http"),
        @JsonSubTypes.Type(value = Source.Sftp.class, name = "sftp"),
        @JsonSubTypes.Type(value = Source.S3.class, name = "s3"),
        @JsonSubTypes.Type(value = Source.Sqs.class, name = "sqs"),
        @JsonSubTypes.Type(value = Source.RabbitMq.class, name = "rabbitmq"),
        @JsonSubTypes.Type(value = Source.DatabasePoll.class, name = "database-poll"),
        @JsonSubTypes.Type(value = Source.ApiPoll.class, name = "api-poll"),
        @JsonSubTypes.Type(value = Source.Custom.class, name = "custom")
    })
    public sealed interface Source {
        @JsonProperty("kind")
        default String kind() {
            return kindOf(Source.class, getClass());
        }


        /** JSON posted to the endpoint the runtime exposes; {@code rateLimit} may be null. */
        record Http(HttpAuth auth, RateLimit rateLimit) implements Source {}

        /** Files in a directory; {@code knownHosts} and {@code include} may be null. */
        record Sftp(
                String host,
                int port,
                String directory,
                String username,
                SecretRef credential,
                String knownHosts,
                String include,
                Duration every)
                implements Source {}

        /** Objects under a prefix; {@code prefix} and {@code credential} may be null (the runtime's own identity). */
        record S3(String region, String bucket, String prefix, Duration every, AwsCredential credential)
                implements Source {}

        /** A queue by name, not URL; {@code credential} may be null. */
        record Sqs(String region, String queue, AwsCredential credential) implements Source {}

        record RabbitMq(String uri, String queue, String username, SecretRef password) implements Source {}

        record DatabasePoll(
                String jdbcUrl,
                String username,
                SecretRef password,
                String table,
                Track track,
                int maxRows,
                Duration every)
                implements Source {}

        /** One request per poll; {@code headers}, {@code body} and {@code cursor} may be null. */
        record ApiPoll(
                String url,
                String method,
                List<Header> headers,
                String body,
                ApiAuth auth,
                Results results,
                Cursor cursor,
                Duration every,
                Duration idleDelay)
                implements Source {}

        /** A kind a runtime extension supplies; the platform stores {@code settings} and reads nothing in them. */
        record Custom(String type, Map<String, Object> settings) implements Source {}
    }

    // ── Auth, tracking and results inside a source ──────────────────

    /** How the caller of an http source authenticates. */
    @JsonTypeInfo(use = JsonTypeInfo.Id.NAME, include = JsonTypeInfo.As.EXISTING_PROPERTY, property = "kind")
    @JsonSubTypes({
        @JsonSubTypes.Type(value = HttpAuth.FlowCatalystToken.class, name = "flowcatalyst-token"),
        @JsonSubTypes.Type(value = HttpAuth.Basic.class, name = "basic"),
        @JsonSubTypes.Type(value = HttpAuth.Bearer.class, name = "bearer")
    })
    public sealed interface HttpAuth {
        @JsonProperty("kind")
        default String kind() {
            return kindOf(HttpAuth.class, getClass());
        }

        /** The caller's own platform token; {@code principalId} may be null. */
        record FlowCatalystToken(String application, String principalId) implements HttpAuth {}

        record Basic(String username, SecretRef password) implements HttpAuth {}

        record Bearer(SecretRef token) implements HttpAuth {}
    }

    /** How a database poll tells rows read from rows not yet read. */
    @JsonTypeInfo(use = JsonTypeInfo.Id.NAME, include = JsonTypeInfo.As.EXISTING_PROPERTY, property = "kind")
    @JsonSubTypes({
        @JsonSubTypes.Type(value = Track.Watermark.class, name = "watermark"),
        @JsonSubTypes.Type(value = Track.Status.class, name = "status")
    })
    public sealed interface Track {
        @JsonProperty("kind")
        default String kind() {
            return kindOf(Track.class, getClass());
        }

        /** A column that only grows; {@code type} is NUMBER or TIMESTAMP. */
        record Watermark(String column, String type) implements Track {}

        /** Rows whose status column holds {@code pending}, set to {@code processed} once read. */
        record Status(String column, String keyColumn, String pending, String processed) implements Track {}
    }

    /** How an api-poll logs in to the API. */
    @JsonTypeInfo(use = JsonTypeInfo.Id.NAME, include = JsonTypeInfo.As.EXISTING_PROPERTY, property = "kind")
    @JsonSubTypes({
        @JsonSubTypes.Type(value = ApiAuth.None.class, name = "none"),
        @JsonSubTypes.Type(value = ApiAuth.Basic.class, name = "basic"),
        @JsonSubTypes.Type(value = ApiAuth.Bearer.class, name = "bearer"),
        @JsonSubTypes.Type(value = ApiAuth.ApiKey.class, name = "api-key"),
        @JsonSubTypes.Type(value = ApiAuth.OAuthClientCredentials.class, name = "oauth-client-credentials"),
        @JsonSubTypes.Type(value = ApiAuth.Login.class, name = "login")
    })
    public sealed interface ApiAuth {
        @JsonProperty("kind")
        default String kind() {
            return kindOf(ApiAuth.class, getClass());
        }

        record None() implements ApiAuth {}

        record Basic(String username, SecretRef password) implements ApiAuth {}

        record Bearer(SecretRef token) implements ApiAuth {}

        /** A secret sent in a header. */
        record ApiKey(String header, SecretRef value) implements ApiAuth {}

        /** {@code credentialsIn} is BASIC_HEADER or BODY; {@code scope} and {@code audience} may be null. */
        record OAuthClientCredentials(
                String tokenUrl,
                String clientId,
                SecretRef clientSecret,
                String scope,
                String audience,
                String credentialsIn)
                implements ApiAuth {}

        /** {@code format} is FORM or JSON; {@code extraFields} and {@code expiresInPath} may be null. */
        record Login(
                String url,
                String format,
                String usernameField,
                String passwordField,
                String username,
                SecretRef password,
                Map<String, String> extraFields,
                String tokenPath,
                String expiresInPath,
                Duration lifetime,
                String header,
                String prefix)
                implements ApiAuth {}
    }

    /** How an api-poll's answer is told to have results. */
    @JsonTypeInfo(use = JsonTypeInfo.Id.NAME, include = JsonTypeInfo.As.EXISTING_PROPERTY, property = "kind")
    @JsonSubTypes({
        @JsonSubTypes.Type(value = Results.Records.class, name = "records"),
        @JsonSubTypes.Type(value = Results.JsonPath.class, name = "json-path"),
        @JsonSubTypes.Type(value = Results.HeaderPresent.class, name = "header")
    })
    public sealed interface Results {
        @JsonProperty("kind")
        default String kind() {
            return kindOf(Results.class, getClass());
        }

        /** The split makes at least one record of the answer. */
        record Records() implements Results {}

        /** The path ({@code $.items}) holds a non-empty array or a value that is not null, false or empty. */
        record JsonPath(String path) implements Results {}

        /** The answer carries the header, optionally with the value {@code equals}. */
        record HeaderPresent(String name, String equals) implements Results {}
    }

    // ── Split ───────────────────────────────────────────────────────

    /** How a payload becomes records. */
    @JsonTypeInfo(use = JsonTypeInfo.Id.NAME, include = JsonTypeInfo.As.EXISTING_PROPERTY, property = "kind")
    @JsonSubTypes({
        @JsonSubTypes.Type(value = Split.None.class, name = "none"),
        @JsonSubTypes.Type(value = Split.Csv.class, name = "csv"),
        @JsonSubTypes.Type(value = Split.Json.class, name = "json"),
        @JsonSubTypes.Type(value = Split.Xml.class, name = "xml")
    })
    public sealed interface Split {
        @JsonProperty("kind")
        default String kind() {
            return kindOf(Split.class, getClass());
        }

        /** One record, the payload. */
        record None() implements Split {}

        /** One record per line; {@code header} drops the first. */
        record Csv(boolean header) implements Split {}

        /** One record per element of the array at {@code arrayPath}: {@code $} or {@code $.field.field}. */
        record Json(String arrayPath) implements Split {}

        /** One record per occurrence of {@code element}. */
        record Xml(String element) implements Split {}
    }
}
