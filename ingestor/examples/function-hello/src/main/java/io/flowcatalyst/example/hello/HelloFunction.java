package io.flowcatalyst.example.hello;

import io.flowcatalyst.framework.http.Json;
import io.flowcatalyst.function.Caller;
import io.flowcatalyst.function.EmitResult;
import io.flowcatalyst.function.Event;
import io.flowcatalyst.function.Function;
import io.flowcatalyst.function.FunctionContext;
import io.flowcatalyst.function.OutboundEvent;
import io.flowcatalyst.function.Request;
import io.flowcatalyst.function.Result;
import io.flowcatalyst.function.Webhook;
import java.lang.System.Logger.Level;
import java.time.Duration;
import java.util.Map;
import java.util.UUID;
import tools.jackson.databind.JsonNode;

/// The hello function (ported from flowcatalyst-javalin's examples/function-hello): the endpoints
/// declared in META-INF/flowcatalyst/function.json, one handler. The host routes and authenticates
/// before `handle` runs, so a path here is always one the describe declares. One instance serves
/// every call at once, each on its own virtual thread: keep it free of mutable state.
public final class HelloFunction implements Function {

    private static final String SENT = "hello:greeting:greeting:sent";

    @Override
    public void init(FunctionContext ctx) {
        ctx.logger().log(Level.INFO, "hello function starting (address={0}, version={1})", ctx.address(), ctx.version());
    }

    @Override
    public Result handle(Request in, FunctionContext ctx) throws Exception {
        return switch (in.route()) {
            case "GET /healthz" -> Result.json(200, "{\"status\":\"ok\"}");
            case "GET /hello/{name}" -> hello(in);
            case "POST /events/greeting-requested" -> greetingRequested(in, ctx);
            default -> Result.fail("no handler for " + in.route());
        };
    }

    /// A platform caller (a FlowCatalyst token) is greeted by name.
    private static Result hello(Request in) {
        if (!(in.caller() instanceof Caller.Principal principal)) {
            return Result.json(401, "{\"error\":\"UNAUTHENTICATED\"}");
        }
        String name = in.pathParams().getOrDefault("name", "world");
        return Result.json(200, Json.mapper().writeValueAsString(Map.of("message", "hello, " + name + "!", "principalId", principal.id())));
    }

    /// The subscription's delivery: a greeting requested becomes a greeting sent.
    private static Result greetingRequested(Request in, FunctionContext ctx) {
        Event event = Webhook.event(in);
        String greeting = ctx.config().get("GREETING").orElse("Hello");
        JsonNode data = event.dataJson() == null ? null : Json.mapper().readTree(event.dataJson());
        String name = data != null && data.hasNonNull("name") ? data.get("name").asString() : "world";
        byte[] sent = Json.mapper().writeValueAsBytes(Map.of("name", name, "greeting", greeting + ", " + name + "!"));
        OutboundEvent outbound = new OutboundEvent(
                SENT, "function:" + ctx.address().render(), event.subject(), "application/json", sent,
                event.correlationId(), event.id(), event.messageGroup(), UUID.randomUUID().toString());
        return switch (ctx.events().emit(outbound)) {
            case EmitResult.Emitted emitted -> Result.ack();
            case EmitResult.Refused refused when refused.retryable() -> Result.retry(Duration.ofSeconds(5));
            case EmitResult.Refused refused -> Result.reject("emit refused: " + refused.code());
        };
    }
}
