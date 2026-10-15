// payload-logger: the delivery target for dispatch-creator's jobs. The
// platform POSTs each job's payload here (the job is dataOnly, so the body is
// exactly what dispatch-creator was given), signed; the runner verifies the
// signature (webhook auth) before this handler runs. It logs the payload and
// acks.
//
// It declares no subscription: nothing raises an event for it any more. The
// job is created directly by dispatch-creator (fn.dispatch), which names this
// function's address and this path.
import { fn, type FnRequest } from "@flowcatalyst/fn";

fn.webhook("POST /deliver", (req: FnRequest) => {
  fn.log.info("payload received", { payload: req.text(), invocation: req.invocation.id });
  return fn.json(200, { ack: true });
});
