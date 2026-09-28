// A ServeMux-compatible routing subset (plan §9 "Routing"), mirroring
// clients/fn-rust/src/route.rs's pattern compiling/matching plus the
// most-specific-wins / 404-vs-405 / GET-serves-HEAD rules Go 1.22's
// http.ServeMux (and internal/functions/abi.Router, which wraps that same
// ServeMux) implement. The guest re-derives its own match from method+path
// rather than trusting the host's `route`/`pathParams` fields — the same
// "let the router own matching" choice both reference SDKs make.

export type SegKind = "literal" | "param" | "rest";
export interface Seg {
  readonly kind: SegKind;
  readonly value: string;
}

/** Splits a path into non-empty segments ("/" and "" both yield []). */
export function splitPath(path: string): string[] {
  const trimmed = path.replace(/^\/+/, "");
  if (trimmed === "") return [];
  return trimmed.split("/").filter((s) => s.length > 0);
}

/** Parses a "METHOD /path" registration pattern (same syntax the Go SDK and
 * Go 1.22's http.ServeMux accept). Every fn.open/platform/webhook
 * registration must name a method explicitly, tighter than bare ServeMux
 * patterns — the same deliberate choice clients/fn-go/registry.go makes. */
export function splitPattern(pattern: string): { method: string; path: string } {
  const i = pattern.indexOf(" ");
  if (i < 0) {
    throw new Error(`fn: endpoint pattern must be "METHOD /path", got ${JSON.stringify(pattern)}`);
  }
  const method = pattern.slice(0, i);
  const path = pattern.slice(i + 1).trim();
  if (!method || !path) {
    throw new Error(`fn: endpoint pattern must be "METHOD /path", got ${JSON.stringify(pattern)}`);
  }
  return { method, path };
}

/** Compiles a path into segments. A trailing "{name...}" segment must be
 * last; any other placement throws (mirrors Go 1.22 ServeMux). */
export function compileSegments(path: string): Seg[] {
  const parts = splitPath(path);
  const segs: Seg[] = parts.map((seg) => {
    if (seg.startsWith("{") && seg.endsWith("...}")) {
      return { kind: "rest", value: seg.slice(1, -4) };
    }
    if (seg.startsWith("{") && seg.endsWith("}")) {
      return { kind: "param", value: seg.slice(1, -1) };
    }
    return { kind: "literal", value: seg };
  });
  for (let i = 0; i < segs.length - 1; i++) {
    if (segs[i]!.kind === "rest") {
      throw new Error(`fn: path ${JSON.stringify(path)}: "{...}" must be the final segment`);
    }
  }
  return segs;
}

/** Matches path segments against compiled pattern segments, extracting path
 * params. A trailing rest segment consumes every remaining segment,
 * including zero. */
export function matchSegments(segs: Seg[], pathSegs: string[]): Record<string, string> | null {
  const params: Record<string, string> = {};
  let pi = 0;
  for (let i = 0; i < segs.length; i++) {
    const seg = segs[i]!;
    if (seg.kind === "rest") {
      params[seg.value] = pathSegs.slice(pi).join("/");
      return params;
    }
    if (seg.kind === "literal") {
      if (pi >= pathSegs.length || pathSegs[pi] !== seg.value) return null;
      pi++;
    } else {
      if (pi >= pathSegs.length) return null;
      params[seg.value] = pathSegs[pi]!;
      pi++;
    }
  }
  return pi === pathSegs.length ? params : null;
}

export interface RouteEntry {
  readonly index: number;
  readonly method: string;
  readonly path: string;
  readonly segs: Seg[];
}

export interface RouteFound {
  readonly kind: "found";
  readonly index: number;
  readonly params: Record<string, string>;
  /** True when a HEAD request matched a GET-only registration: the
   * dispatcher runs the GET handler but must suppress the response body. */
  readonly headFallback: boolean;
}
export interface RouteMethodNotAllowed {
  readonly kind: "method-not-allowed";
  readonly allow: string[];
}
export interface RouteNoMatch {
  readonly kind: "no-match";
}
export type RouteResult = RouteFound | RouteMethodNotAllowed | RouteNoMatch;

function segRank(seg: Seg | undefined): number {
  if (!seg) return -1;
  if (seg.kind === "literal") return 2;
  if (seg.kind === "param") return 1;
  return 0; // rest
}

/** Orders entries most-specific first: literal beats param beats rest at
 * each position; a non-rest pattern beats a rest pattern of otherwise equal
 * specificity; a longer pattern beats a shorter one; ties keep registration
 * order. This is the "most-specific pattern wins" subset of Go's ServeMux
 * precedence rule (plan §9), not full conflict detection. */
function compareSpecificity(a: RouteEntry, b: RouteEntry): number {
  const len = Math.max(a.segs.length, b.segs.length);
  for (let i = 0; i < len; i++) {
    const ra = segRank(a.segs[i]);
    const rb = segRank(b.segs[i]);
    if (ra !== rb) return rb - ra;
  }
  const aRest = a.segs.length > 0 && a.segs[a.segs.length - 1]!.kind === "rest" ? 1 : 0;
  const bRest = b.segs.length > 0 && b.segs[b.segs.length - 1]!.kind === "rest" ? 1 : 0;
  if (aRest !== bRest) return aRest - bRest;
  if (a.segs.length !== b.segs.length) return b.segs.length - a.segs.length;
  return a.index - b.index;
}

export class Router {
  constructor(private readonly entries: readonly RouteEntry[]) {}

  private candidatesFor(method: string, pathSegs: string[]): { entry: RouteEntry; params: Record<string, string> }[] {
    const out: { entry: RouteEntry; params: Record<string, string> }[] = [];
    for (const entry of this.entries) {
      if (entry.method !== method) continue;
      const params = matchSegments(entry.segs, pathSegs);
      if (params) out.push({ entry, params });
    }
    return out;
  }

  /** Matches method+path against every registered endpoint (plan §9):
   * "404 when nothing matches, 405 (+ Allow header) when the path matches
   * but not the method". GET registrations also answer HEAD requests. */
  match(method: string, path: string): RouteResult {
    const pathSegs = splitPath(path);

    const exact = this.candidatesFor(method, pathSegs);
    if (exact.length > 0) {
      const best = exact.slice().sort((x, y) => compareSpecificity(x.entry, y.entry))[0]!;
      return { kind: "found", index: best.entry.index, params: best.params, headFallback: false };
    }

    if (method === "HEAD") {
      const getCandidates = this.candidatesFor("GET", pathSegs);
      if (getCandidates.length > 0) {
        const best = getCandidates.slice().sort((x, y) => compareSpecificity(x.entry, y.entry))[0]!;
        return { kind: "found", index: best.entry.index, params: best.params, headFallback: true };
      }
    }

    const allowed = new Set<string>();
    for (const entry of this.entries) {
      if (matchSegments(entry.segs, pathSegs)) allowed.add(entry.method);
    }
    if (allowed.size === 0) return { kind: "no-match" };
    if (allowed.has("GET")) allowed.add("HEAD");
    return { kind: "method-not-allowed", allow: [...allowed].sort() };
  }
}
