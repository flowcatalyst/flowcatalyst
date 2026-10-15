import { test, beforeEach } from "node:test";
import assert from "node:assert/strict";
import { dispatch, dispatches, metadataEntries } from "./job";
import { FnError, isRetryable } from "./errors";
import { OpDispatch } from "./ops";
import { buildDescribeDoc } from "./describe";
import { installFakeHost, resetRegistry } from "./testing";

const dec = (b: Uint8Array) => new TextDecoder().decode(b);

beforeEach(() => {
  resetRegistry();
});

test("dispatches() declares the (target, path) pair into describe", () => {
  dispatches("orders.logger", "/deliver");
  dispatches("orders.logger", "/other");
  assert.deepEqual(buildDescribeDoc().dispatches, [
    { target: "orders.logger", path: "/deliver" },
    { target: "orders.logger", path: "/other" },
  ]);
});

test("dispatch() sends target/path, JSON-encodes an object payload and returns the job id", async () => {
  const fake = installFakeHost((op, meta, body) => {
    assert.equal(op, OpDispatch);
    assert.equal(meta.target, "orders.logger");
    assert.equal(meta.path, "/deliver");
    assert.equal(meta.contentType, "application/json");
    assert.equal(meta.subject, "order/42");
    assert.equal(meta.messageGroup, "g");
    assert.equal(meta.mode, "IMMEDIATE");
    assert.equal(meta.dataOnly, false);
    assert.deepEqual(JSON.parse(dec(body!)), { order: 42 });
    return { code: 0, meta: { jobId: "djob_1" } };
  });
  try {
    const id = await dispatch({
      target: "orders.logger", path: "/deliver", payload: { order: 42 },
      subject: "order/42", messageGroup: "g", mode: "IMMEDIATE", dataOnly: false,
    });
    assert.equal(id, "djob_1");
  } finally {
    fake.restore();
  }
});

test("dispatch() leaves an explicit contentType and a string payload alone", async () => {
  const fake = installFakeHost((_op, meta, body) => {
    assert.equal(meta.contentType, "text/plain");
    assert.equal(dec(body!), "hello");
    return { code: 0, meta: { jobId: "j" } };
  });
  try {
    await dispatch({ target: "a.b", path: "/d", payload: "hello", contentType: "text/plain" });
  } finally {
    fake.restore();
  }
});

test("metadata: an object becomes a key-sorted array of {key,value}; an array passes through", async () => {
  assert.deepEqual(metadataEntries({ b: "2", a: "1" }), [{ key: "a", value: "1" }, { key: "b", value: "2" }]);
  assert.deepEqual(metadataEntries([{ key: "z", value: "9" }, { key: "a", value: "1" }]), [{ key: "z", value: "9" }, { key: "a", value: "1" }]);
  assert.equal(metadataEntries(undefined), undefined);
  const fake = installFakeHost((_op, meta) => {
    assert.ok(Array.isArray(meta.metadata), "metadata must travel as an array");
    assert.deepEqual(meta.metadata, [{ key: "tenant", value: "acme" }]);
    return { code: 0, meta: { jobId: "j" } };
  });
  try {
    await dispatch({ target: "a.b", path: "/d", payload: {}, metadata: { tenant: "acme" } });
  } finally {
    fake.restore();
  }
});

test("dispatch() omits unset optional fields from the request", async () => {
  const fake = installFakeHost((_op, meta) => {
    assert.deepEqual(Object.keys(meta).sort(), ["contentType", "path", "target"]);
    return { code: 0, meta: { jobId: "j" } };
  });
  try {
    await dispatch({ target: "a.b", path: "/d", payload: {} });
  } finally {
    fake.restore();
  }
});

test("dispatch() UNAVAILABLE is retryable; a refusal is not", async () => {
  for (const [code, retryable] of [["UNAVAILABLE", true], ["NOT_DECLARED", false], ["NOT_ALLOWED", false], ["BAD_REQUEST", false], ["TOO_LARGE", false]] as const) {
    const fake = installFakeHost(() => ({ code: 1, meta: { code, message: "x" } }));
    try {
      await dispatch({ target: "a.b", path: "/d" });
      assert.fail("expected dispatch to throw");
    } catch (err) {
      assert.ok(err instanceof FnError);
      assert.equal((err as FnError).code, code);
      assert.equal(isRetryable(err), retryable, code);
    } finally {
      fake.restore();
    }
  }
});
