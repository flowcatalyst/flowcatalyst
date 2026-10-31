package io.flowcatalyst.function;

import java.time.Instant;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.Optional;

import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

/// `Request`'s case-insensitive
/// header lookup keeps the map's own key spelling, every array/collection
/// component is a defensive copy both ways, and equality/`toString` treat
/// `body` by content (length only in `toString`).
class RequestTest {

    @Test
    void bodyIsIndependentOfTheArrayPassedInAndReadOut() {
        byte[] body = {1, 2, 3};
        Request request = request(Map.of(), body);
        body[0] = 99;
        assertThat(request.body()).containsExactly(1, 2, 3);

        byte[] read = request.body();
        read[0] = 42;
        assertThat(request.body()).containsExactly(1, 2, 3);
    }

    @Test
    void pathParamsQueryAndHeadersAreUnmodifiable() {
        Request request = request(Map.of("Content-Type", List.of("text/plain")), new byte[0]);
        assertThatThrownBy(() -> request.pathParams().put("x", "y")).isInstanceOf(UnsupportedOperationException.class);
        assertThatThrownBy(() -> request.query().put("x", List.of("y"))).isInstanceOf(UnsupportedOperationException.class);
        assertThatThrownBy(() -> request.headers().put("x", List.of("y"))).isInstanceOf(UnsupportedOperationException.class);
        assertThatThrownBy(() -> request.headers().get("Content-Type").add("z"))
                .isInstanceOf(UnsupportedOperationException.class);
    }

    @Test
    void headersAreIndependentOfTheMapPassedIn() {
        Map<String, List<String>> headers = new HashMap<>();
        headers.put("X-Trace", new ArrayList<>(List.of("abc")));
        Request request = request(headers, new byte[0]);
        headers.get("X-Trace").add("mutated");
        assertThat(request.headers().get("X-Trace")).containsExactly("abc");
    }

    @Test
    void headerLookupIsCaseInsensitiveButTheMapKeepsOriginalSpelling() {
        Request request = request(Map.of("X-Request-Id", List.of("abc-123")), new byte[0]);

        assertThat(request.header("x-request-id")).contains("abc-123");
        assertThat(request.header("X-REQUEST-ID")).contains("abc-123");
        assertThat(request.headers("x-request-id")).containsExactly("abc-123");
        assertThat(request.header("x-missing")).isEmpty();

        // The map itself was not lower-cased or otherwise normalised.
        assertThat(request.headers()).containsOnlyKeys("X-Request-Id");
    }

    @Test
    void headerLookupReturnsEmptyOptionalNotNull() {
        Request request = request(Map.of(), new byte[0]);
        Optional<String> value = request.header("Absent");
        assertThat(value).isEmpty();
    }

    @Test
    void requiredComponentsRejectNull() {
        assertThatThrownBy(() -> new Request(null, ADDRESS, 1, "GET", "/", "", Map.of(), "", Map.of(),
                Caller.Anonymous.INSTANCE, DEADLINE, "", "", false, new byte[0])).isInstanceOf(NullPointerException.class);
        assertThatThrownBy(() -> new Request("id", null, 1, "GET", "/", "", Map.of(), "", Map.of(),
                Caller.Anonymous.INSTANCE, DEADLINE, "", "", false, new byte[0])).isInstanceOf(NullPointerException.class);
        assertThatThrownBy(() -> new Request("id", ADDRESS, 1, "GET", "/", "", Map.of(), "", Map.of(),
                null, DEADLINE, "", "", false, new byte[0])).isInstanceOf(NullPointerException.class);
        assertThatThrownBy(() -> new Request("id", ADDRESS, 1, "GET", "/", "", Map.of(), "", Map.of(),
                Caller.Anonymous.INSTANCE, null, "", "", false, new byte[0])).isInstanceOf(NullPointerException.class);
    }

    @Test
    void optionalTextFieldsDefaultToEmptyAndABodyDefaultsToEmpty() {
        Request request = new Request("id", ADDRESS, 3, "GET", "/", null, null, null, null,
                Caller.Anonymous.INSTANCE, DEADLINE, null, null, true, null);
        assertThat(request.rawQuery()).isEmpty();
        assertThat(request.route()).isEmpty();
        assertThat(request.host()).isEmpty();
        assertThat(request.remoteAddr()).isEmpty();
        assertThat(request.headers()).isEmpty();
        assertThat(request.pathParams()).isEmpty();
        assertThat(request.body()).isEmpty();
        assertThat(request.isPublic()).isTrue();
    }

    @Test
    void carriesEveryAbiRequestField() {
        Request request = new Request("inv-9", ADDRESS, 4, "POST", "/hello/ann", "a=1", Map.of("X", List.of("1")),
                "/hello/{name}", Map.of("name", "ann"), Caller.Platform.INSTANCE, DEADLINE, "api.example.com",
                "10.0.0.1", true, new byte[] {1});
        assertThat(request.id()).isEqualTo("inv-9");
        assertThat(request.address()).isEqualTo(ADDRESS);
        assertThat(request.version()).isEqualTo(4);
        assertThat(request.method()).isEqualTo("POST");
        assertThat(request.path()).isEqualTo("/hello/ann");
        assertThat(request.rawQuery()).isEqualTo("a=1");
        assertThat(request.route()).isEqualTo("/hello/{name}");
        assertThat(request.pathParams()).containsEntry("name", "ann");
        assertThat(request.caller()).isEqualTo(Caller.Platform.INSTANCE);
        assertThat(request.deadline()).isEqualTo(DEADLINE);
        assertThat(request.host()).isEqualTo("api.example.com");
        assertThat(request.remoteAddr()).isEqualTo("10.0.0.1");
        assertThat(request.isPublic()).isTrue();
    }

    @Test
    void queryParsesAndDecodesTheRawQuery() {
        Request request = withQuery("a=1&b=x%20y&a=2&flag&c=%E2%98%83&empty=");
        assertThat(request.query()).containsOnlyKeys("a", "b", "flag", "c", "empty");
        assertThat(request.query().get("a")).containsExactly("1", "2");
        assertThat(request.query().get("b")).containsExactly("x y");
        assertThat(request.query().get("flag")).containsExactly("");
        assertThat(request.query().get("c")).containsExactly("\u2603");
        assertThat(request.query().get("empty")).containsExactly("");
        assertThat(request.queryParam("a")).contains("1");
        assertThat(request.queryParam("nope")).isEmpty();
    }

    @Test
    void emptyAndMalformedQueriesDoNotThrow() {
        assertThat(withQuery("").query()).isEmpty();
        assertThat(withQuery("&&").query()).isEmpty();
        assertThat(withQuery("a=%zz").query().get("a")).containsExactly("%zz");
    }

    @Test
    void bodyTextDecodesUtf8() {
        assertThat(request(Map.of(), "caf\u00e9".getBytes(java.nio.charset.StandardCharsets.UTF_8)).bodyText())
                .isEqualTo("caf\u00e9");
    }

    @Test
    void equalityIsByContentIncludingBody() {
        Request a = request(Map.of("X", List.of("1")), new byte[] {1, 2});
        Request b = request(Map.of("X", List.of("1")), new byte[] {1, 2});
        Request c = request(Map.of("X", List.of("1")), new byte[] {1, 3});
        assertThat(a).isEqualTo(b).hasSameHashCodeAs(b);
        assertThat(a).isNotEqualTo(c);
    }

    @Test
    void toStringReportsBodyLengthNotBytes() {
        Request request = request(Map.of(), new byte[] {1, 2, 3});
        assertThat(request.toString()).contains("body.length=3").doesNotContain("[1, 2, 3]");
    }

    private static final FunctionAddress ADDRESS = FunctionAddress.parse("billing.api");
    private static final Instant DEADLINE = Instant.parse("2026-09-20T00:00:30Z");

    private static Request withQuery(String rawQuery) {
        return new Request("invocation-1", ADDRESS, 1, "GET", "/x", rawQuery, Map.of(), "", Map.of(),
                Caller.Anonymous.INSTANCE, DEADLINE, "", "", false, new byte[0]);
    }

    private static Request request(Map<String, List<String>> headers, byte[] body) {
        return new Request("invocation-1", ADDRESS, 1, "GET", "/x", "", headers, "/x", Map.of(),
                Caller.Anonymous.INSTANCE, DEADLINE, "api.acme.com", "127.0.0.1", false, body);
    }
}
