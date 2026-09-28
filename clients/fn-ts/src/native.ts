// The seam between portable SDK logic and the engine's natives
// (clients/fn-js-engine/src/lib.rs's module doc): `__fc_call`,
// `__fc_utf8_encode`, `__fc_utf8_decode`. Everything else in this package
// reaches the host only through `callHost` below, so it is exercised on a
// normal `node --test` by setting `globalThis.__fc_call` to a fake before a
// test runs (see testing.ts) — no QuickJS runtime needed, mirroring the Go
// SDK's FakeHost / the Rust SDK's FakeHost closure.
import { ErrCodeUnavailable, FnError } from "./errors";

export interface NativeCallResult {
  code: number;
  meta: string;
  body: Uint8Array;
}

type NativeCallFn = (op: number, meta: string, body?: Uint8Array) => NativeCallResult;
type NativeEncodeFn = (s: string) => Uint8Array;
type NativeDecodeFn = (b: Uint8Array) => string;

const g = globalThis as unknown as {
  __fc_call?: NativeCallFn;
  __fc_utf8_encode?: NativeEncodeFn;
  __fc_utf8_decode?: NativeDecodeFn;
};

const EMPTY = new Uint8Array(0);

export interface HostResult {
  meta: Record<string, unknown>;
  body: Uint8Array;
}

/**
 * Runs host capability `op` with `meta` (JSON-encoded) and an optional body.
 * Throws FnError when the host reports a capability failure (plan §5.3: "a
 * capability failure is never a trap"). Not part of the ABI's error-code
 * vocabulary (e.g. `__fc_call` missing entirely) is also surfaced as an
 * UNAVAILABLE FnError, since there is nothing else useful to do with it.
 */
export function callHost(op: number, meta: unknown, body?: Uint8Array): HostResult {
  if (typeof g.__fc_call !== "function") {
    throw new FnError(ErrCodeUnavailable, "the __fc_call native is not installed", true);
  }
  const metaStr = JSON.stringify(meta ?? {});
  const result = g.__fc_call(op, metaStr, body);
  const resultMeta = safeParseObject(result.meta);
  if (result.code !== 0) {
    const code = typeof resultMeta.code === "string" ? resultMeta.code : "UNKNOWN";
    const message = typeof resultMeta.message === "string" ? resultMeta.message : "";
    throw new FnError(code, message, code === ErrCodeUnavailable);
  }
  return { meta: resultMeta, body: result.body ?? EMPTY };
}

function safeParseObject(s: string | undefined): Record<string, unknown> {
  if (!s) return {};
  try {
    const v = JSON.parse(s);
    return v && typeof v === "object" ? (v as Record<string, unknown>) : {};
  } catch {
    return {};
  }
}

/**
 * Encodes `s` as UTF-8. QuickJS has no TextEncoder, so this always goes
 * through the engine's `__fc_utf8_encode` native in production; the
 * TextEncoder fallback exists only so the SDK also runs under plain
 * `node --test`, where that native is never installed.
 */
export function utf8Encode(s: string): Uint8Array {
  if (typeof g.__fc_utf8_encode === "function") return g.__fc_utf8_encode(s);
  return new TextEncoder().encode(s);
}

/** Decodes UTF-8 bytes to a string; see utf8Encode for the fallback note. */
export function utf8Decode(b: Uint8Array | undefined): string {
  if (!b || b.length === 0) return "";
  if (typeof g.__fc_utf8_decode === "function") return g.__fc_utf8_decode(b);
  return new TextDecoder().decode(b);
}
