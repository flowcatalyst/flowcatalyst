import { test, beforeEach } from "node:test";
import assert from "node:assert/strict";
import { open, platform, webhook } from "./registry";
import { json, text, status as statusResp, retry, reject } from "./respond";
import { handleRequest } from "./dispatch";
import { resetRegistry, installFakeHost } from "./testing";
import type { FnRequest } from "./types";

beforeEach(() => {
  resetRegistry();
});

function requestMeta(overrides: Record<string, unknown> = {}): string {
  return JSON.stringify({
    id: "inv_1",
    address: "app.fn",
    version: 1,
    method: "GET",
    path: "/healthz",
    rawQuery: "",
    headers: {},
    caller: { kind: "anonymous" },
    deadlineUnixMs: Date.now() + 5000,
    ...overrides,
  });
}

const enc = (s: string) => new TextEncoder().encode(s);
const dec = (b: Uint8Array) => new TextDecoder().decode(b);

test("dispatch: a matching route returns the handler's response", async () => {
  open("GET /healthz", () => json(200, { status: "ok" }));
  const { meta, body } = await handleRequest(requestMeta(), new Uint8Array());
  const m = JSON.parse(meta);
  assert.equal(m.status, 200);
  assert.deepEqual(JSON.parse(dec(body)), { status: "ok" });
  assert.deepEqual(m.headers["Content-Type"], ["application/json"]);
});

test("dispatch: 404 when no route matches", async () => {
  open("GET /healthz", () => json(200, {}));
  const { meta, body } = await handleRequest(requestMeta({ path: "/nope" }), new Uint8Array());
  assert.equal(JSON.parse(meta).status, 404);
  assert.equal(body.length, 0);
});

test("dispatch: 405 with Allow when the path matches but not the method", async () => {
  open("GET /only-get", () => json(200, {}));
  const { meta } = await handleRequest(requestMeta({ method: "POST", path: "/only-get" }), new Uint8Array());
  const m = JSON.parse(meta);
  assert.equal(m.status, 405);
  assert.ok(m.headers.Allow[0].includes("GET"));
});

test("dispatch: HEAD falls back to GET with the body suppressed", async () => {
  open("GET /healthz", () => text(200, "ok"));
  const { meta, body } = await handleRequest(requestMeta({ method: "HEAD", path: "/healthz" }), new Uint8Array());
  assert.equal(JSON.parse(meta).status, 200);
  assert.equal(body.length, 0);
});

test("dispatch: a thrown error becomes 500 FUNCTION_PANIC and logs", async () => {
  const fake = installFakeHost();
  open("GET /boom", () => {
    throw new Error("kaboom");
  });
  const { meta, body } = await handleRequest(requestMeta({ path: "/boom" }), new Uint8Array());
  assert.equal(JSON.parse(meta).status, 500);
  assert.deepEqual(JSON.parse(dec(body)), { error: "FUNCTION_PANIC" });
  assert.equal(fake.calls.length, 1);
  assert.equal(fake.calls[0]!.meta.level, "ERROR");
  fake.restore();
});

test("dispatch: a rejected async handler also becomes 500 FUNCTION_PANIC", async () => {
  open("GET /boom", async () => {
    await Promise.resolve();
    throw new Error("async kaboom");
  });
  const { meta, body } = await handleRequest(requestMeta({ path: "/boom" }), new Uint8Array());
  assert.equal(JSON.parse(meta).status, 500);
  assert.deepEqual(JSON.parse(dec(body)), { error: "FUNCTION_PANIC" });
});

test("dispatch: a malformed meta frame becomes 500 FUNCTION_PANIC, not a throw", async () => {
  open("GET /healthz", () => json(200, {}));
  const { meta, body } = await handleRequest("{not json", new Uint8Array());
  assert.equal(JSON.parse(meta).status, 500);
  assert.deepEqual(JSON.parse(dec(body)), { error: "FUNCTION_PANIC" });
});

test("dispatch: retry() answers 429 with Retry-After", async () => {
  open("GET /x", () => retry(5));
  const { meta } = await handleRequest(requestMeta({ path: "/x" }), new Uint8Array());
  const m = JSON.parse(meta);
  assert.equal(m.status, 429);
  assert.deepEqual(m.headers["Retry-After"], ["5"]);
});

test("dispatch: reject() answers 422 with the FlowCatalyst-Outcome header and a plain-text body", async () => {
  open("GET /x", () => reject("nope"));
  const { meta, body } = await handleRequest(requestMeta({ path: "/x" }), new Uint8Array());
  const m = JSON.parse(meta);
  assert.equal(m.status, 422);
  assert.deepEqual(m.headers["FlowCatalyst-Outcome"], ["reject"]);
  assert.equal(dec(body), "nope");
});

test("dispatch: reject() with no reason writes no body", async () => {
  open("GET /x", () => reject());
  const { body } = await handleRequest(requestMeta({ path: "/x" }), new Uint8Array());
  assert.equal(body.length, 0);
});

test("dispatch: status() and a raw object body both work", async () => {
  open("GET /a", () => statusResp(204));
  const a = await handleRequest(requestMeta({ path: "/a" }), new Uint8Array());
  assert.equal(JSON.parse(a.meta).status, 204);
  assert.equal(a.body.length, 0);

  open("GET /b", () => ({ status: 201, body: { ok: true } }));
  const b = await handleRequest(requestMeta({ path: "/b" }), new Uint8Array());
  const bm = JSON.parse(b.meta);
  assert.equal(bm.status, 201);
  assert.deepEqual(bm.headers["Content-Type"], ["application/json"]);
  assert.deepEqual(JSON.parse(dec(b.body)), { ok: true });
});

test("dispatch: a raw Uint8Array body passes through untouched", async () => {
  open("GET /bin", () => ({ status: 200, body: new Uint8Array([1, 2, 3]) }));
  const { body } = await handleRequest(requestMeta({ path: "/bin" }), new Uint8Array());
  assert.deepEqual([...body], [1, 2, 3]);
});

test("dispatch: request parsing exposes params, query, headers, body and caller", async () => {
  let captured: FnRequest | undefined;
  open("GET /widgets/{id}", (req) => {
    captured = req;
    return statusResp(200);
  });
  const meta = requestMeta({
    path: "/widgets/42",
    rawQuery: "a=1&a=2&b=x",
    headers: { "X-Trace": ["abc"] },
    caller: { kind: "principal", id: "prn_1", tier: "CLIENT", permissions: ["platform:function:view"] },
  });
  await handleRequest(meta, enc("hello"));
  assert.ok(captured);
  assert.equal(captured!.params.id, "42");
  assert.deepEqual(captured!.query.getAll("a"), ["1", "2"]);
  assert.equal(captured!.query.get("b"), "x");
  assert.equal(captured!.headers.get("x-trace"), "abc");
  assert.equal(captured!.text(), "hello");
  assert.equal(captured!.caller.kind, "principal");
  assert.equal(captured!.caller.principal?.hasPermission("platform:function:view"), true);
});

test("dispatch: req.json() parses the body", async () => {
  let seen: unknown;
  webhook("POST /events/x", (req) => {
    seen = req.json();
    return statusResp(202);
  });
  await handleRequest(requestMeta({ method: "POST", path: "/events/x" }), enc(JSON.stringify({ id: "evt_1" })));
  assert.deepEqual(seen, { id: "evt_1" });
});

test("dispatch: platform auth exposes no principal for a webhook/anonymous caller", async () => {
  let sawPrincipal = true;
  platform("GET /p", (req) => {
    sawPrincipal = req.caller.principal !== undefined;
    return statusResp(200);
  });
  await handleRequest(requestMeta({ path: "/p", caller: { kind: "anonymous" } }), new Uint8Array());
  assert.equal(sawPrincipal, false);
});
