package io.flowcatalyst.ingestor.functions.http;

import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.ingestor.functions.control.Describe;
import java.util.List;
import org.junit.jupiter.api.Test;

/// CORS as the Go runner does it (`handleCORS`).
class CorsPolicyTest {

    private static Describe.Cors cors(List<String> origins, List<String> methods, List<String> headers, boolean credentials) {
        return new Describe.Cors(origins, methods, headers, credentials);
    }

    @Test
    void noOriginIsNotACorsRequest() {
        CorsPolicy.Outcome o = CorsPolicy.evaluate(cors(List.of("*"), List.of(), List.of(), false), "GET", null, null, null);
        assertThat(o.answered()).isFalse();
        assertThat(o.headers()).isEmpty();
    }

    @Test
    void anAllowedOriginGetsTheHeadersOnAnActualRequest() {
        CorsPolicy.Outcome o = CorsPolicy.evaluate(cors(List.of("https://app.example.com"), List.of(), List.of(), true), "GET", "https://APP.example.com", null, null);

        assertThat(o.answered()).isFalse();
        assertThat(o.headers()).containsEntry("Vary", "Origin")
                .containsEntry("Access-Control-Allow-Origin", "https://APP.example.com")
                .containsEntry("Access-Control-Allow-Credentials", "true");
    }

    @Test
    void aLoneWildcardIsStarWithoutCredentials() {
        CorsPolicy.Outcome o = CorsPolicy.evaluate(cors(List.of("*"), List.of(), List.of(), false), "GET", "https://x.example", null, null);
        assertThat(o.headers()).containsEntry("Access-Control-Allow-Origin", "*");
    }

    @Test
    void aWildcardNeverAppliesWithCredentials() {
        CorsPolicy.Outcome o = CorsPolicy.evaluate(cors(List.of("*"), List.of(), List.of(), true), "GET", "https://x.example", null, null);
        assertThat(o.answered()).isFalse();
        assertThat(o.headers()).isEmpty();
    }

    @Test
    void aWildcardAmongOthersIsTheRequestsOrigin() {
        CorsPolicy.Outcome o = CorsPolicy.evaluate(cors(List.of("https://a.example", "*"), List.of(), List.of(), false), "GET", "https://x.example", null, null);
        assertThat(o.headers()).containsEntry("Access-Control-Allow-Origin", "https://x.example");
    }

    @Test
    void aPreflightIsAnsweredByTheHost() {
        CorsPolicy.Outcome o = CorsPolicy.evaluate(
                cors(List.of("https://a.example"), List.of("GET", "POST"), List.of("X-Token"), false),
                "OPTIONS", "https://a.example", "POST", "x-token, x-other");

        assertThat(o.answered()).isTrue();
        assertThat(o.status()).isEqualTo(204);
        assertThat(o.headers()).containsEntry("Access-Control-Allow-Methods", "GET, POST")
                .containsEntry("Access-Control-Allow-Headers", "X-Token")
                .containsEntry("Access-Control-Max-Age", "600");
    }

    @Test
    void aPreflightWithoutDeclaredMethodsAndHeadersEchoesTheRequest() {
        CorsPolicy.Outcome o = CorsPolicy.evaluate(cors(List.of("*"), List.of(), List.of(), false), "OPTIONS", "https://a.example", "PUT", "x-a");

        assertThat(o.headers()).containsEntry("Access-Control-Allow-Methods", "PUT").containsEntry("Access-Control-Allow-Headers", "x-a");
    }

    @Test
    void aPreflightFromADisallowedOriginIs403WithoutHeaders() {
        CorsPolicy.Outcome o = CorsPolicy.evaluate(cors(List.of("https://a.example"), List.of(), List.of(), false), "OPTIONS", "https://evil.example", "GET", null);

        assertThat(o.answered()).isTrue();
        assertThat(o.status()).isEqualTo(403);
        assertThat(o.headers()).isEmpty();
    }

    @Test
    void anOptionsRequestWithoutRequestMethodIsNotAPreflight() {
        CorsPolicy.Outcome o = CorsPolicy.evaluate(cors(List.of("*"), List.of(), List.of(), false), "OPTIONS", "https://a.example", null, null);
        assertThat(o.answered()).isFalse();
    }
}
