// Response builders (plan §13.2 for retry/reject; mirrors
// clients/fn-go/respond.go). Handlers return these plain objects rather
// than writing through a ResponseWriter — there is no imperative writer in
// this SDK, only a value the dispatcher turns into a response frame.
import type { FnResponseLike, ResponseBody } from "./types";

export function json(status: number, value: unknown): FnResponseLike {
  return { status, headers: { "Content-Type": "application/json" }, body: value as ResponseBody };
}

export function text(status: number, s: string): FnResponseLike {
  return { status, headers: { "Content-Type": "text/plain; charset=utf-8" }, body: s };
}

export function status(statusCode: number): FnResponseLike {
  return { status: statusCode };
}

/** Answers 429 with a Retry-After header. The dispatch-job / scheduled-job
 * delivery paths treat this as a deferral that spends no retry budget
 * (plan §6.2 step 5, §13.3). */
export function retry(seconds: number): FnResponseLike {
  const secs = Math.max(Math.floor(seconds), 1);
  return { status: 429, headers: { "Retry-After": String(secs) } };
}

export const OUTCOME_HEADER = "FlowCatalyst-Outcome";
export const OUTCOME_REJECT = "reject";

/** Answers 422 with the FlowCatalyst-Outcome: reject header, the terminal
 * "don't retry" outcome (plan §13.2). reason is written as a plain-text
 * body for diagnostics (no JSON envelope, matching clients/fn-go/respond.go);
 * an empty/omitted reason writes no body. */
export function reject(reason?: string): FnResponseLike {
  const r: FnResponseLike = { status: 422, headers: { [OUTCOME_HEADER]: OUTCOME_REJECT } };
  if (reason) r.body = reason;
  return r;
}
