// payload-logger: the delivery target. The platform POSTs each dispatched
// event's data (dataOnly) here, signed; the runner verifies the signature
// (webhook auth) before this handler runs. It logs the payload and acks.
import { fn, type FnRequest } from "@flowcatalyst/fn";
import { DISPATCH_REQUESTED } from "./eventType";

fn.webhook("POST /deliver", (req: FnRequest) => {
  fn.log.info("payload received", { payload: req.text(), invocation: req.invocation.id });
  return fn.json(200, { ack: true });
});

// dataOnly: the body is the event's data, i.e. exactly what was POSTed to
// dispatch-creator. IMMEDIATE: the demo has no ordering requirement.
fn.subscribe(DISPATCH_REQUESTED, "/deliver", { mode: "IMMEDIATE", dataOnly: true });
