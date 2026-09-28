// Test-only helpers: a settable fake `__fc_call` host, mirroring
// clients/fn-go's FakeHost/SetHost and clients/fn-rust's FakeHost closure.
// Never imported by production code (index.ts does not re-export this
// module); test files import it directly by relative path.
import { resetRegistry } from "./registry";

export interface FakeCall {
  op: number;
  meta: Record<string, unknown>;
  body?: Uint8Array;
}

export interface FakeHostResult {
  code: number;
  meta: Record<string, unknown>;
  body?: Uint8Array;
}

export type FakeHostHandler = (op: number, meta: Record<string, unknown>, body?: Uint8Array) => FakeHostResult;

/** Installs a fake `__fc_call` native. `handler` is invoked for every host
 * call the SDK issues; if omitted, every call succeeds with an empty
 * result (matching what ops like `log` return on the real ABI). Returns a
 * restore function, and the list of calls made while installed. */
export function installFakeHost(handler?: FakeHostHandler): { calls: FakeCall[]; restore: () => void } {
  const g = globalThis as unknown as {
    __fc_call?: (op: number, meta: string, body?: Uint8Array) => { code: number; meta: string; body: Uint8Array };
  };
  const previous = g.__fc_call;
  const calls: FakeCall[] = [];
  g.__fc_call = (op: number, metaStr: string, body?: Uint8Array) => {
    const meta = metaStr ? (JSON.parse(metaStr) as Record<string, unknown>) : {};
    calls.push({ op, meta, body });
    const result = handler ? handler(op, meta, body) : { code: 0, meta: {} };
    return { code: result.code, meta: JSON.stringify(result.meta ?? {}), body: result.body ?? new Uint8Array(0) };
  };
  return {
    calls,
    restore: () => {
      g.__fc_call = previous;
    },
  };
}

export { resetRegistry };
