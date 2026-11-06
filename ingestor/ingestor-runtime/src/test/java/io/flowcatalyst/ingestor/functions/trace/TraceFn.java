package io.flowcatalyst.ingestor.functions.trace;

import io.flowcatalyst.ingestor.functions.Fx;
import java.util.List;

/// A function jar for the trace tests: routes chosen by `in.route()` that log, call a vendor over HTTP (the CTrack shape: a
/// login by `GET ?username=&password=` whose answer carries a session that later calls put in the URL's path), write trace
/// records and fail in the ways the listener maps.
final class TraceFn {

    static final String ENTRYPOINT = "fixture.app.TraceFn";

    private TraceFn() {
    }

    // JSON is written with single quotes and turned into double quotes by q(), so the source needs no escapes.
    static final String SOURCE = """
            package fixture.app;
            import io.flowcatalyst.function.*;
            import java.lang.System.Logger.Level;
            import java.nio.charset.StandardCharsets;
            import java.util.*;
            public final class TraceFn implements Function {
                private String initSession = "";
                static String q(String s) { return s.replace('\\'', '"'); }
                static HttpReply get(FunctionContext ctx, String url) throws Exception {
                    return ctx.http().send(new HttpCall("GET", url, Map.of(), new byte[0]));
                }
                static String field(HttpReply r, String name) {
                    String body = new String(r.body(), StandardCharsets.UTF_8);
                    int at = body.indexOf("\\"" + name + "\\":\\"");
                    if (at < 0) return "";
                    int from = at + name.length() + 4;
                    return body.substring(from, body.indexOf('"', from));
                }
                public void init(FunctionContext ctx) throws Exception {
                    if (ctx.config().get("loginInInit").isPresent()) {
                        String base = ctx.config().require("base");
                        initSession = field(get(ctx, base + "/Membership/Login?username=u&password=" + ctx.config().require("password")), "SessionToken");
                        ctx.conceal(initSession);
                    }
                }
                public Result handle(Request in, FunctionContext ctx) throws Exception {
                    String base = ctx.config().require("base");
                    switch (in.route()) {
                        case "GET /ok": {
                            ctx.logger().log(Level.INFO, "hello from ok");
                            ctx.logger().log(Level.DEBUG, "a debug line");
                            ctx.logger().log(Level.WARNING, "careful {0}", "now");
                            get(ctx, base + "/REST/Items?limit=10&api_key=zzz-key-value");
                            return Result.http(200, Map.of("Content-Type", List.of("application/json"), "X-Reply", List.of("r1")), q("{'ok':true}").getBytes(StandardCharsets.UTF_8));
                        }
                        case "POST /echo":
                            return Result.http(201, Map.of("Content-Type", List.of("text/plain")), in.body());
                        case "GET /boom":
                            ctx.logger().log(Level.ERROR, "about to fail");
                            throw new IllegalStateException("kaput");
                        case "GET /sleep":
                            Thread.sleep(5000);
                            return Result.ack();
                        case "GET /pause/{ms}":
                            Thread.sleep(Long.parseLong(in.pathParams().get("ms")));
                            return Result.ack();
                        case "GET /badheader":
                            return Result.http(200, Map.of("Bad Header Name", List.of("x")), new byte[0]);
                        case "GET /ctrack": {
                            HttpReply login = get(ctx, base + "/Membership/Login?username=u&password=" + ctx.config().require("password"));
                            String session = field(login, "SessionToken");
                            HttpReply v = get(ctx, base + "/Vehicles/" + session + "/GetVehicles");
                            return Result.json(200, q("{'session':'") + session + q("','vehicles':") + new String(v.body(), StandardCharsets.UTF_8) + "}");
                        }
                        case "GET /ctrack-init": {
                            HttpReply v = get(ctx, base + "/Vehicles/" + initSession + "/GetVehicles");
                            return Result.json(200, q("{'n':1}"));
                        }
                        case "GET /secret": {
                            String s = ctx.secrets().require("key");
                            ctx.logger().log(Level.INFO, "using " + s);
                            ctx.trace("sync", "msg " + s, Map.of("k", s, "list", List.of(s, "x")));
                            return Result.json(200, q("{'echo':'") + s + q("'}"));
                        }
                        case "GET /author": {
                            boolean before = ctx.tracing("sync");
                            ctx.trace("sync", "first");
                            ctx.setReference("order-77");
                            ctx.trace("sync", "second", Map.of("n", 1));
                            ctx.trace("sync", "third", null, new TraceOptions("warn", "own-ref"));
                            ctx.trace("sync", "fourth", null, new TraceOptions("bogus", null));
                            ctx.trace("nope", "undeclared");
                            ctx.trace("nope", "again");
                            ctx.trace("http", "builtin");
                            return Result.json(200, q("{'syncBefore':" + before + ",'http':" + ctx.tracing("http") + ",'nope':" + ctx.tracing("nope") + "}"));
                        }
                        case "GET /big": {
                            ctx.trace("sync", "too big", Map.of("blob", "x".repeat(40000)));
                            ctx.trace("sync", "small");
                            return Result.ack();
                        }
                        case "GET /flood": {
                            for (int i = 0; i < 600; i++) {
                                ctx.logger().log(Level.INFO, "line " + i);
                                ctx.trace("sync", "trace " + i);
                            }
                            for (int i = 0; i < 120; i++) {
                                get(ctx, base + "/REST/Items?limit=" + i);
                            }
                            return Result.ack();
                        }
                        default:
                            return Result.status(418);
                    }
                }
            }
            """;

    static Fx.Jar jar() {
        return Fx.jar(ENTRYPOINT, SOURCE);
    }

    static String describe() {
        return Fx.describe(ENTRYPOINT, List.of(
                        Fx.endpoint("GET", "/ok", "none"),
                        Fx.endpoint("POST", "/echo", "none"),
                        Fx.endpoint("GET", "/boom", "none"),
                        Fx.endpoint("GET", "/sleep", "none"),
                        Fx.endpoint("GET", "/pause/{ms}", "none"),
                        Fx.endpoint("GET", "/badheader", "none"),
                        Fx.endpoint("GET", "/ctrack", "none"),
                        Fx.endpoint("GET", "/ctrack-init", "none"),
                        Fx.endpoint("GET", "/secret", "none"),
                        Fx.endpoint("GET", "/author", "none"),
                        Fx.endpoint("GET", "/big", "none"),
                        Fx.endpoint("GET", "/flood", "none")),
                "\"config\":[\"base\",\"password\",\"loginInInit\"]",
                "\"secrets\":[\"key\"]",
                "\"httpAllow\":[\"127.0.0.1\"]",
                "\"traceContexts\":[{\"name\":\"sync\",\"description\":\"the sync steps\"}]");
    }
}
