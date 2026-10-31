package io.flowcatalyst.function.testing;

import io.flowcatalyst.function.Caller;
import io.flowcatalyst.function.FunctionAddress;
import io.flowcatalyst.function.Request;

import java.nio.charset.StandardCharsets;
import java.time.Instant;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

/// Builds a [Request] for a unit test with sensible defaults: address
/// `app.fn`, version 1, id `inv-1`, caller [Caller.Anonymous], a deadline far
/// in the future, no headers and an empty body.
///
/// ```java
/// Request in = FakeRequest.post("/events").caller(Caller.Platform.INSTANCE)
///         .body("{\"id\":\"j1\",\"type\":\"t\",\"attemptNumber\":1}").build();
/// ```
public final class FakeRequest {

    private final String method;
    private final String path;
    private String id = "inv-1";
    private FunctionAddress address = FunctionAddress.parse("app.fn");
    private int version = 1;
    private String rawQuery = "";
    private final Map<String, List<String>> headers = new LinkedHashMap<>();
    private String route = "";
    private final Map<String, String> pathParams = new LinkedHashMap<>();
    private Caller caller = Caller.Anonymous.INSTANCE;
    private Instant deadline = Instant.parse("2099-01-01T00:00:00Z");
    private String host = "";
    private String remoteAddr = "";
    private boolean isPublic;
    private byte[] body = new byte[0];

    private FakeRequest(String method, String path) {
        this.method = method;
        this.path = path;
    }

    public static FakeRequest get(String path) {
        return new FakeRequest("GET", path);
    }

    public static FakeRequest post(String path) {
        return new FakeRequest("POST", path);
    }

    public static FakeRequest of(String method, String path) {
        return new FakeRequest(method, path);
    }

    public FakeRequest id(String id) {
        this.id = id;
        return this;
    }

    public FakeRequest address(FunctionAddress address) {
        this.address = address;
        return this;
    }

    public FakeRequest version(int version) {
        this.version = version;
        return this;
    }

    public FakeRequest rawQuery(String rawQuery) {
        this.rawQuery = rawQuery;
        return this;
    }

    public FakeRequest header(String name, String value) {
        headers.computeIfAbsent(name, k -> new ArrayList<>()).add(value);
        return this;
    }

    public FakeRequest route(String route) {
        this.route = route;
        return this;
    }

    public FakeRequest pathParam(String name, String value) {
        pathParams.put(name, value);
        return this;
    }

    public FakeRequest caller(Caller caller) {
        this.caller = caller;
        return this;
    }

    public FakeRequest deadline(Instant deadline) {
        this.deadline = deadline;
        return this;
    }

    public FakeRequest host(String host) {
        this.host = host;
        return this;
    }

    public FakeRequest remoteAddr(String remoteAddr) {
        this.remoteAddr = remoteAddr;
        return this;
    }

    public FakeRequest isPublic(boolean isPublic) {
        this.isPublic = isPublic;
        return this;
    }

    public FakeRequest body(byte[] body) {
        this.body = body;
        return this;
    }

    public FakeRequest body(String body) {
        this.body = body.getBytes(StandardCharsets.UTF_8);
        return this;
    }

    public Request build() {
        return new Request(id, address, version, method, path, rawQuery, headers, route, pathParams, caller,
                deadline, host, remoteAddr, isPublic, body);
    }
}
