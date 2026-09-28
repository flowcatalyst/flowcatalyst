import { test, beforeEach } from "node:test";
import assert from "node:assert/strict";
import { open, platform, webhook, subscribe, schedule } from "./registry";
import { config } from "./config";
import { secret } from "./config";
import { db } from "./db";
import { httpAllow } from "./http";
import { emits } from "./event";
import { buildDescribeDoc, describeJSON } from "./describe";
import { resetRegistry } from "./testing";

beforeEach(() => {
  resetRegistry();
});

const ok = () => ({ status: 200 });

test("describe omits every empty optional field", () => {
  open("GET /healthz", ok);
  const doc = buildDescribeDoc();
  assert.equal(doc.abi, 1);
  assert.equal(doc.endpoints.length, 1);
  for (const key of ["subscriptions", "schedules", "config", "secrets", "db", "httpAllow", "emits"] as const) {
    assert.equal(key in doc, false, `expected ${key} to be omitted when empty`);
  }
});

test("describe: full shape, matching plan §5.4's field names exactly", () => {
  webhook("POST /events/order-created", ok);
  platform("GET /hello/{name}", ok);
  open("GET /healthz", ok);
  subscribe("orders:order:order:created", "/events/order-created", { mode: "IMMEDIATE", maxRetries: 3, dataOnly: true });
  schedule("*/5 * * * *", "/events/tick", { timezone: "UTC" });
  config("GREETING");
  secret("STRIPE_KEY");
  db("main");
  httpAllow("api.stripe.com");
  emits("orders:order:order:shipped");

  const doc = JSON.parse(describeJSON());
  assert.equal(doc.abi, 1);
  assert.equal(doc.endpoints.length, 3);
  assert.deepEqual(
    doc.endpoints.find((e: { path: string }) => e.path === "/events/order-created"),
    { method: "POST", path: "/events/order-created", auth: "webhook" },
  );
  assert.equal(doc.subscriptions[0].eventType, "orders:order:order:created");
  assert.equal(doc.subscriptions[0].mode, "IMMEDIATE");
  assert.equal(doc.subscriptions[0].maxRetries, 3);
  assert.equal(doc.subscriptions[0].dataOnly, true);
  assert.equal(doc.schedules[0].cron, "*/5 * * * *");
  assert.equal(doc.schedules[0].timezone, "UTC");
  assert.deepEqual(doc.config, ["GREETING"]);
  assert.deepEqual(doc.secrets, ["STRIPE_KEY"]);
  assert.deepEqual(doc.db, ["main"]);
  assert.deepEqual(doc.httpAllow, ["api.stripe.com"]);
  assert.deepEqual(doc.emits, ["orders:order:order:shipped"]);
});

test("describe: endpoint options (maxBodyBytes, timeoutMs, cors)", () => {
  open("GET /healthz", ok, { maxBodyBytes: 0, timeoutMs: 5000, cors: { origins: ["https://app.acme.com"], allowCredentials: true } });
  const doc = buildDescribeDoc();
  const ep = doc.endpoints[0]!;
  assert.equal(ep.maxBodyBytes, 0);
  assert.equal(ep.timeoutMs, 5000);
  assert.deepEqual(ep.cors, { origins: ["https://app.acme.com"], allowCredentials: true });
});

test("webhook registration must be POST-only", () => {
  assert.throws(() => webhook("GET /events/x", ok), /POST-only/);
});
