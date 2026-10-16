// dispatch-creator: POST /dispatch with a JSON body (platform-authenticated)
// creates one DISPATCH JOB that delivers that body to the payload-logger
// function's /deliver webhook.
//
// A function holds no platform credential (plan 6.3), so it cannot call the
// dispatch-job API. It uses the `dispatch` capability instead: it declares the
// one (function, path) it may dispatch to, names that target in fn.dispatch,
// and the runner asks the platform to create the job on its behalf. The
// platform checks the target against this version's declared `dispatches`,
// requires it to be a live function of the SAME application with a webhook
// endpoint at the path, builds the delivery URL and signs the delivery as the
// declared connection's service account (the job carries the connection, so
// pausing it holds the jobs). clientCode, source and the metadata rules
// are the platform's: the job is recorded as kind TASK, source
// "function:<this address>", with fn.* provenance metadata.
import { fn, FnError, isRetryable, type FnRequest } from "@flowcatalyst/fn";
import { CONNECTION_CODE, DELIVER_PATH, LOGGER_ADDRESS, REQUIRED_PERMISSION } from "./names";

fn.dispatches(LOGGER_ADDRESS, DELIVER_PATH, { connection: CONNECTION_CODE });

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

  try {
    const jobId = await fn.dispatch({
      target: LOGGER_ADDRESS,
      path: DELIVER_PATH,
      payload: payload as Record<string, unknown>,
      messageGroup: "dispatch-demo",
      descriptor: "dispatch-demo payload",
      // An object is fine: it travels as the platform's [{key, value}] array.
      metadata: { requestedBy: principal.id },
    });
    fn.log.info("job dispatched", { jobId });
    return fn.json(202, { jobId });
  } catch (err) {
    if (isRetryable(err)) return fn.json(503, { error: "PLATFORM_UNAVAILABLE" });
    fn.log.error("dispatch refused", { error: err instanceof FnError ? `${err.code}: ${err.message}` : String(err) });
    return fn.json(502, { error: "DISPATCH_REFUSED" });
  }
});
