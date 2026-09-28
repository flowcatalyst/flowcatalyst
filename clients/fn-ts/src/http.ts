// fetch (plan §5.3 op 4, http.fetch), mirroring clients/fn-go/http.go's
// HTTPClient()/RoundTripper, as a fetch-shaped async function instead of a
// RoundTripper (there is no net/http equivalent to extend in TypeScript).
import { registry } from "./registry";
import { callHost, utf8Decode, utf8Encode } from "./native";
import { OpHTTPFetch } from "./ops";

export interface FetchOptions {
  method?: string;
  headers?: Record<string, string | string[]>;
  body?: string | Uint8Array;
  timeoutMs?: number;
}

function normalizeHeaders(h?: Record<string, string | string[]>): Record<string, string[]> | undefined {
  if (!h) return undefined;
  const out: Record<string, string[]> = {};
  for (const [k, v] of Object.entries(h)) out[k] = Array.isArray(v) ? v : [v];
  return out;
}

/** An outbound response, fully buffered (the ABI carries no streaming). */
export class FnResponse {
  constructor(
    readonly status: number,
    readonly headers: Readonly<Record<string, string[]>>,
    private readonly bodyBytes: Uint8Array,
  ) {}
  text(): string {
    return utf8Decode(this.bodyBytes);
  }
  json(): unknown {
    return JSON.parse(this.text());
  }
  bytes(): Uint8Array {
    return this.bodyBytes;
  }
}

/** Runs an outbound HTTP request through host op 4. Only hosts declared
 * with httpAllow() (and matched by the runner's allowlist, including on
 * redirect) may be reached. */
export async function fetchFn(url: string, opts: FetchOptions = {}): Promise<FnResponse> {
  const bodyBytes = opts.body === undefined ? undefined : typeof opts.body === "string" ? utf8Encode(opts.body) : opts.body;
  const meta = {
    method: opts.method ?? "GET",
    url,
    headers: normalizeHeaders(opts.headers),
    timeoutMs: opts.timeoutMs,
  };
  const { meta: rm, body } = callHost(OpHTTPFetch, meta, bodyBytes);
  const status = typeof rm.status === "number" ? rm.status : 0;
  const headers = (rm.headers && typeof rm.headers === "object" ? rm.headers : {}) as Record<string, string[]>;
  return new FnResponse(status, headers, body);
}

/** Declares outbound HTTP hosts this function may reach. Wildcards like
 * "*.acme.com" are supported. */
export function httpAllow(...hosts: string[]): void {
  registry.httpAllow.push(...hosts);
}
