package io.flowcatalyst.sdk.outbox;

import io.flowcatalyst.sdk.http.Json;
import io.flowcatalyst.sdk.outbox.PushException.Code;
import java.nio.charset.StandardCharsets;
import java.util.regex.Pattern;
import tools.jackson.core.JacksonException;
import tools.jackson.core.JsonParser;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;

/**
 * A push to the clients of an application, written to the outbox in the same transaction as the
 * change it announces (docs/push-gateway-spec.md section 6.1). The platform's outbox processor
 * sends the row to the push gateway ({@code POST /api/push/batch}), which fans it out to the
 * clients listening on the topic over server-sent events. Pushes are signals: the clients read the
 * change again; durable work is events and dispatch jobs.
 *
 * <p>Immutable builder: every {@code with*} method returns a new instance. The rules are the push
 * contract's section 1, the refusal codes those of {@code conformance/push/topics.json}, and the
 * payload and row shape those of the Go SDK ({@code pkg/fcsdk/outbox/push.go}); the shared vector
 * {@code outbox-push.json} pins all three.
 *
 * <pre>{@code
 * outbox.createPush(
 *         CreatePushDto.create("shop:orders:ord_2", "order.updated", Map.of("status", "shipped")),
 *         tx);
 * }</pre>
 */
public final class CreatePushDto {

    /** The row type of a push. */
    public static final String ROW_TYPE = "PUSH";

    /**
     * The Postgres notification channel a PUSH row's writer notifies in the same statement as the
     * insert, so a processor that listens on it delivers the push at once instead of at its next
     * poll.
     */
    public static final String NOTIFY_CHANNEL = "fc_outbox_push";

    /** Longest topic, in characters. */
    public static final int MAX_TOPIC_LENGTH = 255;
    /** Longest type, in characters. */
    public static final int MAX_TYPE_LENGTH = 100;
    /** Default data limit in bytes ({@code push.max-data-bytes}). */
    public static final int DEFAULT_MAX_DATA_BYTES = 16384;
    /** The most the data limit may be set to. */
    public static final int MAX_DATA_BYTES = 65536;

    private static final Pattern APPLICATION = Pattern.compile("[a-z][a-z0-9-]*");
    private static final Pattern SEGMENT = Pattern.compile("[0-9A-Za-z_.-]+");
    private static final Pattern ROW_ID = Pattern.compile("[0-9A-Za-z_-]{1,26}");
    private static final ObjectMapper MAPPER = Json.newMapper();

    private String id;
    private final String topic;
    private final String type;
    private final Object data;
    private String clientCode;
    private String messageGroup;
    private boolean ungrouped;
    private int maxDataBytes;

    private CreatePushDto(String topic, String type, Object data) {
        this.topic = topic;
        this.type = type;
        this.data = data;
    }

    private CreatePushDto copy() {
        CreatePushDto c = new CreatePushDto(topic, type, data);
        c.id = id;
        c.clientCode = clientCode;
        c.messageGroup = messageGroup;
        c.ungrouped = ungrouped;
        c.maxDataBytes = maxDataBytes;
        return c;
    }

    /**
     * A push of {@code type} to {@code topic} carrying {@code data}: a map, record or other value
     * Jackson writes as a JSON object, or a {@link JsonNode}, or JSON text (a {@link String}) that
     * holds an object. Anything else is refused by {@link #validate()} with
     * {@code DATA_NOT_OBJECT}. Nothing is checked until then (which writing calls).
     *
     * @param topic {@code {application}:{segment}(:{segment})*}, the first segment the
     *     application ({@code [a-z][a-z0-9-]*}), the others {@code [0-9A-Za-z_.-]+}
     * @param type {@code [a-z][a-z0-9-]*(.[a-z][a-z0-9-]*)*}; {@code reset}, {@code auth-expired},
     *     {@code subscribed} and {@code fw.*} are reserved
     */
    public static CreatePushDto create(String topic, String type, Object data) {
        return new CreatePushDto(topic, type, data);
    }

    /**
     * The row id and the payload's {@code pushId}; a 13-character TSID is generated when unset.
     * At most 26 characters {@code [0-9A-Za-z_-]}.
     */
    public CreatePushDto withId(String id) {
        CreatePushDto c = copy();
        c.id = id;
        return c;
    }

    /**
     * The client the push is for, by identifier. Unset: the writer's configured client code (none
     * for {@link OutboxManager#PLATFORM_CLIENT_CODE}, which leaves the credential to decide).
     */
    public CreatePushDto withClientCode(String clientCode) {
        CreatePushDto c = copy();
        c.clientCode = clientCode;
        return c;
    }

    /** The outbox message group in place of the topic. Ignored when {@link #withUngrouped()}. */
    public CreatePushDto withMessageGroup(String messageGroup) {
        CreatePushDto c = copy();
        c.messageGroup = messageGroup;
        return c;
    }

    /**
     * Write the row with no message group: the processor batches such rows into concurrent calls,
     * which is faster and keeps no order. For signal-only topics.
     */
    public CreatePushDto withUngrouped() {
        CreatePushDto c = copy();
        c.ungrouped = true;
        return c;
    }

    /** The data limit (default 16 KiB, at most 64 KiB): the application's {@code push.max-data-bytes}. */
    public CreatePushDto withMaxDataBytes(int maxDataBytes) {
        CreatePushDto c = copy();
        c.maxDataBytes = maxDataBytes;
        return c;
    }

    public String id() {
        return id;
    }

    public String topic() {
        return topic;
    }

    public String type() {
        return type;
    }

    public String clientCode() {
        return clientCode;
    }

    public boolean ungrouped() {
        return ungrouped;
    }

    private int limit() {
        return maxDataBytes > 0 ? Math.min(maxDataBytes, MAX_DATA_BYTES) : DEFAULT_MAX_DATA_BYTES;
    }

    /**
     * Apply the push contract's section 1 (topic, type, data) and the row's own rules (id, client
     * code, group), in the Go SDK's order.
     *
     * @throws PushException with the refusal code
     */
    public void validate() {
        compactData();
    }

    /** Runs every rule and returns the data as compact JSON text. */
    private String compactData() {
        if (topic == null || topic.length() > MAX_TOPIC_LENGTH) {
            throw refuse(Code.TOPIC_INVALID, "a topic is at most " + MAX_TOPIC_LENGTH + " characters");
        }
        String[] segs = topic.split(":", -1);
        if (segs.length < 2 || !APPLICATION.matcher(segs[0]).matches()) {
            throw refuse(Code.TOPIC_INVALID,
                    "a topic is {application}:{segment}(:{segment})*: \"" + topic + "\"");
        }
        for (int i = 1; i < segs.length; i++) {
            if (!SEGMENT.matcher(segs[i]).matches()) {
                throw refuse(Code.TOPIC_INVALID,
                        "a topic segment is [0-9A-Za-z_.-]+: \"" + topic + "\"");
            }
        }
        if (type == null || type.length() > MAX_TYPE_LENGTH) {
            throw refuse(Code.TYPE_INVALID, "a type is at most " + MAX_TYPE_LENGTH + " characters");
        }
        for (String part : type.split("\\.", -1)) {
            if (!APPLICATION.matcher(part).matches()) {
                throw refuse(Code.TYPE_INVALID,
                        "a type is [a-z][a-z0-9-]*(.[a-z][a-z0-9-]*)*: \"" + type + "\"");
            }
        }
        if (type.equals("reset") || type.equals("auth-expired") || type.equals("subscribed")
                || type.startsWith("fw.")) {
            throw refuse(Code.TYPE_RESERVED, "the type \"" + type + "\" is reserved");
        }

        String text = jsonText();
        int size = text.getBytes(StandardCharsets.UTF_8).length;
        if (size > limit()) {
            throw refuse(Code.DATA_TOO_LARGE, "the data is " + size + " bytes, more than " + limit());
        }

        if (id != null && !id.isEmpty() && !ROW_ID.matcher(id).matches()) {
            throw refuse(Code.PUSH_ID_INVALID, "id must match [0-9A-Za-z_-]{1,26}");
        }
        if (clientCode != null && !clientCode.isEmpty()
                && (utf8Length(clientCode) > 100 || clientCode.isBlank())) {
            throw refuse(Code.CLIENT_INVALID, "clientCode must be 1 to 100 characters");
        }
        if (messageGroup != null && utf8Length(messageGroup) > 255) {
            throw refuse(Code.TOPIC_INVALID, "messageGroup is at most 255 characters");
        }
        return text;
    }

    /** The data as compact JSON text, checked to be a JSON object. */
    private String jsonText() {
        if (data instanceof String s) {
            String trimmed = s.strip();
            if (trimmed.isEmpty() || !isJson(trimmed)) {
                throw refuse(Code.DATA_INVALID, "the data is not valid JSON");
            }
            if (trimmed.charAt(0) != '{') {
                throw refuse(Code.DATA_NOT_OBJECT, "the data is not a JSON object");
            }
            return compactJson(trimmed);
        }
        String text;
        try {
            text = MAPPER.writeValueAsString(data);
        } catch (JacksonException e) {
            throw refuse(Code.DATA_INVALID, "the data does not marshal to JSON: " + e.getOriginalMessage());
        }
        if (text.isEmpty() || text.charAt(0) != '{') {
            throw refuse(Code.DATA_NOT_OBJECT, "the data is not a JSON object");
        }
        return text;
    }

    /** True when {@code text} is exactly one JSON value (nothing after it). */
    private static boolean isJson(String text) {
        try (JsonParser parser = MAPPER.createParser(text)) {
            JsonNode node = parser.readValueAsTree();
            return node != null && parser.nextToken() == null;
        } catch (JacksonException e) {
            return false;
        }
    }

    /** Whitespace-strip JSON text without re-serialising its values. */
    private static String compactJson(String text) {
        StringBuilder out = new StringBuilder(text.length());
        boolean inString = false;
        boolean escaped = false;
        for (int i = 0; i < text.length(); i++) {
            char ch = text.charAt(i);
            if (inString) {
                out.append(ch);
                if (escaped) {
                    escaped = false;
                } else if (ch == '\\') {
                    escaped = true;
                } else if (ch == '"') {
                    inString = false;
                }
            } else if (ch == '"') {
                inString = true;
                out.append(ch);
            } else if (ch != ' ' && ch != '\t' && ch != '\n' && ch != '\r') {
                out.append(ch);
            }
        }
        return out.toString();
    }

    private static int utf8Length(String s) {
        return s.getBytes(StandardCharsets.UTF_8).length;
    }

    private static PushException refuse(Code code, String message) {
        return new PushException(code, message);
    }

    /**
     * Validate and serialise as the PUSH row's payload JSON text, members in the framework's
     * order: {@code topic}, {@code type}, {@code data}, {@code clientCode}, {@code pushId}.
     * {@code data} is the object itself, never encoded JSON in a string.
     *
     * @param id the row id (also the {@code pushId})
     * @param defaultClientCode the writer's client code, used when the push names none;
     *     {@link OutboxManager#PLATFORM_CLIENT_CODE} leaves {@code clientCode} out
     * @throws PushException when the push is refused
     */
    public String toPayloadJson(String id, String defaultClientCode) {
        String compact = compactData();
        String code = clientCode == null ? "" : clientCode.strip();
        if (code.isEmpty()) {
            String c = defaultClientCode == null ? "" : defaultClientCode.strip();
            if (!c.equals(OutboxManager.PLATFORM_CLIENT_CODE)) {
                code = c;
            }
        }
        StringBuilder out = new StringBuilder(compact.length() + 96);
        out.append("{\"topic\":").append(quote(topic))
                .append(",\"type\":").append(quote(type))
                .append(",\"data\":").append(compact);
        if (!code.isEmpty()) {
            out.append(",\"clientCode\":").append(quote(code));
        }
        return out.append(",\"pushId\":").append(quote(id)).append('}').toString();
    }

    /** The message group the row is written with: none when ungrouped, else the override, else the topic. */
    public String resolvedMessageGroup() {
        if (ungrouped) {
            return null;
        }
        return messageGroup != null && !messageGroup.isEmpty() ? messageGroup : topic;
    }

    private static String quote(String s) {
        return MAPPER.writeValueAsString(s);
    }
}
