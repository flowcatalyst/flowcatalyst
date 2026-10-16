// The minimal FlowCatalyst function example for the TypeScript/JavaScript
// guest SDK, mirroring clients/fn-go/examples/hello and
// clients/fn-rust/examples/hello exactly: an open health check, a
// platform-authenticated greeting that reads a declared config value and
// the caller's principal, and a webhook endpoint that emits an event.
//
// Build it with (from clients/fn-ts):
//
//   npm run build:hello
import { fn, FnError, isRetryable, type FnRequest } from "../../../src/index";

const greeting = fn.config("GREETING");

fn.open("GET /healthz", () => fn.json(200, { status: "ok" }));

fn.platform("GET /hello/{name}", (req: FnRequest) => {
  let g: string;
  try {
    g = greeting.get() ?? "Hello";
  } catch (err) {
    fn.log.error("config.get failed", { error: err instanceof Error ? err.message : String(err) });
    return fn.json(500, { error: "CONFIG_UNAVAILABLE" });
  }

  const body: Record<string, unknown> = { message: `${g}, ${req.params.name}` };
  if (req.caller.principal) body.caller = req.caller.principal.id;
  return fn.json(200, body);
});

fn.webhook("POST /events/greeting", async (req: FnRequest) => {
  // Derive the dedup id from the delivered event's id, so a redelivery of
  // the same event emits the same follow-up exactly once.
  let inboundId = "";
  try {
    const ev = req.json() as { id?: string };
    inboundId = ev.id ?? "";
  } catch {
    // malformed body; fall through to the invocation id below
  }
  if (!inboundId) inboundId = req.invocation.id;

  try {
    const eventId = await fn.emit({
      type: "hello:greeting:greeting:sent",
      dedupId: `greeting-sent-${inboundId}`,
      source: "hello.example",
      contentType: "application/json",
      data: { ok: true },
    });
    fn.log.info("greeting event emitted", { eventId });
    return fn.status(202);
  } catch (err) {
    if (isRetryable(err)) return fn.retry(5);
    fn.log.error("emit failed", { error: err instanceof FnError ? err.message : String(err) });
    return fn.reject("emit refused");
  }
});

// POST /dispatch creates a dispatch job for the hello.logger function's
// /deliver webhook. The target and path are declared below; the platform
// builds the URL and signs the delivery, the function holds no credential.
fn.open("POST /dispatch", async (req: FnRequest) => {
  try {
    const jobId = await fn.dispatch({
      target: "hello.logger",
      path: "/deliver",
      payload: { hello: "world" },
      metadata: { from: "hello" }, // an object is fine; it travels as [{key,value}]
      messageGroup: "hello",
    });
    fn.log.info("job dispatched", { jobId, invocation: req.invocation.id });
    return fn.json(202, { jobId });
  } catch (err) {
    if (isRetryable(err)) return fn.retry(5);
    fn.log.error("dispatch failed", { error: err instanceof FnError ? err.message : String(err) });
    return fn.reject("dispatch refused");
  }
});

fn.emits("hello:greeting:greeting:sent");
fn.dispatches("hello.logger", "/deliver", { pool: "hello-pool", connection: "hello-conn" });
