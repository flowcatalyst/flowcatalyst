import { test, beforeEach } from "node:test";
import assert from "node:assert/strict";
import { buildDescribeDoc } from "./describe";
import { handleRequest } from "./dispatch";
import { OpTrace, OpTraceReference, OpTracing } from "./ops";
import { open } from "./registry";
import { newTracer, resetTraceWarnings, traceContexts } from "./trace";
import { installFakeHost, resetRegistry } from "./testing";

beforeEach(() => {
  resetRegistry();
  resetTraceWarnings();
});

// Captures console.error for the duration of fn.
function captureStderr(fn: () => void): string[] {
  const lines: string[] = [];
  const orig = console.error;
  console.error = (...args: unknown[]) => lines.push(args.join(" "));
  try {
    fn();
  } finally {
    console.error = orig;
  }
  return lines;
}

test("trace sends op 12 with context, msg, level, data and reference", () => {
  const fake = installFakeHost();
  newTracer().trace("orders", "resolved", { lines: 3 }, { level: "warn", reference: "TRIP-1" });
  fake.restore();
  assert.equal(fake.calls.length, 1);
  assert.equal(fake.calls[0]!.op, OpTrace);
  assert.deepEqual(fake.calls[0]!.meta, { context: "orders", msg: "resolved", level: "warn", data: { lines: 3 }, reference: "TRIP-1" });
});

test("trace omits optional fields it was not given", () => {
  const fake = installFakeHost();
  newTracer().trace("orders", "hi");
  fake.restore();
  assert.deepEqual(fake.calls[0]!.meta, { context: "orders", msg: "hi" });
});

test("setReference stamps later calls; a call's own reference wins", () => {
  const fake = installFakeHost();
  const t = newTracer();
  t.trace("orders", "before");
  t.setReference("ORD-9");
  t.trace("orders", "after");
  t.trace("orders", "override", undefined, { reference: "ORD-10" });
  fake.restore();
  const traces = fake.calls.filter((c) => c.op === OpTrace);
  assert.equal(traces[0]!.meta.reference, undefined);
  assert.equal(traces[1]!.meta.reference, "ORD-9");
  assert.equal(traces[2]!.meta.reference, "ORD-10");
});

test("a reference is per invocation: a fresh tracer starts empty", () => {
  const fake = installFakeHost();
  newTracer().setReference("ORD-9");
  newTracer().trace("orders", "other invocation");
  fake.restore();
  assert.equal(fake.calls.filter((c) => c.op === OpTrace)[0]!.meta.reference, undefined);
});

test("an over-long reference and message are cut client-side", () => {
  const fake = installFakeHost();
  newTracer().trace("orders", "é".repeat(3000), undefined, { reference: "r".repeat(500) });
  fake.restore();
  const m = fake.calls[0]!.meta;
  assert.equal((m.reference as string).length, 200);
  assert.ok(new TextEncoder().encode(m.msg as string).length <= 4096);
  assert.ok(!(m.msg as string).includes("�"));
});

test("trace never throws on any host error and warns once per undeclared context", () => {
  for (const code of ["UNKNOWN_OP", "TOO_LARGE", "BAD_REQUEST", "UNAVAILABLE"]) {
    const fake = installFakeHost(() => ({ code: 1, meta: { code, message: "x" } }));
    assert.doesNotThrow(() => newTracer().trace("orders", "m", { a: 1 }));
    fake.restore();
  }
  const fake = installFakeHost(() => ({ code: 1, meta: { code: "NOT_DECLARED", message: "x" } }));
  const lines = captureStderr(() => {
    const t = newTracer();
    t.trace("nope", "a");
    t.trace("nope", "b");
    t.tracing("nope");
    t.trace("other", "c");
  });
  fake.restore();
  assert.equal(lines.length, 2);
  assert.match(lines[0]!, /"nope" is not declared/);
  assert.match(lines[1]!, /"other" is not declared/);
});

test("trace swallows a missing __fc_call native", () => {
  const g = globalThis as unknown as { __fc_call?: unknown };
  const prev = g.__fc_call;
  delete g.__fc_call;
  try {
    assert.doesNotThrow(() => newTracer().trace("orders", "m"));
    assert.equal(newTracer().tracing("orders"), false);
  } finally {
    g.__fc_call = prev;
  }
});

test("tracing sends op 13 and returns live", () => {
  const fake = installFakeHost((op, meta) => {
    assert.equal(op, OpTracing);
    assert.equal(meta.context, "orders");
    return { code: 0, meta: { live: true } };
  });
  assert.equal(newTracer().tracing("orders"), true);
  fake.restore();
});

test("tracing is false when not live, on a missing result, and on every error", () => {
  let fake = installFakeHost(() => ({ code: 0, meta: { live: false } }));
  assert.equal(newTracer().tracing("orders"), false);
  fake.restore();
  fake = installFakeHost(() => ({ code: 0, meta: {} }));
  assert.equal(newTracer().tracing("orders"), false);
  fake.restore();
  for (const code of ["UNKNOWN_OP", "NOT_DECLARED", "UNAVAILABLE"]) {
    fake = installFakeHost(() => ({ code: 1, meta: { code, message: "x" } }));
    captureStderr(() => assert.equal(newTracer().tracing("orders"), false));
    fake.restore();
  }
});

test("tracing is never cached: every call, and every invocation, asks the host", () => {
  let live = false;
  const fake = installFakeHost(() => ({ code: 0, meta: { live } }));
  const t = newTracer();
  assert.equal(t.tracing("orders"), false);
  live = true; // the next invocation was sampled in
  assert.equal(t.tracing("orders"), true);
  assert.equal(newTracer().tracing("orders"), true);
  fake.restore();
  assert.equal(fake.calls.length, 3);
});

test("a request carries the tracer, with its own reference state", async () => {
  open("GET /x", (req) => {
    req.setReference("TRIP-7");
    req.trace("orders", "in handler");
    return { status: 200 };
  });
  const fake = installFakeHost();
  const meta = JSON.stringify({ id: "inv_1", method: "GET", path: "/x" });
  const frame = await handleRequest(meta, new Uint8Array(0));
  fake.restore();
  assert.equal(JSON.parse(frame.meta).status, 200);
  const call = fake.calls.find((c) => c.op === OpTrace)!;
  assert.deepEqual(call.meta, { context: "orders", msg: "in handler", reference: "TRIP-7" });
});

test("traceContexts emits the describe key and omits it when empty", () => {
  assert.equal("traceContexts" in buildDescribeDoc(), false);
  traceContexts({ name: "orders", description: "What the mapping decided." }, { name: "retry-3" });
  assert.deepEqual(buildDescribeDoc().traceContexts, [{ name: "orders", description: "What the mapping decided." }, { name: "retry-3" }]);
});

test("traceContexts rejects bad declarations with a clear message", () => {
  assert.throws(() => traceContexts({ name: "http" }), /built-in capture context/);
  assert.throws(() => traceContexts({ name: "Orders" }), /not a valid context name/);
  assert.throws(() => traceContexts({ name: "1x" }), /not a valid context name/);
  assert.throws(() => traceContexts({ name: "a".repeat(41) }), /not a valid context name/);
  assert.throws(() => traceContexts({ name: "ok", description: "d".repeat(201) }), /description is over 200 bytes/);
  assert.throws(() => traceContexts({ name: "ok", description: "é".repeat(101) }), /description is over 200 bytes/);
  traceContexts({ name: "multibyte", description: "é".repeat(100) }); // 200 bytes: accepted
  traceContexts({ name: "dup" });
  assert.throws(() => traceContexts({ name: "dup" }), /declared twice/);
  assert.equal(buildDescribeDoc().traceContexts!.length, 2);
});

test("traceContexts allows at most 20", () => {
  for (let i = 0; i < 20; i++) traceContexts({ name: `c${i}` });
  assert.throws(() => traceContexts({ name: "one-more" }), /at most 20/);
});

test("setReference makes an op 14 call with the reference", () => {
  const fake = installFakeHost();
  newTracer().setReference("TRIP-123");
  fake.restore();
  assert.equal(fake.calls.length, 1);
  assert.equal(fake.calls[0]!.op, OpTraceReference);
  assert.deepEqual(fake.calls[0]!.meta, { reference: "TRIP-123" });
});

test("setReference swallows every host error and still remembers the value", () => {
  for (const code of ["UNKNOWN_OP", "TOO_LARGE", "BAD_REQUEST"]) {
    const fake = installFakeHost(() => ({ code: 1, meta: { code, message: "x" } }));
    const t = newTracer();
    assert.doesNotThrow(() => t.setReference("R"));
    t.trace("orders", "m");
    fake.restore();
    assert.equal(fake.calls.find((c) => c.op === OpTrace)!.meta.reference, "R");
  }
  const g = globalThis as unknown as { __fc_call?: unknown };
  const prev = g.__fc_call;
  delete g.__fc_call;
  try {
    assert.doesNotThrow(() => newTracer().setReference("R"));
  } finally {
    g.__fc_call = prev;
  }
});

test("a per-call reference applies to that record only", () => {
  const fake = installFakeHost();
  const t = newTracer();
  t.trace("orders", "one", undefined, { reference: "ONE" });
  t.trace("orders", "two");
  t.setReference("SET");
  t.trace("orders", "three", undefined, { reference: "THREE" });
  t.trace("orders", "four");
  fake.restore();
  const refs = fake.calls.filter((c) => c.op === OpTrace).map((c) => c.meta.reference);
  assert.deepEqual(refs, ["ONE", undefined, "THREE", "SET"]);
});

test("a reference cut never splits a surrogate pair", () => {
  const fake = installFakeHost();
  const ref = "a".repeat(199) + "\u{1F600}" + "bcd";
  const t = newTracer();
  t.setReference(ref);
  t.trace("orders", "m", undefined, { reference: ref });
  fake.restore();
  for (const c of fake.calls) {
    const r = c.meta.reference as string;
    assert.ok(Array.from(r).length <= 200);
    assert.equal(r, "a".repeat(199) + "\u{1F600}");
    const last = r.charCodeAt(r.length - 1);
    assert.ok(!(last >= 0xd800 && last <= 0xdbff));
  }
  assert.equal(fake.calls.length, 2);
});
