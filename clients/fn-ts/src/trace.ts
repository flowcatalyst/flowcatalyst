// Trace logs (host ops 12 trace / 13 tracing; docs/function-trace-design.md
// sections 2 and 9), mirroring clients/fn-go/trace.go. A context is a named
// trace point the function declares with traceContexts(); it records nothing
// unless an operator has switched it on for a limited time. trace() and
// tracing() NEVER throw: a runner without the ops (UNKNOWN_OP), NOT_DECLARED,
// TOO_LARGE or any other host failure is swallowed, so tracing can never fail
// an invocation. NOT_DECLARED additionally writes one warning per context to
// stderr, since it is the author's mistake.
import { ErrCodeNotDeclared, FnError } from "./errors";
import { callHost, utf8Decode, utf8Encode } from "./native";
import { OpTrace, OpTracing } from "./ops";
import { registry } from "./registry";

export type TraceLevel = "debug" | "info" | "warn" | "error";

export interface TraceOptions {
  /** One of debug, info, warn, error. Default info. */
  level?: TraceLevel;
  /** The business key (trip number, order number) for this record; overrides
   * the invocation's setReference() value for this call only. */
  reference?: string;
}

/** One trace context declaration. */
export interface TraceContext {
  /** Lowercase letter first, then lowercase letters, digits or `-`; at most
   * 40 characters; not `http` (the built-in capture context). */
  name: string;
  /** What the context records, shown to operators. At most 200 characters. */
  description?: string;
}

// Limits (internal/functions/abi: TraceMaxMessage, TraceMaxReference,
// MaxTraceContexts, MaxTraceContextDescLength).
const MAX_MESSAGE_BYTES = 4096;
const MAX_REFERENCE_CHARS = 200;
const MAX_CONTEXTS = 20;
const MAX_DESCRIPTION_CHARS = 200;
const CONTEXT_NAME = /^[a-z][a-z0-9-]{0,39}$/;

/** Declares the trace contexts this function writes to (describe key
 * `traceContexts`). Call it at module top level. Throws at registration, like
 * abi.Describe.Validate would at publish, so a bad declaration fails at
 * describe time with a clear message. */
export function traceContexts(...contexts: TraceContext[]): void {
  for (const c of contexts) {
    const at = `traceContexts[${registry.traceContexts.length}]`;
    if (c.name === "http") {
      throw new Error(`fn: ${at}: "http" is the built-in capture context and cannot be declared`);
    }
    if (typeof c.name !== "string" || !CONTEXT_NAME.test(c.name)) {
      throw new Error(
        `fn: ${at}: ${JSON.stringify(c.name)} is not a valid context name (lowercase letter first, then lowercase letters, digits or -; at most 40 characters)`,
      );
    }
    if (c.description !== undefined && c.description.length > MAX_DESCRIPTION_CHARS) {
      throw new Error(`fn: ${at}: description is over ${MAX_DESCRIPTION_CHARS} characters`);
    }
    if (registry.traceContexts.some((x) => x.name === c.name)) {
      throw new Error(`fn: ${at}: ${JSON.stringify(c.name)} is declared twice`);
    }
    if (registry.traceContexts.length >= MAX_CONTEXTS) {
      throw new Error(`fn: traceContexts: at most ${MAX_CONTEXTS} contexts`);
    }
    const d: TraceContext = { name: c.name };
    if (c.description) d.description = c.description;
    registry.traceContexts.push(d);
  }
}

// One stderr warning per undeclared context for the life of the instance.
const warned = new Set<string>();

/** Forgets which NOT_DECLARED warnings were written. Test-only. */
export function resetTraceWarnings(): void {
  warned.clear();
}

function warnNotDeclared(context: string): void {
  if (warned.has(context)) return;
  warned.add(context);
  try {
    // QuickJS has no process; console.error is routed through host op 1, which
    // is the function's stderr as far as the platform is concerned.
    console.error(`fn: trace context ${JSON.stringify(context)} is not declared; add it with traceContexts() (records for it are dropped)`);
  } catch {
    // ignored
  }
}

// Cuts s to at most maxBytes of UTF-8 on a character boundary.
function clipUtf8(s: string, maxBytes: number): string {
  if (s.length * 3 <= maxBytes) return s;
  if (utf8Encode(s).length <= maxBytes) return s;
  const cut = utf8Decode(utf8Encode(s).subarray(0, maxBytes));
  // A cut through a multi-byte character decodes to a trailing U+FFFD.
  return cut.endsWith("�") ? cut.slice(0, -1) : cut;
}

/** The per-invocation trace handle a request carries (req.trace, req.tracing,
 * req.setReference). */
export interface Tracer {
  /** Records `message` (and optional JSON-object `data`) under `context`. A
   * silent no-op unless an operator has switched the context on and this
   * invocation was sampled in. Fire and forget; never throws. */
  trace(context: string, message: string, data?: Record<string, unknown>, options?: TraceOptions): void;
  /** True when `context` is switched on now and this invocation was sampled
   * in. Ask before building an expensive payload. Never throws (false on any
   * failure). The answer is not cached: it depends on the invocation. */
  tracing(context: string): boolean;
  /** Stamps later trace calls of this invocation with `ref`, unless a call
   * passes its own reference. */
  setReference(ref: string): void;
}

/** Creates the tracer for one invocation. */
export function newTracer(): Tracer {
  let reference = "";
  return {
    trace(context, message, data, options) {
      try {
        const meta: Record<string, unknown> = { context, msg: clipUtf8(String(message), MAX_MESSAGE_BYTES) };
        if (options?.level) meta.level = options.level;
        if (data !== undefined) meta.data = data;
        const ref = options?.reference ?? reference;
        if (ref) meta.reference = ref.slice(0, MAX_REFERENCE_CHARS);
        callHost(OpTrace, meta);
      } catch (err) {
        if (err instanceof FnError && err.code === ErrCodeNotDeclared) warnNotDeclared(context);
      }
    },
    tracing(context) {
      try {
        return callHost(OpTracing, { context }).meta.live === true;
      } catch (err) {
        if (err instanceof FnError && err.code === ErrCodeNotDeclared) warnNotDeclared(context);
        return false;
      }
    },
    setReference(ref) {
      reference = String(ref);
    },
  };
}
