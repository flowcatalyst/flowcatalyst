// Request/response shapes handlers see (plan §5.2), shared between
// registry.ts (Handler) and dispatch.ts (the frame <-> FnRequest/response
// translation) without either importing the other.
import type { Caller } from "./caller";
import type { FnHeaders, FnQuery } from "./headers";

export interface Invocation {
  readonly id: string;
  readonly address: string;
  readonly version: number;
  readonly deadline: Date;
}

export interface FnRequest {
  readonly id: string;
  readonly method: string;
  readonly path: string;
  readonly query: FnQuery;
  readonly headers: FnHeaders;
  readonly params: Readonly<Record<string, string>>;
  readonly body: Uint8Array;
  /** Decodes the body as UTF-8 (cached after the first call). */
  text(): string;
  /** Parses the body as JSON (`JSON.parse(this.text())`). */
  json(): unknown;
  readonly caller: Caller;
  readonly invocation: Invocation;
}

export type ResponseBody = string | Uint8Array | Record<string, unknown> | unknown[] | null | undefined;

export interface FnResponseLike {
  status: number;
  headers?: Record<string, string | string[]>;
  body?: ResponseBody;
}

export type Handler = (req: FnRequest) => FnResponseLike | void | Promise<FnResponseLike | void>;
