// Turns a request frame (meta JSON string + body bytes, as the engine hands
// them to globalThis.__fc.handle) into a call through the registered
// routes, and the handler's return value back into a response frame.
// Mirrors clients/fn-go/dispatch.go: a malformed frame or a handler
// throw/rejection never propagates past this module — both become
// `500 {"error":"FUNCTION_PANIC"}`, logged via host op 1.
import { registry, type EndpointDef } from "./registry";
import { Router } from "./route";
import { callerFromMeta } from "./caller";
import { FnHeaders, FnQuery } from "./headers";
import { callHost, utf8Decode, utf8Encode } from "./native";
import { OpLog } from "./ops";
import type { FnRequest, FnResponseLike, Invocation } from "./types";

interface RequestMeta {
  id?: string;
  address?: string;
  version?: number;
  method?: string;
  path?: string;
  rawQuery?: string;
  headers?: Record<string, string[]>;
  route?: string;
  pathParams?: Record<string, string>;
  caller?: unknown;
  deadlineUnixMs?: number;
}

interface ResponseMetaOut {
  status: number;
  headers?: Record<string, string[]>;
}

export interface Frame {
  meta: string;
  body: Uint8Array;
}

const PANIC_BODY = `{"error":"FUNCTION_PANIC"}`;

function panicFrame(): Frame {
  const meta: ResponseMetaOut = { status: 500, headers: { "Content-Type": ["application/json"] } };
  return { meta: JSON.stringify(meta), body: utf8Encode(PANIC_BODY) };
}

function logPanic(err: unknown): void {
  try {
    const message = err instanceof Error ? (err.stack ?? err.message) : String(err);
    callHost(OpLog, { level: "ERROR", msg: "handler panic", attrs: { panic: message } });
  } catch {
    // never fail on a logging failure
  }
}

function buildRouter(): Router {
  const entries = registry.endpoints.map((e, i) => ({ index: i, method: e.method, path: e.path, segs: e.segs }));
  return new Router(entries);
}

function makeRequest(rm: RequestMeta, body: Uint8Array, params: Record<string, string>): FnRequest {
  const headers = new FnHeaders(rm.headers ?? {});
  const query = new FnQuery(rm.rawQuery ?? "");
  const caller = callerFromMeta(rm.caller);
  const invocation: Invocation = {
    id: rm.id ?? "",
    address: rm.address ?? "",
    version: rm.version ?? 0,
    deadline: new Date(rm.deadlineUnixMs ?? 0),
  };
  let cachedText: string | undefined;
  const req: FnRequest = {
    id: rm.id ?? "",
    method: rm.method ?? "",
    path: rm.path ?? "",
    query,
    headers,
    params,
    body,
    text(): string {
      if (cachedText === undefined) cachedText = utf8Decode(body);
      return cachedText;
    },
    json(): unknown {
      return JSON.parse(req.text());
    },
    caller,
    invocation,
  };
  return req;
}

interface RawResponse {
  status: number;
  headers: Record<string, string[]>;
  bodyBytes: Uint8Array;
}

function normalizeResponse(r: FnResponseLike | void): RawResponse {
  const resp: FnResponseLike = r ?? { status: 200 };
  const headersIn = resp.headers ?? {};
  const headers: Record<string, string[]> = {};
  let hasContentType = false;
  for (const [k, v] of Object.entries(headersIn)) {
    headers[k] = Array.isArray(v) ? v : [v];
    if (k.toLowerCase() === "content-type") hasContentType = true;
  }

  let bodyBytes: Uint8Array;
  const body = resp.body;
  if (body === undefined || body === null) {
    bodyBytes = new Uint8Array(0);
  } else if (body instanceof Uint8Array) {
    bodyBytes = body;
  } else if (typeof body === "string") {
    bodyBytes = utf8Encode(body);
  } else {
    bodyBytes = utf8Encode(JSON.stringify(body));
    if (!hasContentType) headers["Content-Type"] = ["application/json"];
  }
  return { status: resp.status, headers, bodyBytes };
}

async function runHandler(endpoint: EndpointDef, req: FnRequest): Promise<RawResponse> {
  try {
    const result = await endpoint.handler(req);
    return normalizeResponse(result);
  } catch (err) {
    logPanic(err);
    return { status: 500, headers: { "Content-Type": ["application/json"] }, bodyBytes: utf8Encode(PANIC_BODY) };
  }
}

/** Decodes a request frame, dispatches it through the registered routes,
 * and encodes the response frame. Never throws: a malformed meta frame or a
 * handler failure both resolve to a 500 FUNCTION_PANIC frame. */
export async function handleRequest(metaStr: string, body: Uint8Array): Promise<Frame> {
  let rm: RequestMeta;
  try {
    rm = JSON.parse(metaStr) as RequestMeta;
  } catch {
    return panicFrame();
  }

  const router = buildRouter();
  const result = router.match(rm.method ?? "", rm.path ?? "");

  if (result.kind === "no-match") {
    return { meta: JSON.stringify({ status: 404 } satisfies ResponseMetaOut), body: new Uint8Array(0) };
  }
  if (result.kind === "method-not-allowed") {
    const meta: ResponseMetaOut = { status: 405, headers: { Allow: [result.allow.join(", ")] } };
    return { meta: JSON.stringify(meta), body: new Uint8Array(0) };
  }

  const endpoint = registry.endpoints[result.index];
  if (!endpoint) {
    // Unreachable: result.index was produced from the same registry.endpoints
    // array buildRouter() just snapshotted. Defensive, not a real case.
    return panicFrame();
  }
  const req = makeRequest(rm, body, result.params);
  const out = await runHandler(endpoint, req);
  if (result.headFallback) {
    out.bodyBytes = new Uint8Array(0);
  }

  const respMeta: ResponseMetaOut = { status: out.status };
  if (Object.keys(out.headers).length > 0) respMeta.headers = out.headers;
  return { meta: JSON.stringify(respMeta), body: out.bodyBytes };
}
