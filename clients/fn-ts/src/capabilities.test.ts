import { test, beforeEach } from "node:test";
import assert from "node:assert/strict";
import { config, secret } from "./config";
import { db } from "./db";
import { emit } from "./event";
import { fetchFn } from "./http";
import { log, installConsole } from "./log";
import { callHost } from "./native";
import { FnError, isRetryable } from "./errors";
import { OpConfigGet, OpDBBegin, OpDBCommit, OpDBExec, OpDBQuery, OpDBRollback, OpEventEmit, OpHTTPFetch, OpLog, OpSecretGet } from "./ops";
import { installFakeHost, resetRegistry } from "./testing";

const enc = (s: string) => new TextEncoder().encode(s);
const dec = (b: Uint8Array) => new TextDecoder().decode(b);

beforeEach(() => {
  resetRegistry();
});

// --- config / secret -------------------------------------------------------

test("config().get() returns the value when found", () => {
  const fake = installFakeHost((op, meta) => {
    assert.equal(op, OpConfigGet);
    assert.equal(meta.key, "GREETING");
    return { code: 0, meta: { found: true }, body: enc("Kia ora") };
  });
  const v = config("GREETING");
  assert.equal(v.get(), "Kia ora");
  fake.restore();
});

test("config().get() returns undefined when not found", () => {
  const fake = installFakeHost(() => ({ code: 0, meta: { found: false } }));
  assert.equal(config("X").get(), undefined);
  fake.restore();
});

test("config().get() throws FnError on a host error", () => {
  const fake = installFakeHost(() => ({ code: 1, meta: { code: "CAPABILITY_UNAVAILABLE", message: "nope" } }));
  assert.throws(() => config("X").get(), (err: unknown) => err instanceof FnError && err.code === "CAPABILITY_UNAVAILABLE");
  fake.restore();
});

test("secret().get() uses op 3 and never logs the value itself", () => {
  const fake = installFakeHost((op) => {
    assert.equal(op, OpSecretGet);
    return { code: 0, meta: { found: true }, body: enc("sh") };
  });
  assert.equal(secret("K").get(), "sh");
  fake.restore();
});

test("config/secret declare into the registry", () => {
  const fake = installFakeHost();
  config("GREETING");
  secret("STRIPE_KEY");
  fake.restore();
  // Declared keys are asserted via describe.test.ts; here we only check
  // that calling config()/secret() doesn't itself make a host call.
  assert.equal(fake.calls.length, 0);
});

// --- db ----------------------------------------------------------------

test("db().query() decodes rows in select order", async () => {
  const fake = installFakeHost((op, meta) => {
    assert.equal(op, OpDBQuery);
    assert.equal(meta.db, "main");
    assert.equal(meta.sql, "select id, name from widgets");
    return { code: 0, meta: { truncated: false }, body: enc(JSON.stringify([{ id: 1, name: "a" }, { id: 2, name: "b" }])) };
  });
  const rows = await db("main").query("select id, name from widgets");
  assert.deepEqual(rows, [{ id: 1, name: "a" }, { id: 2, name: "b" }]);
  fake.restore();
});

test("db().query() base64-encodes Uint8Array params and ISO-encodes Date params", async () => {
  const fake = installFakeHost((_op, meta) => {
    assert.equal((meta.params as unknown[])[0], "AQID");
    assert.equal(typeof (meta.params as unknown[])[1], "string");
    return { code: 0, meta: {}, body: enc("[]") };
  });
  await db("main").query("select 1", [new Uint8Array([1, 2, 3]), new Date(0)]);
  fake.restore();
});

test("db().exec() returns rowsAffected", async () => {
  const fake = installFakeHost((op) => {
    assert.equal(op, OpDBExec);
    return { code: 0, meta: { rowsAffected: 3 } };
  });
  const r = await db("main").exec("update widgets set x = 1");
  assert.equal(r.rowsAffected, 3);
  fake.restore();
});

test("db().transaction() commits on resolve", async () => {
  const calls: number[] = [];
  const fake = installFakeHost((op) => {
    calls.push(op);
    if (op === OpDBBegin) return { code: 0, meta: { tx: "tx_1" } };
    if (op === OpDBExec) return { code: 0, meta: { rowsAffected: 1 } };
    if (op === OpDBCommit) return { code: 0, meta: {} };
    throw new Error("unexpected op " + op);
  });
  const result = await db("main").transaction(async (tx) => {
    await tx.exec("insert into widgets default values");
    return "done";
  });
  assert.equal(result, "done");
  assert.deepEqual(calls, [OpDBBegin, OpDBExec, OpDBCommit]);
  fake.restore();
});

test("db().transaction() rolls back and rethrows on failure", async () => {
  const calls: number[] = [];
  const fake = installFakeHost((op) => {
    calls.push(op);
    if (op === OpDBBegin) return { code: 0, meta: { tx: "tx_1" } };
    if (op === OpDBRollback) return { code: 0, meta: {} };
    throw new Error("unexpected op " + op);
  });
  await assert.rejects(
    db("main").transaction(async () => {
      throw new Error("business rule violated");
    }),
    /business rule violated/,
  );
  assert.deepEqual(calls, [OpDBBegin, OpDBRollback]);
  fake.restore();
});

// --- event.emit ----------------------------------------------------------

test("emit() sends the declared event fields and JSON-encodes an object payload", async () => {
  const fake = installFakeHost((op, meta, body) => {
    assert.equal(op, OpEventEmit);
    assert.equal(meta.type, "hello:greeting:greeting:sent");
    assert.equal(meta.dedupId, "greeting-sent-evt_1");
    assert.equal(meta.contentType, "application/json");
    assert.deepEqual(JSON.parse(dec(body!)), { ok: true });
    return { code: 0, meta: { eventId: "evt_out_1" } };
  });
  const id = await emit({ type: "hello:greeting:greeting:sent", dedupId: "greeting-sent-evt_1", data: { ok: true } });
  assert.equal(id, "evt_out_1");
  fake.restore();
});

test("emit() surfaces isRetryable for UNAVAILABLE", async () => {
  const fake = installFakeHost(() => ({ code: 1, meta: { code: "UNAVAILABLE", message: "db down" } }));
  try {
    await emit({ type: "a:b:c:d", dedupId: "1" });
    assert.fail("expected emit to throw");
  } catch (err) {
    assert.equal(isRetryable(err), true);
  }
  fake.restore();
});

test("emit() a permanent refusal is not retryable", async () => {
  const fake = installFakeHost(() => ({ code: 1, meta: { code: "NOT_ALLOWED", message: "not declared" } }));
  try {
    await emit({ type: "a:b:c:d", dedupId: "1" });
    assert.fail("expected emit to throw");
  } catch (err) {
    assert.equal(isRetryable(err), false);
  }
  fake.restore();
});

// --- fetch -----------------------------------------------------------------

test("fetch() round-trips method/url/headers/body and decodes the response", async () => {
  const fake = installFakeHost((op, meta, body) => {
    assert.equal(op, OpHTTPFetch);
    assert.equal(meta.method, "POST");
    assert.equal(meta.url, "https://api.example.com/x");
    assert.equal(dec(body!), "hi");
    return { code: 0, meta: { status: 201, headers: { "Content-Type": ["application/json"] } }, body: enc(`{"ok":true}`) };
  });
  const resp = await fetchFn("https://api.example.com/x", { method: "POST", body: "hi" });
  assert.equal(resp.status, 201);
  assert.deepEqual(resp.json(), { ok: true });
  assert.equal(resp.text(), `{"ok":true}`);
  fake.restore();
});

// --- log / console ---------------------------------------------------------

test("fn.log.* calls op 1 with the right level and swallows host failures", () => {
  const fake = installFakeHost(() => ({ code: 1, meta: { code: "UNAVAILABLE", message: "down" } }));
  assert.doesNotThrow(() => log.error("boom", { a: 1 }));
  assert.equal(fake.calls[0]!.op, OpLog);
  assert.equal(fake.calls[0]!.meta.level, "ERROR");
  assert.equal(fake.calls[0]!.meta.msg, "boom");
  assert.deepEqual(fake.calls[0]!.meta.attrs, { a: 1 });
  fake.restore();
});

test("installConsole routes console.log/info/warn/error/debug through op 1", () => {
  const fake = installFakeHost();
  const target: Record<string, unknown> = {};
  installConsole(target);
  const c = target.console as { log: (...a: unknown[]) => void; warn: (...a: unknown[]) => void };
  c.log("hello", 1, { a: 1 });
  c.warn("careful");
  assert.equal(fake.calls.length, 2);
  assert.equal(fake.calls[0]!.meta.level, "INFO");
  assert.equal(fake.calls[0]!.meta.msg, `hello 1 {"a":1}`);
  assert.equal(fake.calls[1]!.meta.level, "WARN");
  fake.restore();
});

// --- native.callHost ---------------------------------------------------

test("callHost throws UNAVAILABLE when the native isn't installed", () => {
  const g = globalThis as unknown as { __fc_call?: unknown };
  const prev = g.__fc_call;
  g.__fc_call = undefined;
  assert.throws(() => callHost(OpLog, {}), (err: unknown) => err instanceof FnError && err.code === "UNAVAILABLE" && err.retryable);
  g.__fc_call = prev;
});
