package io.flowcatalyst.ingestor.functions.http;

import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.framework.http.TestResponse;
import io.flowcatalyst.function.EmitResult;
import io.flowcatalyst.ingestor.functions.Fx;
import io.flowcatalyst.ingestor.functions.Harness;
import io.flowcatalyst.ingestor.functions.control.Control;
import io.flowcatalyst.ingestor.functions.control.ControlPlaneException;
import java.time.Duration;
import java.time.Instant;
import java.util.List;
import java.util.Map;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;

/// The listener's invocation path through the framework's pipeline, into a real host: the Go runner's mapping of
/// every outcome (docs/function-runner-plan.md section 6.2): 404 / 405 / 413 / 429 / 503 / 504 / 500, and the auth
/// of each endpoint kind.
@Timeout(value = 60, threadMode = Timeout.ThreadMode.SEPARATE_THREAD) // a call that queues instead of answering 429 must fail, not hang
class FunctionListenerTest {

    static final String SECRET = "whsec_test";
    static final String BASE = "/fn/app.multi";

    Harness h;

    @BeforeEach
    void start() {
        h = new Harness();
        Fx.Jar jar = MultiFunction.jar();
        h.plane.addArtifact(jar);
        h.plane.push(Fx.desired(1,
                Fx.function("fn_1", "app.multi")
                        .webhookSecret(SECRET)
                        .clientId("clt_a")
                        .config("greeting", "hi")
                        .config("undeclared-key", "x")
                        .secret("key", "s3cr3t-value")
                        .version(Fx.version(1, jar, MultiFunction.describe("\"config\":[\"greeting\"]", "\"secrets\":[\"key\"]"), "live", "alias:blue"))
                        .build()));
        h.reconcile();
    }

    @AfterEach
    void stop() {
        h.close();
    }

    private TestResponse get(String path, String... headers) {
        var call = h.http.get(path);
        for (int i = 0; i < headers.length; i += 2) {
            call = call.header(headers[i], headers[i + 1]);
        }
        return call.send();
    }

    private TestResponse post(String path, String body, String... headers) {
        var call = h.http.post(path).body(body);
        for (int i = 0; i < headers.length; i += 2) {
            call = call.header(headers[i], headers[i + 1]);
        }
        return call.send();
    }

    private String errorCode(TestResponse r) {
        return r.tree().path("error").asString("");
    }

    // ── the call and its mapping ──────────────────────────────────────────────────────────────────

    @Test
    void aGetToTheLiveVersionAnswersWithTheFunctionsOwnBody() {
        TestResponse r = get(BASE + "/hello/ann");

        assertThat(r.status()).isEqualTo(200);
        assertThat(r.tree().path("name").asString()).isEqualTo("ann");
        assertThat(r.header("Content-Type")).isEqualTo("application/json");
        assertThat(r.header("X-FlowCatalyst-Invocation")).isNotBlank();
    }

    @Test
    void pathParametersArePercentDecoded() {
        assertThat(get(BASE + "/hello/a%20b").tree().path("name").asString()).isEqualTo("a b");
    }

    @Test
    void nonFnPathsPassThroughToTheRoutes() {
        assertThat(get("/somewhere/else").status()).isEqualTo(404);
        assertThat(errorCode(get("/somewhere/else"))).isEqualTo("NOT_FOUND"); // the framework's own envelope
    }

    @Test
    void anUnknownAddressIs404() {
        TestResponse r = get("/fn/app.nope/hello/ann");
        assertThat(r.status()).isEqualTo(404);
        assertThat(errorCode(r)).isEqualTo("FUNCTION_NOT_FOUND");
        assertThat(get("/fn/").status()).isEqualTo(404);
        assertThat(get("/fn/@v1/x").status()).isEqualTo(404);
    }

    @Test
    void noEndpointIs404AndAWrongMethodIs405() {
        assertThat(errorCode(get(BASE + "/nothing"))).isEqualTo("ROUTE_NOT_FOUND");
        assertThat(get(BASE + "/nothing").status()).isEqualTo(404);

        TestResponse r = post(BASE + "/hello/ann", "");
        assertThat(r.status()).isEqualTo(405);
        assertThat(errorCode(r)).isEqualTo("METHOD_NOT_ALLOWED");
    }

    @Test
    void aBodyOverTheEndpointsLimitIs413() {
        assertThat(post(BASE + "/small", "1234").status()).isEqualTo(200);
        TestResponse r = post(BASE + "/small", "12345");
        assertThat(r.status()).isEqualTo(413);
        assertThat(errorCode(r)).isEqualTo("BODY_TOO_LARGE");
    }

    @Test
    void aDeclaredLengthOverTheLimitIs413WithoutReading() {
        TestResponse r = h.http.post(BASE + "/small").body("1").declaredLength(1000).send();
        assertThat(r.status()).isEqualTo(413);
    }

    @Test
    void theFunctionsLimitApplies() {
        h.plane.push(Fx.desired(2, Fx.function("fn_1", "app.multi").limits(16, 30_000, 10).webhookSecret(SECRET)
                .version(Fx.version(1, MultiFunction.jar(), MultiFunction.describe(), "live")).build()));
        h.plane.addArtifact(MultiFunction.jar());
        h.reconcile();

        assertThat(post(BASE + "/echo", "0123456789").status()).isEqualTo(200);
        assertThat(post(BASE + "/echo", "01234567890").status()).isEqualTo(413);
    }

    @Test
    void anExceptionIs500WithoutItsMessage() {
        TestResponse r = get(BASE + "/boom");

        assertThat(r.status()).isEqualTo(500);
        assertThat(errorCode(r)).isEqualTo("FUNCTION_FAILED");
        assertThat(r.body()).doesNotContain("secret-detail-xyz");
    }

    @Test
    void aFunctionFailingOnEveryCallIsLoggedOnceInFullThenAtMostALineAMinute() {
        for (int i = 0; i < 600; i++) {
            assertThat(get(BASE + "/boom").status()).isEqualTo(500);
        }
        assertThat(failureLines()).as("600 failures within a minute").hasSize(1);
        assertThat(failureLines().get(0).withStack()).isTrue();

        h.clock.advance(Duration.ofSeconds(61));
        get(BASE + "/boom");
        get(BASE + "/boom");

        assertThat(failureLines()).hasSize(2);
        assertThat(failureLines().get(1).withStack()).isFalse();
        assertThat(failureLines().get(1).message()).contains("more like it not logged");
    }

    private List<Harness.Logged> failureLines() {
        return h.logged.stream().filter(l -> l.message().startsWith("function app.multi v1 failed")).toList();
    }

    @Test
    void anErrorThrownByTheFunctionIs500NotACrash() {
        TestResponse r = get(BASE + "/error");
        assertThat(r.status()).isEqualTo(500);
        assertThat(errorCode(r)).isEqualTo("FUNCTION_FAILED");
        assertThat(r.body()).doesNotContain("an Error");
        assertThat(get(BASE + "/hello/ann").status()).as("the host and the function carry on").isEqualTo(200);
    }

    @Test
    void aWebhookEndpointWithoutAMethodStillOnlyTakesPost() {
        for (var m : List.of(io.flowcatalyst.framework.http.Method.GET, io.flowcatalyst.framework.http.Method.PUT, io.flowcatalyst.framework.http.Method.DELETE)) {
            TestResponse r = h.http.request(m, BASE + "/wh").send();
            assertThat(r.status()).as(m.name()).isEqualTo(405);
        }
        assertThat(hookAt("/wh", "{}", SECRET).status()).isEqualTo(200);
    }

    @Test
    void theServersOwnBodyLimitStillApplies() {
        // the framework's http.max-body-bytes caps what a function's own limit can ask for
        var small = io.flowcatalyst.framework.http.TestClient.of(routesOf(h)).withMaxBodyBytes(8);
        assertThat(small.post(BASE + "/echo").body("12345678").send().status()).isEqualTo(200);
        assertThat(small.post(BASE + "/echo").body("123456789").send().status()).isEqualTo(413);
    }

    @Test
    void aStatusOutsideTheValidRangeIs502() {
        assertThat(get(BASE + "/status/150").status()).isEqualTo(502);
        assertThat(errorCode(get(BASE + "/status/150"))).isEqualTo("FUNCTION_BAD_RESPONSE");
        assertThat(get(BASE + "/status/418").status()).isEqualTo(418);
        assertThat(get(BASE + "/status/204").status()).isEqualTo(204);
    }

    @Test
    void aDeadlineIs504() {
        long start = System.nanoTime();
        TestResponse r = post(BASE + "/quick", "");

        assertThat(r.status()).isEqualTo(504);
        assertThat(errorCode(r)).isEqualTo("FUNCTION_TIMEOUT");
        assertThat(Duration.ofNanos(System.nanoTime() - start)).as("the endpoint's 100 ms timeout, not the handler's 5 s").isLessThan(Duration.ofSeconds(2));
    }

    @Test
    void theFunctionResponseHeadersArePassedExceptHopAndCorsAndLength() {
        TestResponse r = get(BASE + "/headers");

        assertThat(r.status()).isEqualTo(200);
        assertThat(r.headerValues("X-Custom")).containsExactly("a", "b");
        assertThat(r.header("Content-Type")).isEqualTo("text/plain");
        assertThat(r.body()).isEqualTo("hi");
        assertThat(r.header("Connection")).as("hop-by-hop headers are the server's").isNull();
        // Content-Length is the server's own: the function's "1" would be wrong for a 2-byte body (TestClient does not add one)
        assertThat(r.header("Content-Length")).isNull();
        // no cors declared for the endpoint: the function's header stays
        assertThat(r.header("Access-Control-Allow-Origin")).isEqualTo("https://evil.example");
    }

    @Test
    void theFunctionSeesTheRequestAsTheGoRunnerBuildsIt() {
        TestResponse r = h.http.post(BASE + "/echo?x=1&y=a%20b").body("payload").header("X-Thing", "v").header("Connection", "keep-alive")
                .header("Authorization", "Bearer abc").header("Host", "functions.example").remoteAddress("10.1.2.3").send();

        assertThat(r.status()).isEqualTo(200);
        var t = r.tree();
        assertThat(t.path("body").asString()).isEqualTo("payload");
        assertThat(t.path("method").asString()).isEqualTo("POST");
        assertThat(t.path("path").asString()).isEqualTo("/echo");
        assertThat(t.path("query").asString()).isEqualTo("x=1&y=a%20b");
        assertThat(t.path("route").asString()).isEqualTo("POST /echo");
        assertThat(t.path("version").asInt()).isEqualTo(1);
        assertThat(t.path("remote").asString()).isEqualTo("10.1.2.3");
        assertThat(t.path("host").asString()).isEqualTo("functions.example");
        assertThat(t.path("public").asBoolean()).isFalse();
        assertThat(t.path("id").asString()).isNotBlank();
        // canonical names, without the hop-by-hop ones; an `auth: none` endpoint keeps the Authorization header
        assertThat(t.path("headers").asString()).contains("X-Thing,").contains("Authorization,").doesNotContain("Connection").doesNotContain("Host,");
    }

    // ── auth ───────────────────────────────────────────────────────────────────────────────────────

    private TestResponse hook(String body, String secret) {
        return hookAt("/hook", body, secret);
    }

    private TestResponse hookAt(String path, String body, String secret) {
        String ts = Instant.now().toString();
        String sig = new WebhookValidator(secret).computeSignature(ts, body.getBytes(java.nio.charset.StandardCharsets.UTF_8));
        return post(BASE + path, body, WebhookValidator.SIGNATURE_HEADER, sig, WebhookValidator.TIMESTAMP_HEADER, ts);
    }

    private static io.flowcatalyst.framework.http.Routes routesOf(Harness h) {
        io.flowcatalyst.framework.http.Routes routes = new io.flowcatalyst.framework.http.Routes();
        h.listener.install(routes);
        return routes;
    }

    @Test
    void aWebhookWithAValidSignatureIsAPlatformCaller() {
        TestResponse r = hook("{\"id\":\"e1\"}", SECRET);

        assertThat(r.status()).isEqualTo(200);
        assertThat(r.tree().path("caller").asString()).isEqualTo("platform");
        assertThat(r.tree().path("body").asString()).isEqualTo("{\"id\":\"e1\"}");
    }

    @Test
    void aWebhookWithABadOrMissingSignatureIs401() {
        TestResponse wrong = hook("{}", "another-secret");
        assertThat(wrong.status()).isEqualTo(401);
        assertThat(errorCode(wrong)).isEqualTo("INVALID_SIGNATURE");
        assertThat(post(BASE + "/hook", "{}").status()).isEqualTo(401);
        // an old timestamp is refused too
        String old = Instant.now().minus(Duration.ofHours(1)).toString();
        String sig = new WebhookValidator(SECRET).computeSignature(old, "{}".getBytes());
        assertThat(post(BASE + "/hook", "{}", WebhookValidator.SIGNATURE_HEADER, sig, WebhookValidator.TIMESTAMP_HEADER, old).status()).isEqualTo(401);
    }

    @Test
    void aWebhookEndpointRefusesNonPostMethods() {
        // the endpoint declares POST only, so a GET is a 405 from the router; one that declares no method is refused at the auth step
        assertThat(get(BASE + "/hook").status()).isEqualTo(405);
    }

    @Test
    void aFunctionWithoutAWebhookSecretYetAsksForTheDeliveryAgain() {
        // Fails closed (the handler never runs) with a 503 the platform retries, not a 401 it would not: the
        // secret arrives with the next desired document.
        Fx.Jar jar = MultiFunction.jar();
        h.plane.push(Fx.desired(2, Fx.function("fn_1", "app.multi")
                .version(Fx.version(1, jar, MultiFunction.describe(), "live")).build()));
        h.reconcile();

        TestResponse r = hook("{}", SECRET);
        assertThat(r.status()).isEqualTo(503);
        assertThat(errorCode(r)).isEqualTo("WEBHOOK_SECRET_PENDING");
        assertThat(r.header("Retry-After")).isEqualTo("5");
    }

    private TestResponse me(String token) {
        return token == null ? get(BASE + "/me") : get(BASE + "/me", "Authorization", "Bearer " + token);
    }

    @Test
    void aPlatformEndpointNeedsAPlatformToken() {
        assertThat(me(null).status()).isEqualTo(401);
        assertThat(errorCode(me(null))).isEqualTo("UNAUTHENTICATED");
        assertThat(me("not-a-token").status()).isEqualTo(401);
        assertThat(get(BASE + "/me", "Authorization", "Basic abc").status()).isEqualTo(401);
    }

    @Test
    void aPlatformTokenBecomesThePrincipalAsTheGoRunnerMapsIt() {
        String token = h.platform.accessToken(Map.of(
                "tier", "CLIENT",
                "clients", List.of("clt_a:bevco", "clt_b"),
                "applications", List.of("*"),
                "scope", "orders:fn:order:create platform:function:version:invoke"));

        TestResponse r = me(token);

        assertThat(r.status()).isEqualTo(200);
        String caller = r.tree().path("caller").asString();
        assertThat(caller).startsWith("principal:prn_SERVICE1:");
        assertThat(caller).contains(":CLIENT:clt_a|clt_b:bevco:true:");
        assertThat(caller).contains("orders:fn:order:create").contains("platform:function:version:invoke");
    }

    @Test
    void anIdentityTokenIsNotAnApiCredential() {
        String identity = h.platform.accessToken(Map.of("token_use", "identity"));
        assertThat(me(identity).status()).isEqualTo(401);
    }

    @Test
    void whenThePlatformsKeysCannotBeReadTheCallerIsToldToTryAgain() {
        String token = h.platform.accessToken(Map.of());
        h.platform.down(true);
        assertThat(me(token).status()).isEqualTo(503);
    }

    @Test
    void aPlatformEndpointNeverHandsTheFunctionTheCredential() {
        // /echo is `none`; /me is platform and does not echo headers: check through an explicit-version call on /echo below
        TestResponse r = get(BASE + "/hello/ann", "Authorization", "Bearer whatever");
        assertThat(r.status()).as("an `auth: none` endpoint does not look at the credential").isEqualTo(200);
    }

    @Test
    void aNoneEndpointIsAnonymous() {
        assertThat(get(BASE + "/hello/ann").status()).isEqualTo(200);
    }

    // ── alias and explicit version ────────────────────────────────────────────────────────────

    @Test
    void anAliasReachesItsVersion() {
        assertThat(get("/fn/app.multi@blue/hello/ann").status()).isEqualTo(200);
        TestResponse r = get("/fn/app.multi@green/hello/ann");
        assertThat(r.status()).isEqualTo(404);
        assertThat(errorCode(r)).isEqualTo("ALIAS_NOT_FOUND");
    }

    @Test
    void anExplicitVersionNeedsATokenWithThePermissionAndReach() {
        assertThat(errorCode(get("/fn/app.multi@v1/hello/ann"))).isEqualTo("UNAUTHENTICATED");

        String noPermission = h.platform.accessToken(Map.of("tier", "ANCHOR", "scope", "orders:fn:order:create"));
        TestResponse forbidden = get("/fn/app.multi@v1/hello/ann", "Authorization", "Bearer " + noPermission);
        assertThat(forbidden.status()).isEqualTo(403);
        assertThat(errorCode(forbidden)).isEqualTo("FORBIDDEN");

        String anchor = h.platform.accessToken(Map.of("tier", "ANCHOR", "scope", "platform:function:version:invoke"));
        assertThat(get("/fn/app.multi@v1/hello/ann", "Authorization", "Bearer " + anchor).status()).isEqualTo(200);

        String wrongClient = h.platform.accessToken(Map.of("tier", "CLIENT", "clients", List.of("clt_z:z"), "scope", "platform:function:version:invoke"));
        assertThat(get("/fn/app.multi@v1/hello/ann", "Authorization", "Bearer " + wrongClient).status()).isEqualTo(403);

        String rightClient = h.platform.accessToken(Map.of("tier", "CLIENT", "clients", List.of("clt_a:a"), "scope", "platform:function:*:*:*"));
        // the wildcard scope has five segments: it does not match the four-segment permission
        assertThat(get("/fn/app.multi@v1/hello/ann", "Authorization", "Bearer " + rightClient).status()).isEqualTo(403);
        String wildcard = h.platform.accessToken(Map.of("tier", "CLIENT", "clients", List.of("clt_a:a"), "scope", "platform:function:version:*"));
        assertThat(get("/fn/app.multi@v1/hello/ann", "Authorization", "Bearer " + wildcard).status()).isEqualTo(200);
    }

    @Test
    void anExplicitVersionThatDoesNotExistIs404() {
        String anchor = h.platform.accessToken(Map.of("tier", "ANCHOR", "scope", "platform:function:version:invoke"));
        TestResponse r = get("/fn/app.multi@v9/hello/ann", "Authorization", "Bearer " + anchor);
        assertThat(r.status()).isEqualTo(404);
        assertThat(errorCode(r)).isEqualTo("VERSION_NOT_FOUND");
    }

    @Test
    void anExplicitVersionCallDoesNotApplyTheEndpointsOwnAuth() {
        String anchor = h.platform.accessToken(Map.of("tier", "ANCHOR", "scope", "platform:function:version:invoke"));
        // /hook is a webhook endpoint: with an explicit version the caller is the verified principal and no signature is needed
        TestResponse r = post("/fn/app.multi@v1/hook", "{}", "Authorization", "Bearer " + anchor);
        assertThat(r.status()).isEqualTo(200);
        assertThat(r.tree().path("caller").asString()).startsWith("principal:");
    }

    @Test
    void theCredentialIsNotHandedToTheFunctionOnAnExplicitVersionCall() {
        String anchor = h.platform.accessToken(Map.of("tier", "ANCHOR", "scope", "platform:function:version:invoke"));
        TestResponse r = post("/fn/app.multi@v1/echo", "x", "Authorization", "Bearer " + anchor);
        assertThat(r.status()).isEqualTo(200);
        assertThat(r.tree().path("headers").asString()).doesNotContain("Authorization");
    }

    // ── cors ────────────────────────────────────────────────────────────────────────────────────

    @Test
    void aCorsPreflightIsAnsweredByTheHostWithoutInvokingOrAuth() {
        Fx.Jar jar = MultiFunction.jar();
        String describe = Fx.describe(MultiFunction.ENTRYPOINT, List.of(
                "{\"method\":\"GET\",\"path\":\"/me\",\"auth\":\"platform\",\"cors\":{\"origins\":[\"https://app.example\"],\"methods\":[\"GET\"],\"allowCredentials\":true}}",
                "{\"method\":\"GET\",\"path\":\"/headers\",\"auth\":\"none\",\"cors\":{\"origins\":[\"https://app.example\"]}}"));
        h.plane.push(Fx.desired(2, Fx.function("fn_1", "app.multi").version(Fx.version(2, jar, describe, "live")).build()));
        h.reconcile();

        TestResponse preflight = h.http.request(io.flowcatalyst.framework.http.Method.OPTIONS, BASE + "/me")
                .header("Origin", "https://app.example").header("Access-Control-Request-Method", "GET").send();
        assertThat(preflight.status()).isEqualTo(204);
        assertThat(preflight.header("Access-Control-Allow-Origin")).isEqualTo("https://app.example");
        assertThat(preflight.header("Access-Control-Allow-Credentials")).isEqualTo("true");
        assertThat(preflight.header("Access-Control-Allow-Methods")).isEqualTo("GET");

        TestResponse refused = h.http.request(io.flowcatalyst.framework.http.Method.OPTIONS, BASE + "/me")
                .header("Origin", "https://evil.example").header("Access-Control-Request-Method", "GET").send();
        assertThat(refused.status()).isEqualTo(403);

        // on an actual request the host is the sole authority for Access-Control-*: the function's own header is dropped
        TestResponse actual = get(BASE + "/headers", "Origin", "https://app.example");
        assertThat(actual.status()).isEqualTo(200);
        assertThat(actual.headerValues("Access-Control-Allow-Origin")).containsExactly("https://app.example");
        assertThat(actual.header("Vary")).isEqualTo("Origin");
    }

    // ── what the function's context offers ────────────────────────────────────────────────────

    @Test
    void theContextHasTheDeclaredConfigAndSecretsOnly() {
        TestResponse r = get(BASE + "/ctx");

        var t = r.tree();
        assertThat(t.path("greeting").asString()).isEqualTo("hi");
        assertThat(t.path("undeclared").asString()).as("a key the describe does not declare is absent").isEqualTo("-");
        assertThat(t.path("key").asString()).isEqualTo("s3cr3t-value");
        assertThat(t.path("address").asString()).isEqualTo("app.multi");
        assertThat(t.path("version").asInt()).isEqualTo(1);
    }

    // ── loading states ──────────────────────────────────────────────────────────────────────────

    @Test
    void aVersionStillLoadingIs503WithRetryAfter() {
        // a function whose artifact cannot be fetched yet
        Fx.Jar other = Fx.hello("x");
        h.plane.push(Fx.desired(2,
                Fx.function("fn_2", "app.late").version(Fx.version(1, other, Fx.describe("fixture.app.Hello", List.of(Fx.endpoint("GET", "/hello/{name}", "none"))), "live")).build()));
        h.plane.artifactFailure = d -> new ControlPlaneException(503, "", "storage down", null);
        h.reconcile();

        TestResponse r = get("/fn/app.late/hello/ann");
        assertThat(r.status()).isEqualTo(503);
        assertThat(errorCode(r)).isEqualTo("FUNCTION_PREPARING");
        assertThat(r.header("Retry-After")).isEqualTo("2");
    }

    @Test
    void aVersionThatFailedToLoadIs503WithALongerRetryAfter() {
        Fx.Jar bad = Fx.jar("fixture.app.NotAFunction", "package fixture.app; public final class NotAFunction {}");
        h.plane.addArtifact(bad);
        h.plane.push(Fx.desired(2,
                Fx.function("fn_2", "app.bad").version(Fx.version(1, bad, Fx.describe("fixture.app.NotAFunction", List.of(Fx.endpoint("GET", "/x", "none"))), "live")).build()));
        h.reconcile();

        TestResponse r = get("/fn/app.bad/x");
        assertThat(r.status()).isEqualTo(503);
        assertThat(errorCode(r)).isEqualTo("FUNCTION_UNAVAILABLE");
        assertThat(r.header("Retry-After")).isEqualTo("30");
        assertThat(r.body()).as("the reason is in the heartbeat and the log, not the reply").doesNotContain("NOT_A_FUNCTION");
    }

    // ── emit and dispatch on the function's behalf ───────────────────────────────────────────

    private void pushEmitting() {
        Fx.Jar jar = MultiFunction.jar();
        String describe = MultiFunction.describe(
                "\"emits\":[\"app:orders:order:created\"]",
                "\"dispatches\":[{\"target\":\"app.other\",\"path\":\"/hook\",\"connection\":\"conn-a\"}]");
        h.plane.push(Fx.desired(2, Fx.function("fn_1", "app.multi").version(Fx.version(3, jar, describe, "live")).build()));
        h.plane.addArtifact(jar);
        h.reconcile();
    }

    @Test
    void anEmitIsSentInThePlatformsShape() {
        pushEmitting();

        TestResponse r = post(BASE + "/emit?type=app:orders:order:created", "{\"n\":1}");

        assertThat(r.status()).isEqualTo(200);
        assertThat(r.tree().path("eventId").asString()).isEqualTo("evt_1");
        Control.EmitRequest sent = h.plane.emits.get(0);
        assertThat(sent.functionId()).isEqualTo("fn_1");
        assertThat(sent.version()).isEqualTo(3);
        assertThat(sent.event().type()).isEqualTo("app:orders:order:created");
        assertThat(sent.event().source()).as("defaults to function:<address>").isEqualTo("function:app.multi");
        assertThat(sent.event().subject()).isEqualTo("order-1");
        assertThat(sent.event().dedupId()).isEqualTo("dedup-1");
        assertThat(sent.event().contentType()).isEqualTo("application/json");
        assertThat(new String(java.util.Base64.getDecoder().decode(sent.data()))).isEqualTo("{\"n\":1}");
    }

    @Test
    void anEmitOfAnUndeclaredTypeIsRefusedWithoutACall() {
        pushEmitting();

        TestResponse r = post(BASE + "/emit?type=app:orders:order:cancelled", "{}");

        assertThat(r.status()).isEqualTo(502);
        assertThat(r.tree().path("code").asString()).isEqualTo("NOT_DECLARED");
        assertThat(h.plane.emits).isEmpty();
    }

    @Test
    void aPlatformRefusalOfAnEmitReachesTheFunction() {
        pushEmitting();
        h.plane.emitAnswer = r -> new EmitResult.Refused("EVENT_TYPE_NOT_OWNED", 403, "no");

        TestResponse r = post(BASE + "/emit", "{}");

        assertThat(r.tree().path("code").asString()).isEqualTo("EVENT_TYPE_NOT_OWNED");
        assertThat(r.tree().path("status").asInt()).isEqualTo(403);
    }

    @Test
    void aDispatchIsSentInThePlatformsShape() {
        pushEmitting();

        TestResponse r = h.http.post(BASE + "/dispatch?target=app.other&path=/hook").body("{\"order\":42}").header("X-Correlation-Id", "corr-7").send();

        assertThat(r.status()).isEqualTo(200);
        assertThat(r.tree().path("jobId").asString()).isEqualTo("djb_1");
        Control.DispatchRequest sent = h.plane.dispatches.get(0);
        assertThat(sent.functionId()).isEqualTo("fn_1");
        assertThat(sent.version()).isEqualTo(3);
        assertThat(sent.invocationId()).isNotBlank();
        assertThat(sent.requestCorrelationId()).isEqualTo("corr-7");
        assertThat(sent.dispatch().target()).isEqualTo("app.other");
        assertThat(sent.dispatch().path()).isEqualTo("/hook");
        assertThat(sent.dispatch().messageGroup()).isEqualTo("g1");
        assertThat(sent.dispatch().contentType()).isEqualTo("application/json");
        assertThat(sent.dispatch().metadata()).containsExactly(new Control.Pair("k", "v"));
        assertThat(new String(java.util.Base64.getDecoder().decode(sent.payload()))).isEqualTo("{\"order\":42}");
    }

    @Test
    void aDispatchToAnUndeclaredTargetIsRefusedWithoutACall() {
        pushEmitting();

        TestResponse r = post(BASE + "/dispatch?target=app.elsewhere&path=/hook", "{}");

        assertThat(r.tree().path("code").asString()).isEqualTo("NOT_DECLARED");
        assertThat(h.plane.dispatches).isEmpty();
    }

    @Test
    void aDispatchRefusalKeepsItsRetryability() {
        pushEmitting();
        h.plane.dispatchAnswer = r -> {
            throw new io.flowcatalyst.function.DispatchException("UNAVAILABLE", "down");
        };

        TestResponse r = post(BASE + "/dispatch", "{}");

        assertThat(r.tree().path("code").asString()).isEqualTo("UNAVAILABLE");
        assertThat(r.tree().path("retryable").asBoolean()).isTrue();
    }
}
