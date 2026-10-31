package io.flowcatalyst.function;

import java.nio.charset.StandardCharsets;
import java.util.Arrays;
import java.util.Collections;
import java.util.Map;
import java.util.Objects;
import java.util.TreeMap;

/// A dispatch job a function asks the platform to create through
/// [Dispatcher#dispatch]: the input of `fn.dispatch` in the other SDKs. Build
/// one with [#to(String, String)]:
///
/// ```java
/// ctx.dispatcher().dispatch(DispatchJob.to("orders.fulfil", "/fulfil")
///         .payloadJson("{\"order\":42}").messageGroup("order-42").build());
/// ```
///
/// Only `target` and `path` are required. `payload` is cloned in the
/// constructor and again by [#payload()]; `metadata` is copied, unmodifiable
/// and sorted by key (the platform stores an array of entries; keys starting
/// `fn.` are the platform's and are refused).
///
/// @param target         address of a function of this function's application, declared in `dispatches`
/// @param path           literal webhook path on the target, declared in `dispatches`
/// @param payload        the job's payload, delivered as the request body (see [#dataOnly]); `null` for none
/// @param contentType    the payload's media type; `null` for the platform's default
/// @param code           the job's code; `null` for `<application>:function:<name>:dispatch`, otherwise it must start with the function's application
/// @param subject        the job's subject, `null` for none
/// @param metadata       job metadata, sorted by key
/// @param messageGroup   jobs of one group are delivered in order, per `mode`; `null` for none
/// @param mode           `IMMEDIATE`, `NEXT_ON_ERROR` or `BLOCK_ON_ERROR`; `null` for the platform's default
/// @param correlationId  `null` to default to the calling request's `X-Correlation-Id`, if it had one
/// @param causationId    the id of what directly caused this job, `null` for none
/// @param descriptor     what the job is, in words, for the dispatch-jobs grid; `null` for none
/// @param maxRetries     `null` for the platform's default
/// @param timeoutSeconds `null` for the platform's default
/// @param dataOnly       deliver the payload alone as the body (`true`) or wrapped in the platform's envelope; `null` means `true`
public record DispatchJob(
        String target,
        String path,
        byte[] payload,
        String contentType,
        String code,
        String subject,
        Map<String, String> metadata,
        String messageGroup,
        String mode,
        String correlationId,
        String causationId,
        String descriptor,
        Integer maxRetries,
        Integer timeoutSeconds,
        Boolean dataOnly) {

    public DispatchJob {
        Objects.requireNonNull(target, "target");
        Objects.requireNonNull(path, "path");
        payload = Copies.bytes(payload);
        metadata = metadata == null
                ? Map.of()
                : Collections.unmodifiableMap(new TreeMap<>(metadata));
    }

    @Override
    public byte[] payload() {
        return Copies.bytes(payload);
    }

    /// Starts a job to `target` (a function address) at `path`.
    public static Builder to(String target, String path) {
        return new Builder(target, path);
    }

    @Override
    public boolean equals(Object o) {
        if (this == o) return true;
        if (!(o instanceof DispatchJob other)) return false;
        return Objects.equals(target, other.target)
                && Objects.equals(path, other.path)
                && Arrays.equals(payload, other.payload)
                && Objects.equals(contentType, other.contentType)
                && Objects.equals(code, other.code)
                && Objects.equals(subject, other.subject)
                && Objects.equals(metadata, other.metadata)
                && Objects.equals(messageGroup, other.messageGroup)
                && Objects.equals(mode, other.mode)
                && Objects.equals(correlationId, other.correlationId)
                && Objects.equals(causationId, other.causationId)
                && Objects.equals(descriptor, other.descriptor)
                && Objects.equals(maxRetries, other.maxRetries)
                && Objects.equals(timeoutSeconds, other.timeoutSeconds)
                && Objects.equals(dataOnly, other.dataOnly);
    }

    @Override
    public int hashCode() {
        int result = Objects.hash(target, path, contentType, code, subject, metadata, messageGroup, mode,
                correlationId, causationId, descriptor, maxRetries, timeoutSeconds, dataOnly);
        return 31 * result + Arrays.hashCode(payload);
    }

    @Override
    public String toString() {
        return "DispatchJob[target=" + target + ", path=" + path + ", payload.length="
                + (payload == null ? 0 : payload.length) + ", contentType=" + contentType + ", code=" + code
                + ", subject=" + subject + ", metadata=" + metadata + ", messageGroup=" + messageGroup
                + ", mode=" + mode + ", correlationId=" + correlationId + ", causationId=" + causationId
                + ", descriptor=" + descriptor + ", maxRetries=" + maxRetries
                + ", timeoutSeconds=" + timeoutSeconds + ", dataOnly=" + dataOnly + "]";
    }

    /// Builds a [DispatchJob]; every setter is optional.
    public static final class Builder {
        private final String target;
        private final String path;
        private byte[] payload;
        private String contentType;
        private String code;
        private String subject;
        private final Map<String, String> metadata = new TreeMap<>();
        private String messageGroup;
        private String mode;
        private String correlationId;
        private String causationId;
        private String descriptor;
        private Integer maxRetries;
        private Integer timeoutSeconds;
        private Boolean dataOnly;

        private Builder(String target, String path) {
            this.target = target;
            this.path = path;
        }

        /// The payload as bytes, verbatim (set [#contentType] yourself).
        public Builder payload(byte[] payload) {
            this.payload = payload;
            return this;
        }

        /// The payload as UTF-8 text, verbatim (set [#contentType] yourself).
        public Builder payload(String payload) {
            this.payload = payload == null ? null : payload.getBytes(StandardCharsets.UTF_8);
            return this;
        }

        /// The payload as JSON text; the content type becomes `application/json`
        /// unless one was set explicitly.
        public Builder payloadJson(String json) {
            this.payload = json == null ? null : json.getBytes(StandardCharsets.UTF_8);
            if (json != null && contentType == null) contentType = "application/json";
            return this;
        }

        public Builder contentType(String contentType) {
            this.contentType = contentType;
            return this;
        }

        public Builder code(String code) {
            this.code = code;
            return this;
        }

        public Builder subject(String subject) {
            this.subject = subject;
            return this;
        }

        /// Adds one metadata entry.
        public Builder metadata(String key, String value) {
            this.metadata.put(Objects.requireNonNull(key, "key"), Objects.requireNonNull(value, "value"));
            return this;
        }

        public Builder messageGroup(String messageGroup) {
            this.messageGroup = messageGroup;
            return this;
        }

        public Builder mode(String mode) {
            this.mode = mode;
            return this;
        }

        public Builder correlationId(String correlationId) {
            this.correlationId = correlationId;
            return this;
        }

        public Builder causationId(String causationId) {
            this.causationId = causationId;
            return this;
        }

        public Builder descriptor(String descriptor) {
            this.descriptor = descriptor;
            return this;
        }

        public Builder maxRetries(int maxRetries) {
            this.maxRetries = maxRetries;
            return this;
        }

        public Builder timeoutSeconds(int timeoutSeconds) {
            this.timeoutSeconds = timeoutSeconds;
            return this;
        }

        public Builder dataOnly(boolean dataOnly) {
            this.dataOnly = dataOnly;
            return this;
        }

        public DispatchJob build() {
            return new DispatchJob(target, path, payload, contentType, code, subject, metadata, messageGroup, mode,
                    correlationId, causationId, descriptor, maxRetries, timeoutSeconds, dataOnly);
        }
    }
}
