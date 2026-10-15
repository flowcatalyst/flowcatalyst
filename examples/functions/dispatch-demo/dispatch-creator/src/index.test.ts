import { test } from "node:test";
import assert from "node:assert/strict";
import "./index";
import { handleRequest } from "../../../../../clients/fn-ts/src/dispatch";
import { describeJSON } from "../../../../../clients/fn-ts/src/describe";
import { installFakeHost } from "../../../../../clients/fn-ts/src/testing";
import { OpEventEmit } from "../../../../../clients/fn-ts/src/ops";
import { DISPATCH_REQUESTED } from "./eventType";

const enc = (s: string) => new TextEncoder().encode(s);
const dec = (b: Uint8Array) => new TextDecoder().decode(b);

function meta(overrides: Record<string, unknown> = {}): string {
  return JSON.stringify({
    id: "inv_1", address: "dispatchdemo.dispatch-creator", version: 1, method: "POST", path: "/dispatch",
    rawQuery: "", headers: {}, caller: { kind: "principal", id: "prn_1", type: "SERVICE", permissions: ["dispatchdemo:dispatch:job:create"] },
    deadlineUnixMs: Date.now() + 5000, ...overrides,
  });
}

test("describe declares the emit and a platform endpoint", () => {
  const d = JSON.parse(describeJSON()) as { endpoints: { path: string; auth: string }[]; emits: string[] };
  assert.deepEqual(d.emits, [DISPATCH_REQUESTED]);
  assert.ok(d.endpoints.some((e) => e.path === "/dispatch" && e.auth === "platform"));
});

test("POST /dispatch emits the payload as the event data", async () => {
  const host = installFakeHost((op) => (op === OpEventEmit ? { code: 0, meta: { eventId: "evt_1" } } : { code: 0, meta: {} }));
  try {
    const { meta: rm, body } = await handleRequest(meta(), enc('{"order":42}'));
    assert.equal(JSON.parse(rm).status, 202);
    assert.deepEqual(JSON.parse(dec(body)), { eventId: "evt_1" });
    const emit = host.calls.find((c) => c.op === OpEventEmit);
    assert.ok(emit);
    assert.equal(emit.meta.type, DISPATCH_REQUESTED);
    assert.equal(emit.meta.dedupId, "dispatch-demo-inv_1");
    assert.equal(dec(emit.body ?? new Uint8Array()), '{"order":42}');
  } finally {
    host.restore();
  }
});

test("a non-JSON body is a 400 and nothing is emitted", async () => {
  const host = installFakeHost();
  try {
    const { meta: rm } = await handleRequest(meta(), enc("not json"));
    assert.equal(JSON.parse(rm).status, 400);
    assert.equal(host.calls.filter((c) => c.op === OpEventEmit).length, 0);
  } finally {
    host.restore();
  }
});

test("a refused emit is a 502", async () => {
  const host = installFakeHost((op) =>
    op === OpEventEmit ? { code: 1, meta: { code: "NOT_ALLOWED", message: "no" } } : { code: 0, meta: {} });
  try {
    const { meta: rm } = await handleRequest(meta(), enc("{}"));
    assert.equal(JSON.parse(rm).status, 502);
  } finally {
    host.restore();
  }
});

test("a principal without the permission gets 403 and nothing is emitted", async () => {
  const host = installFakeHost();
  try {
    const { meta: rm } = await handleRequest(
      meta({ caller: { kind: "principal", id: "prn_2", type: "SERVICE", permissions: ["other:x:y:z"] } }), enc("{}"));
    assert.equal(JSON.parse(rm).status, 403);
    assert.equal(host.calls.filter((c) => c.op === OpEventEmit).length, 0);
  } finally {
    host.restore();
  }
});
