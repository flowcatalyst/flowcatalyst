// Emit (plan §5.3 op 5, event.emit), mirroring clients/fn-go/event.go.
import { registry } from "./registry";
import { callHost, utf8Encode } from "./native";
import { OpEventEmit } from "./ops";

export interface EventInput {
  type: string;
  source?: string;
  subject?: string;
  dedupId?: string;
  correlationId?: string;
  causationId?: string;
  messageGroup?: string;
  contentType?: string;
  /** A plain object/array is JSON-encoded automatically (and, unless
   * contentType is set explicitly, gets `application/json`). */
  data?: string | Uint8Array | Record<string, unknown> | unknown[];
}

/** Publishes e and returns the platform-assigned event id. type must be one
 * declared with emits(). Throws FnError; isRetryable(err) distinguishes
 * platform trouble (UNAVAILABLE, safe to retry) from a permanent refusal
 * such as NOT_ALLOWED (the type isn't in emits()/isn't owned by the
 * function's application). */
export async function emit(e: EventInput): Promise<string> {
  let contentType = e.contentType;
  let bodyBytes: Uint8Array | undefined;
  if (e.data === undefined) {
    bodyBytes = undefined;
  } else if (e.data instanceof Uint8Array) {
    bodyBytes = e.data;
  } else if (typeof e.data === "string") {
    bodyBytes = utf8Encode(e.data);
  } else {
    bodyBytes = utf8Encode(JSON.stringify(e.data));
    if (!contentType) contentType = "application/json";
  }
  const meta = {
    type: e.type,
    source: e.source,
    subject: e.subject,
    dedupId: e.dedupId,
    correlationId: e.correlationId,
    causationId: e.causationId,
    messageGroup: e.messageGroup,
    contentType,
  };
  const { meta: rm } = callHost(OpEventEmit, meta, bodyBytes);
  return typeof rm.eventId === "string" ? rm.eventId : "";
}

/** Declares event types this function may publish via emit(). */
export function emits(...eventTypes: string[]): void {
  registry.emits.push(...eventTypes);
}
