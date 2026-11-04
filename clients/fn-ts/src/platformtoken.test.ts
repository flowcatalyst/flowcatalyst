import { test, beforeEach } from "node:test";
import assert from "node:assert/strict";
import { platformToken, usesPlatformToken } from "./platformtoken";
import { FnError, isRetryable } from "./errors";
import { OpPlatformToken } from "./ops";
import { buildDescribeDoc } from "./describe";
import { installFakeHost, resetRegistry } from "./testing";

beforeEach(() => {
  resetRegistry();
});

test("describe omits platformToken without the declaration and carries it with", () => {
  assert.equal("platformToken" in buildDescribeDoc(), false);
  usesPlatformToken();
  assert.equal(buildDescribeDoc().platformToken, true);
  resetRegistry();
  assert.equal("platformToken" in buildDescribeDoc(), false);
});

test("platformToken() returns the access token and sends an empty meta", async () => {
  const fake = installFakeHost((op, meta, body) => {
    assert.equal(op, OpPlatformToken);
    assert.deepEqual(meta, {});
    assert.equal(body, undefined);
    return { code: 0, meta: { accessToken: "fake-token-1", expiresAt: 1793736000000 } };
  });
  try {
    assert.equal(await platformToken(), "fake-token-1");
    assert.equal(fake.calls.length, 1);
  } finally {
    fake.restore();
  }
});

test("platformToken({ refresh: true }) sends the refresh flag; refresh: false does not", async () => {
  const fake = installFakeHost(() => ({ code: 0, meta: { accessToken: "fake-token-2", expiresAt: 1 } }));
  try {
    await platformToken({ refresh: true });
    await platformToken({ refresh: false });
    assert.deepEqual(fake.calls[0]!.meta, { refresh: true });
    assert.deepEqual(fake.calls[1]!.meta, {});
  } finally {
    fake.restore();
  }
});

test("platformToken() surfaces each host error code as FnError; only UNAVAILABLE is retryable", async () => {
  for (const [code, retryable] of [
    ["UNAVAILABLE", true],
    ["NOT_DECLARED", false],
    ["NOT_ALLOWED", false],
    ["DEADLINE", false],
  ] as const) {
    const fake = installFakeHost(() => ({ code: 1, meta: { code, message: "PLATFORM_TOKEN_NOT_DECLARED" } }));
    try {
      await assert.rejects(platformToken(), (err: unknown) => {
        assert.ok(err instanceof FnError);
        assert.equal(err.code, code);
        assert.match(err.message, /PLATFORM_TOKEN_NOT_DECLARED/);
        assert.equal(isRetryable(err), retryable, code);
        return true;
      });
    } finally {
      fake.restore();
    }
  }
});

test("platformToken() turns UNKNOWN_OP into a clear 'runner does not support platform tokens' error", async () => {
  const fake = installFakeHost(() => ({ code: 1, meta: { code: "UNKNOWN_OP", message: "op 15" } }));
  try {
    await assert.rejects(platformToken(), (err: unknown) => {
      assert.ok(err instanceof FnError);
      assert.equal(err.code, "UNKNOWN_OP");
      assert.match(err.message, /runner does not support platform tokens/);
      assert.equal(isRetryable(err), false);
      return true;
    });
  } finally {
    fake.restore();
  }
});
