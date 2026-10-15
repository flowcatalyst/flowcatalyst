// Dispatch (plan §5.3 op 11, dispatch.create), mirroring clients/fn-go/job.go.
// Creates a dispatch job on the platform, delivered to a function of the SAME
// application that this function declared with dispatches(). The function
// names the target and path; it never supplies a URL or a credential, and the
// platform builds the delivery URL and signs it as the application.
import { registry } from "./registry";
import { callHost, utf8Encode } from "./native";
import { OpDispatch } from "./ops";

/** One metadata entry. The platform stores metadata as an array of these. */
export interface MetadataEntry {
  key: string;
  value: string;
}

export interface DispatchInput {
  /** Address of a function in this function's application, declared with dispatches(). */
  target: string;
  /** Literal webhook path on the target, declared with dispatches(). */
  path: string;
  /** The job's payload: delivered as the request body (see dataOnly). A plain
   * object/array is JSON-encoded (and, unless contentType is set, gets
   * `application/json`). */
  payload?: string | Uint8Array | Record<string, unknown> | unknown[];
  /** Defaults to `<application>:function:<name>:dispatch`. Must start with the function's application. */
  code?: string;
  subject?: string;
  /** A plain object (sorted by key) or the platform's array of {key, value}.
   * Keys starting with `fn.` are the platform's and are refused. */
  metadata?: Record<string, string> | MetadataEntry[];
  /** Jobs of one group are delivered in order, per mode. */
  messageGroup?: string;
  /** IMMEDIATE, NEXT_ON_ERROR or BLOCK_ON_ERROR; default is the platform's. */
  mode?: string;
  /** Defaults to the calling request's X-Correlation-Id, if it had one. */
  correlationId?: string;
  causationId?: string;
  contentType?: string;
  /** What the job is, in words, for the dispatch-jobs grid. */
  descriptor?: string;
  maxRetries?: number;
  timeoutSeconds?: number;
  /** Deliver the payload alone as the body (true, the default) or wrapped in the platform's envelope. */
  dataOnly?: boolean;
}

/** Normalises metadata to the platform's array of {key, value}. */
export function metadataEntries(m: DispatchInput["metadata"]): MetadataEntry[] | undefined {
  if (m === undefined) return undefined;
  if (Array.isArray(m)) return m.map((e) => ({ key: e.key, value: e.value }));
  return Object.keys(m)
    .sort()
    .map((key) => ({ key, value: String(m[key]) }));
}

/** Creates the job and returns its id. (target, path) must have been declared
 * with dispatches(). Throws FnError; isRetryable(err) is true for platform
 * trouble (UNAVAILABLE), whereas NOT_DECLARED / NOT_ALLOWED / BAD_REQUEST are
 * permanent refusals: undeclared target, a target the platform will not
 * deliver to (other application, not live, no webhook endpoint at the path),
 * or an invalid field.
 *
 * A dispatch is NOT idempotent: the platform does not de-duplicate jobs, so
 * retrying after an ambiguous failure can create a second job. */
export async function dispatch(d: DispatchInput): Promise<string> {
  let contentType = d.contentType;
  let bodyBytes: Uint8Array | undefined;
  if (d.payload === undefined) {
    bodyBytes = undefined;
  } else if (d.payload instanceof Uint8Array) {
    bodyBytes = d.payload;
  } else if (typeof d.payload === "string") {
    bodyBytes = utf8Encode(d.payload);
  } else {
    bodyBytes = utf8Encode(JSON.stringify(d.payload));
    if (!contentType) contentType = "application/json";
  }
  const meta = {
    target: d.target,
    path: d.path,
    code: d.code,
    subject: d.subject,
    metadata: metadataEntries(d.metadata),
    messageGroup: d.messageGroup,
    mode: d.mode,
    correlationId: d.correlationId,
    causationId: d.causationId,
    contentType,
    descriptor: d.descriptor,
    maxRetries: d.maxRetries,
    timeoutSeconds: d.timeoutSeconds,
    dataOnly: d.dataOnly,
  };
  const { meta: rm } = callHost(OpDispatch, meta, bodyBytes);
  return typeof rm.jobId === "string" ? rm.jobId : "";
}

/** Declares a (function, path) this function may dispatch jobs to with
 * dispatch(). The target must be a function of the same application. */
export function dispatches(target: string, path: string): void {
  registry.dispatches.push({ target, path });
}
