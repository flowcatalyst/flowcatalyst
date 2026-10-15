// dispatch-creator: POST /dispatch with a JSON body (platform-authenticated)
// emits one event carrying that body. A subscription owned by the
// payload-logger function turns the event into a dispatch job and delivers it.
//
// Why emit rather than call the dispatch-job API: a function holds no
// platform credential (plan 6.3). Its only write path into the platform is
// the `event.emit` capability, which the runner performs on its behalf and
// the platform checks against this version's declared `emits`. The dispatch
// job is then created by the platform's normal subscription path, so
// clientCode / source / metadata rules are the platform's, not ours.
import { fn, FnError, isRetryable, type FnRequest } from "@flowcatalyst/fn";
import { DISPATCH_REQUESTED, REQUIRED_PERMISSION } from "./eventType";

fn.emits(DISPATCH_REQUESTED);

fn.open("GET /healthz", () => fn.json(200, { status: "ok" }));

fn.platform("POST /dispatch", async (req: FnRequest) => {
  // auth "platform": the runner has already verified the bearer token (401
  // otherwise). Authorisation is the function's own job: the verified
  // principal must hold the permission.
  const principal = req.caller.principal;
  if (!principal || !principal.hasPermission(REQUIRED_PERMISSION)) {
    return fn.json(403, { error: "FORBIDDEN", message: `requires permission ${REQUIRED_PERMISSION}` });
  }

  let payload: unknown;
  try {
    payload = req.json();
  } catch {
    return fn.json(400, { error: "INVALID_JSON" });
  }
  if (payload === null || typeof payload !== "object") {
    return fn.json(400, { error: "PAYLOAD_MUST_BE_A_JSON_OBJECT_OR_ARRAY" });
  }

  // A caller-supplied Idempotency-Key makes a retried request create one
  // event; otherwise the invocation id keeps a runner-level retry of the
  // same invocation from duplicating.
  const key = req.headers.get("Idempotency-Key") || req.invocation.id;

  try {
    const eventId = await fn.emit({
      type: DISPATCH_REQUESTED,
      dedupId: `dispatch-demo-${key}`,
      messageGroup: "dispatch-demo",
      contentType: "application/json",
      data: payload as Record<string, unknown>,
    });
    fn.log.info("dispatch requested", { eventId });
    return fn.json(202, { eventId });
  } catch (err) {
    if (isRetryable(err)) return fn.json(503, { error: "PLATFORM_UNAVAILABLE" });
    fn.log.error("emit refused", { error: err instanceof FnError ? `${err.code}: ${err.message}` : String(err) });
    return fn.json(502, { error: "EMIT_REFUSED" });
  }
});
