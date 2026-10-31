package io.flowcatalyst.function;

/// The host-mediated outbound HTTP client a function calls through — never
/// the JDK's own client directly, so the host can apply the function's
/// `describe.httpAllow` host allowlist and its own timeouts.
public interface HttpCaller {

    /// Makes one outbound call and returns its reply.
    HttpReply send(HttpCall call) throws Exception;
}
