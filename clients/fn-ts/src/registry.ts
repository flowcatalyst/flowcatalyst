// registry accumulates every module-top-level declaration (endpoints,
// subscriptions, schedules; config/secret/db/httpAllow/emits are declared
// from config.ts/db.ts/http.ts/event.ts, which push into the same arrays)
// into the data describe.ts and dispatch.ts both read. Registration must
// happen at module top level (the JS analogue of Go's init()); describe.ts
// and dispatch.ts read this registry lazily, on every call, not a snapshot
// taken at import time — so registrations made anywhere in a bundle after
// `import "@flowcatalyst/fn"` are always visible to the engine.
import { compileSegments, splitPattern, type Seg } from "./route";
import type { Handler } from "./types";

export type Auth = "webhook" | "platform" | "none";

export interface CORSConfig {
  origins: string[];
  methods?: string[];
  headers?: string[];
  allowCredentials?: boolean;
}

export interface EndpointOptions {
  timeoutMs?: number;
  maxBodyBytes?: number;
  cors?: CORSConfig;
}

export interface EndpointDef {
  method: string;
  path: string;
  segs: Seg[];
  auth: Auth;
  handler: Handler;
  options: EndpointOptions;
}

export interface SubOptions {
  mode?: string;
  maxRetries?: number;
  dataOnly?: boolean;
  timeoutSeconds?: number;
}
export interface SubDef {
  eventType: string;
  path: string;
  options: SubOptions;
}

export interface SchedOptions {
  timezone?: string;
  payload?: unknown;
}
export interface SchedDef {
  cron: string;
  path: string;
  options: SchedOptions;
}

export interface Registry {
  endpoints: EndpointDef[];
  subs: SubDef[];
  scheds: SchedDef[];
  config: string[];
  secret: string[];
  db: string[];
  httpAllow: string[];
  emits: string[];
}

export const registry: Registry = {
  endpoints: [],
  subs: [],
  scheds: [],
  config: [],
  secret: [],
  db: [],
  httpAllow: [],
  emits: [],
};

/** Clears every declaration. Test-only (see testing.ts): production code
 * never calls this — a guest's registrations are made once and last the
 * instance's lifetime. */
export function resetRegistry(): void {
  registry.endpoints = [];
  registry.subs = [];
  registry.scheds = [];
  registry.config = [];
  registry.secret = [];
  registry.db = [];
  registry.httpAllow = [];
  registry.emits = [];
}

function registerEndpoint(auth: Auth, pattern: string, handler: Handler, options: EndpointOptions = {}): void {
  const { method, path } = splitPattern(pattern);
  if (auth === "webhook" && method !== "POST") {
    throw new Error(`fn: webhook endpoint ${JSON.stringify(pattern)} must be POST-only, got method ${JSON.stringify(method)}`);
  }
  if (typeof handler !== "function") {
    throw new Error(`fn: endpoint ${JSON.stringify(pattern)} registered with a non-function handler`);
  }
  registry.endpoints.push({ method, path, segs: compileSegments(path), auth, handler, options });
}

/** Declares an endpoint with no authentication. */
export function open(pattern: string, handler: Handler, options?: EndpointOptions): void {
  registerEndpoint("none", pattern, handler, options);
}

/** Declares a platform-authenticated endpoint: the runner validates a
 * bearer token against the platform's JWKS before invoking the handler. */
export function platform(pattern: string, handler: Handler, options?: EndpointOptions): void {
  registerEndpoint("platform", pattern, handler, options);
}

/** Declares a webhook-authenticated endpoint (signature verified by the
 * runner). Must be POST-only; any other method throws. */
export function webhook(pattern: string, handler: Handler, options?: EndpointOptions): void {
  registerEndpoint("webhook", pattern, handler, options);
}

/** Declares that this function handles eventType deliveries at path, which
 * must name a webhook endpoint (plan §5.4 validation, enforced at publish). */
export function subscribe(eventType: string, path: string, options: SubOptions = {}): void {
  registry.subs.push({ eventType, path, options });
}

/** Declares a cron-triggered firing at path, which must name a webhook
 * endpoint. */
export function schedule(cron: string, path: string, options: SchedOptions = {}): void {
  registry.scheds.push({ cron, path, options });
}
