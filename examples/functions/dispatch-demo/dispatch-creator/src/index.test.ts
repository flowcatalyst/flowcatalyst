import { test } from "node:test";
import assert from "node:assert/strict";
import "./index";
import { handleRequest } from "../../../../../clients/fn-ts/src/dispatch";
import { describeJSON } from "../../../../../clients/fn-ts/src/describe";
import { installFakeHost } from "../../../../../clients/fn-ts/src/testing";
import { OpDispatch, OpEventEmit } from "../../../../../clients/fn-ts/src/ops";
import { DELIVER_PATH, LOGGER_ADDRESS, REQUIRED_PERMISSION } from "./names";

const enc = (s: string) => new TextEncoder().encode(s);
const dec = (b: Uint8Array) => new TextDecoder().decode(b);

function meta(overrides: Record<string, unknown> = {}): string {
  return JSON.stringify({
    id: "inv_1", address: "dispatchdemo.dispatch-creator", version: 1, method: "POST", path: "/dispatch",
    rawQuery: "", headers: {}, caller: { kind: "principal", id: "prn_1", type: "SERVICE", permissions: [REQUIRED_PERMISSION] },
    deadlineUnixMs: Date.now() + 5000, ...overrides,
  });
}

test("describe declares the dispatch target and a platform endpoint, and emits nothing", () => {
  const d = JSON.parse(describeJSON()) as { endpoints: { path: string; auth: string }[]; dispatches: { target: string; path: string }[]; emits?: string[] };
  assert.deepEqual(d.dispatches, [{ target: LOGGER_ADDRESS, path: DELIVER_PATH }]);
  assert.equal(d.emits, undefined);
  assert.ok(d.endpoints.some((e) => e.path === "/dispatch" && e.auth === "platform"));
});

test("the target is the payload-logger function of the same application", () => {
  assert.equal(LOGGER_ADDRESS, "dispatchdemo.payload-logger");
  assert.equal(DELIVER_PATH, "/deliver");
  assert.equal(REQUIRED_PERMISSION, "dispatchdemo:dispatch:job:create");
});

test("POST /dispatch creates a dispatch job carrying the payload", async () => {
  const host = installFakeHost((op) => (op === OpDispatch ? { code: 0, meta: { jobId: "djob_1" } } : { code: 0, meta: {} }));
  try {
    const { meta: rm, body } = await handleRequest(meta(), enc('{"order":42}'));
    assert.equal(JSON.parse(rm).status, 202);
    assert.deepEqual(JSON.parse(dec(body)), { jobId: "djob_1" });
    const call = host.calls.find((c) => c.op === OpDispatch);
    assert.ok(call);
    assert.equal(call.meta.target, LOGGER_ADDRESS);
    assert.equal(call.meta.path, DELIVER_PATH);
    assert.equal(call.meta.messageGroup, "dispatch-demo");
    assert.equal(call.meta.contentType, "application/json");
    assert.deepEqual(call.meta.metadata, [{ key: "requestedBy", value: "prn_1" }], "metadata travels as an array");
    assert.deepEqual(JSON.parse(dec(call.body ?? new Uint8Array())), { order: 42 });
    assert.equal(host.calls.filter((c) => c.op === OpEventEmit).length, 0, "no event is emitted any more");
  } finally {
    host.restore();
  }
});

test("a non-JSON body is a 400 and nothing is dispatched", async () => {
  const host = installFakeHost();
  try {
    const { meta: rm } = await handleRequest(meta(), enc("not json"));
    assert.equal(JSON.parse(rm).status, 400);
    assert.equal(host.calls.filter((c) => c.op === OpDispatch).length, 0);
  } finally {
    host.restore();
  }
});

test("a JSON scalar is a 400 and nothing is dispatched", async () => {
  const host = installFakeHost();
  try {
    const { meta: rm } = await handleRequest(meta(), enc("42"));
    assert.equal(JSON.parse(rm).status, 400);
    assert.equal(host.calls.filter((c) => c.op === OpDispatch).length, 0);
  } finally {
    host.restore();
  }
});

test("a refused dispatch is a 502", async () => {
  const host = installFakeHost((op) =>
    op === OpDispatch ? { code: 1, meta: { code: "NOT_ALLOWED", message: "no" } } : { code: 0, meta: {} });
  try {
    const { meta: rm } = await handleRequest(meta(), enc("{}"));
    assert.equal(JSON.parse(rm).status, 502);
  } finally {
    host.restore();
  }
});

test("platform trouble is a retryable 503", async () => {
  const host = installFakeHost((op) =>
    op === OpDispatch ? { code: 1, meta: { code: "UNAVAILABLE", message: "down" } } : { code: 0, meta: {} });
  try {
    const { meta: rm } = await handleRequest(meta(), enc("{}"));
    assert.equal(JSON.parse(rm).status, 503);
  } finally {
    host.restore();
  }
});

test("a principal without the permission gets 403 and nothing is dispatched", async () => {
  const host = installFakeHost();
  try {
    const { meta: rm } = await handleRequest(
      meta({ caller: { kind: "principal", id: "prn_2", type: "SERVICE", permissions: ["other:x:y:z"] } }), enc("{}"));
    assert.equal(JSON.parse(rm).status, 403);
    assert.equal(host.calls.filter((c) => c.op === OpDispatch).length, 0);
  } finally {
    host.restore();
  }
});

test("a caller that is not a principal gets 403", async () => {
  const host = installFakeHost();
  try {
    const { meta: rm } = await handleRequest(meta({ caller: { kind: "anonymous" } }), enc("{}"));
    assert.equal(JSON.parse(rm).status, 403);
    assert.equal(host.calls.filter((c) => c.op === OpDispatch).length, 0);
  } finally {
    host.restore();
  }
});
