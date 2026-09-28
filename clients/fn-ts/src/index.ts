// @flowcatalyst/fn: the JavaScript/TypeScript guest SDK for FlowCatalyst
// functions (docs/function-runner-plan.md §9), running on the function
// runner's shared QuickJS engine. A JS function's artifact is this bundled
// script, produced with `fc-fn-build` (bundle: esbuild --bundle
// --format=iife --platform=neutral --target=es2020).
//
// Importing this module installs globalThis.__fc (the engine's binding
// protocol; see clients/fn-js-engine/src/lib.rs's module doc) and, when
// nothing has defined one yet, a globalThis.console routed through host op
// 1 — QuickJS has neither by default. Endpoint/subscription/schedule/
// config/secret/db/httpAllow/emits declarations must happen at module top
// level (the JS analogue of Go's init()): describe() and handle() read the
// registry lazily, on every call, so anything registered anywhere in the
// bundle before the engine's first call is visible to them.
import { open, platform, webhook, subscribe, schedule } from "./registry";
import { config, secret, ConfigValue, SecretValue } from "./config";
import { db, DBBinding, DBTx } from "./db";
import { httpAllow, fetchFn, FnResponse } from "./http";
import { emit, emits } from "./event";
import { json, text, status, retry, reject } from "./respond";
import { log, installConsole } from "./log";
import { describeJSON } from "./describe";
import { handleRequest } from "./dispatch";
import { FnError, isRetryable } from "./errors";
import { Principal, permissionMatches } from "./caller";

export { FnError, isRetryable, Principal, permissionMatches };
export { ConfigValue, SecretValue, DBBinding, DBTx, FnResponse };
export type { FnRequest, FnResponseLike, ResponseBody, Invocation } from "./types";
export type { Caller } from "./caller";
export type { EventInput } from "./event";
export type { FetchOptions } from "./http";
export type { CORSConfig, EndpointOptions, SubOptions, SchedOptions, Auth } from "./registry";
export type { Row, DBParam } from "./db";

/** Installs globalThis.__fc = {describe, handle}. Exposed mainly for tests
 * that want to install it onto a scratch object instead of globalThis;
 * index.ts calls this against globalThis at import time. */
export function installFc(target: Record<string, unknown> = globalThis as unknown as Record<string, unknown>): void {
  target.__fc = {
    describe(): string {
      return describeJSON();
    },
    handle(meta: string, body: Uint8Array) {
      return handleRequest(meta, body);
    },
  };
}

installFc();
if (typeof (globalThis as unknown as { console?: unknown }).console === "undefined") {
  installConsole();
}

/** The single namespace object guest code uses to declare and respond
 * (plan §9's TypeScript API). */
export const fn = {
  // Declarations (call at module top level).
  open,
  platform,
  webhook,
  subscribe,
  schedule,
  config,
  secret,
  db,
  httpAllow,
  emits,

  // Capabilities.
  emit,
  fetch: fetchFn,
  log,

  // Response builders.
  json,
  text,
  status,
  retry,
  reject,
};
