package io.flowcatalyst.function;

import java.util.Arrays;

/// A `dataOnly` delivery, read by [Webhook#dataOnly(Request)]: the platform
/// sends the job's payload as the whole request body, with no envelope, and
/// carries what the envelope would hold in request headers
/// (`internal/platform/dispatchjob/processing`, `buildRequest`):
///
/// | Header | Field |
/// |---|---|
/// | `X-Dispatch-Job-Id` | [#jobId] |
/// | `X-Event-Type` | [#eventType], the job's code |
/// | `X-FlowCatalyst-Client` (`{clientId}:{clientCode}`, omitted for a platform-scoped job or an unresolved client) | [#clientId], [#clientCode] |
/// | `Content-Type` (the payload's own media type, `application/json` when the job declared none) | [#contentType] |
///
/// A header the request lacks gives a `null` field. `body` is cloned in the
/// constructor and again by [#body()].
///
/// @param jobId       the dispatch job's id, `null` when the header is absent
/// @param eventType   the job's code / event type, `null` when absent
/// @param clientId    the owning client's id, `null` when absent
/// @param clientCode  the owning client's code, `null` when absent
/// @param contentType the payload's media type, `null` when absent
/// @param body        the payload, verbatim
public record DataOnly(String jobId, String eventType, String clientId, String clientCode, String contentType,
                       byte[] body) {

    public DataOnly {
        body = body == null ? new byte[0] : Copies.bytes(body);
    }

    @Override
    public byte[] body() {
        return Copies.bytes(body);
    }

    @Override
    public boolean equals(Object o) {
        if (this == o) return true;
        if (!(o instanceof DataOnly other)) return false;
        return java.util.Objects.equals(jobId, other.jobId)
                && java.util.Objects.equals(eventType, other.eventType)
                && java.util.Objects.equals(clientId, other.clientId)
                && java.util.Objects.equals(clientCode, other.clientCode)
                && java.util.Objects.equals(contentType, other.contentType)
                && Arrays.equals(body, other.body);
    }

    @Override
    public int hashCode() {
        return 31 * java.util.Objects.hash(jobId, eventType, clientId, clientCode, contentType) + Arrays.hashCode(body);
    }

    @Override
    public String toString() {
        return "DataOnly[jobId=" + jobId + ", eventType=" + eventType + ", clientId=" + clientId
                + ", clientCode=" + clientCode + ", contentType=" + contentType + ", body.length=" + body.length + "]";
    }
}
