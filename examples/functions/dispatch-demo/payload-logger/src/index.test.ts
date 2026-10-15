import { test } from "node:test";
import assert from "node:assert/strict";
import "./index";
import { handleRequest } from "../../../../../clients/fn-ts/src/dispatch";
import { describeJSON } from "../../../../../clients/fn-ts/src/describe";
import { installFakeHost } from "../../../../../clients/fn-ts/src/testing";
import { OpLog } from "../../../../../clients/fn-ts/src/ops";
import { DISPATCH_REQUESTED } from "./eventType";

test("describe subscribes the webhook to the demo event, data only", () => {
  const d = JSON.parse(describeJSON()) as { subscriptions: Record<string, unknown>[]; endpoints: { path: string; auth: string }[] };
  assert.deepEqual(d.subscriptions, [{ eventType: DISPATCH_REQUESTED, path: "/deliver", mode: "IMMEDIATE", dataOnly: true }]);
  assert.ok(d.endpoints.some((e) => e.path === "/deliver" && e.auth === "webhook"));
});

test("a delivery is logged and acked", async () => {
  const host = installFakeHost();
  try {
    const m = JSON.stringify({
      id: "inv_9", address: "x.payload-logger", version: 1, method: "POST", path: "/deliver", rawQuery: "",
      headers: {}, caller: { kind: "webhook" }, deadlineUnixMs: Date.now() + 5000,
    });
    const { meta, body } = await handleRequest(m, new TextEncoder().encode('{"order":42}'));
    assert.equal(JSON.parse(meta).status, 200);
    assert.deepEqual(JSON.parse(new TextDecoder().decode(body)), { ack: true });
    const logged = host.calls.find((c) => c.op === OpLog && String(c.meta.msg) === "payload received");
    assert.ok(logged);
    assert.match(JSON.stringify(logged.meta), /order/);
  } finally {
    host.restore();
  }
});
