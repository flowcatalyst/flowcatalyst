package io.flowcatalyst.ingestor.functions.http;

import io.flowcatalyst.ingestor.functions.Fx;
import java.util.List;

/// A function jar that answers every route the listener tests need, chosen by `in.route()`, and its describe document.
final class MultiFunction {

    static final String ENTRYPOINT = "fixture.app.Multi";

    private MultiFunction() {
    }

    static final String SOURCE = """
            package fixture.app;
            import io.flowcatalyst.function.*;
            import java.time.Duration;
            import java.util.*;
            public final class Multi implements Function {
                private static String esc(String s) { return s.replace("\\\\", "\\\\\\\\").replace("\\"", "\\\\\\""); }
                public Result handle(Request in, FunctionContext ctx) throws Exception {
                    switch (in.route()) {
                        case "GET /hello/{name}":
                            return Result.json(200, "{\\"name\\":\\"" + esc(in.pathParams().get("name")) + "\\"}");
                        case "POST /hook":
                            return Result.json(200, "{\\"caller\\":\\"" + callerOf(in) + "\\",\\"body\\":\\"" + esc(in.bodyText()) + "\\"}");
                        case "GET /me":
                            return Result.json(200, "{\\"caller\\":\\"" + callerOf(in) + "\\"}");
                        case "POST /echo": {
                            StringBuilder names = new StringBuilder();
                            for (String h : new TreeSet<>(in.headers().keySet())) { names.append(h).append(','); }
                            return Result.json(200, "{\\"body\\":\\"" + esc(in.bodyText()) + "\\",\\"method\\":\\"" + in.method()
                                    + "\\",\\"path\\":\\"" + esc(in.path()) + "\\",\\"query\\":\\"" + esc(in.rawQuery())
                                    + "\\",\\"route\\":\\"" + esc(in.route()) + "\\",\\"version\\":" + in.version()
                                    + ",\\"remote\\":\\"" + in.remoteAddr() + "\\",\\"host\\":\\"" + in.host()
                                    + "\\",\\"public\\":" + in.isPublic() + ",\\"id\\":\\"" + in.id() + "\\",\\"headers\\":\\"" + names + "\\"}");
                        }
                        case "POST /quick":
                            Thread.sleep(5000);
                            return Result.ack();
                        case "POST /small":
                            return Result.ack();
                        case "GET /error":
                            throw new AssertionError("an Error, not an Exception");
                        case "/wh":
                            return Result.ack();
                        case "GET /boom":
                            throw new IllegalStateException("secret-detail-xyz");
                        case "GET /sleep/{ms}":
                            Thread.sleep(Long.parseLong(in.pathParams().get("ms")));
                            return Result.ack();
                        case "GET /stubborn/{ms}": {
                            // ignores interrupts: keeps running past its deadline
                            long end = System.nanoTime() + Long.parseLong(in.pathParams().get("ms")) * 1_000_000L;
                            while (System.nanoTime() < end) {
                                try { Thread.sleep(5); } catch (InterruptedException ignored) { }
                            }
                            System.getProperties().put("fx.stubborn.returned", Boolean.TRUE);
                            return Result.ack();
                        }
                        case "GET /status/{code}":
                            return Result.status(Integer.parseInt(in.pathParams().get("code")));
                        case "GET /headers":
                            return Result.http(200, Map.of(
                                    "X-Custom", List.of("a", "b"),
                                    "Content-Length", List.of("1"),
                                    "Connection", List.of("close"),
                                    "Access-Control-Allow-Origin", List.of("https://evil.example"),
                                    "Content-Type", List.of("text/plain")), "hi".getBytes());
                        case "GET /ctx":
                            return Result.json(200, "{\\"greeting\\":\\"" + ctx.config().get("greeting").orElse("-")
                                    + "\\",\\"undeclared\\":\\"" + ctx.config().get("other").orElse("-")
                                    + "\\",\\"key\\":\\"" + ctx.secrets().get("key").orElse("-")
                                    + "\\",\\"address\\":\\"" + ctx.address() + "\\",\\"version\\":" + ctx.version() + "}");
                        case "POST /emit": {
                            EmitResult r = ctx.events().emit(new OutboundEvent(in.queryParam("type").orElse("app:orders:order:created"),
                                    null, "order-1", "application/json", in.body(), null, null, null, "dedup-1"));
                            return switch (r) {
                                case EmitResult.Emitted e -> Result.json(200, "{\\"eventId\\":\\"" + e.eventId() + "\\"}");
                                case EmitResult.Refused f -> Result.json(502, "{\\"code\\":\\"" + f.code() + "\\",\\"status\\":" + f.status() + "}");
                            };
                        }
                        case "POST /dispatch": {
                            try {
                                String id = ctx.dispatcher().dispatch(DispatchJob.to(in.queryParam("target").orElse("app.other"), in.queryParam("path").orElse("/hook"))
                                        .payloadJson(in.bodyText()).messageGroup("g1").metadata("k", "v").build());
                                return Result.json(200, "{\\"jobId\\":\\"" + id + "\\"}");
                            } catch (DispatchException e) {
                                return Result.json(502, "{\\"code\\":\\"" + e.code() + "\\",\\"retryable\\":" + e.retryable() + "}");
                            }
                        }
                        default:
                            return Result.status(418);
                    }
                }
                private static String callerOf(Request in) {
                    if (in.caller() instanceof Caller.Platform) return "platform";
                    if (in.caller() instanceof Caller.Principal p) {
                        return "principal:" + p.id() + ":" + p.type() + ":" + p.tier() + ":" + String.join("|", p.clients())
                                + ":" + String.join("|", p.clientCodes()) + ":" + p.allApplications() + ":" + new TreeSet<>(p.permissions());
                    }
                    return "anonymous";
                }
            }
            """;

    static Fx.Jar jar() {
        return Fx.jar(ENTRYPOINT, SOURCE);
    }

    /// The describe: every route of the function, each with the auth the listener tests need.
    static String describe(String... extra) {
        return Fx.describe(ENTRYPOINT, List.of(
                Fx.endpoint("GET", "/hello/{name}", "none"),
                Fx.endpoint("POST", "/hook", "webhook"),
                Fx.endpoint("GET", "/me", "platform"),
                Fx.endpoint("POST", "/echo", "none"),
                Fx.endpoint("GET", "/boom", "none"),
                Fx.endpoint("GET", "/error", "none"),
                Fx.endpoint("", "/wh", "webhook"),
                Fx.endpoint("GET", "/sleep/{ms}", "none"),
                Fx.endpoint("GET", "/stubborn/{ms}", "none"),
                Fx.endpoint("GET", "/status/{code}", "none"),
                Fx.endpoint("GET", "/headers", "none"),
                Fx.endpoint("GET", "/ctx", "none"),
                Fx.endpoint("POST", "/emit", "none"),
                Fx.endpoint("POST", "/dispatch", "none"),
                "{\"method\":\"POST\",\"path\":\"/small\",\"auth\":\"none\",\"maxBodyBytes\":4}",
                "{\"method\":\"POST\",\"path\":\"/quick\",\"auth\":\"none\",\"timeoutMs\":100}"),
                extra);
    }
}
