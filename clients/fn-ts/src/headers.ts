// FnHeaders / FnQuery: small read-only wrappers over the request frame's
// `headers` (plan §5.2: `{k:[v]}`) and `rawQuery`. QuickJS has no Headers or
// URLSearchParams (those are web/Node APIs, not core ECMAScript), so these
// are hand-rolled over plain JS built-ins (decodeURIComponent, Map).

/** Case-insensitive request headers, values in delivery order. */
export class FnHeaders {
  private readonly byLower = new Map<string, string[]>();

  constructor(init?: Record<string, string[] | string | undefined> | null) {
    if (!init) return;
    for (const [k, v] of Object.entries(init)) {
      if (v === undefined) continue;
      this.byLower.set(k.toLowerCase(), Array.isArray(v) ? v.slice() : [v]);
    }
  }

  /** The first value for `name`, or null. */
  get(name: string): string | null {
    const v = this.byLower.get(name.toLowerCase());
    return v && v.length > 0 ? v[0]! : null;
  }

  /** Every value for `name`, in order. */
  getAll(name: string): string[] {
    return this.byLower.get(name.toLowerCase())?.slice() ?? [];
  }

  has(name: string): boolean {
    return this.byLower.has(name.toLowerCase());
  }

  forEach(fn: (value: string, name: string) => void): void {
    for (const [k, vs] of this.byLower) {
      for (const v of vs) fn(v, k);
    }
  }
}

function decodeComponent(s: string): string {
  try {
    return decodeURIComponent(s.replace(/\+/g, " "));
  } catch {
    return s;
  }
}

/** A small read-only view over a request's parsed query string. */
export class FnQuery {
  private readonly params = new Map<string, string[]>();

  constructor(rawQuery: string) {
    if (!rawQuery) return;
    for (const pair of rawQuery.split("&")) {
      if (!pair) continue;
      const eq = pair.indexOf("=");
      const rawKey = eq < 0 ? pair : pair.slice(0, eq);
      const rawVal = eq < 0 ? "" : pair.slice(eq + 1);
      const key = decodeComponent(rawKey);
      const val = decodeComponent(rawVal);
      const list = this.params.get(key);
      if (list) list.push(val);
      else this.params.set(key, [val]);
    }
  }

  get(name: string): string | null {
    const v = this.params.get(name);
    return v && v.length > 0 ? v[0]! : null;
  }

  getAll(name: string): string[] {
    return this.params.get(name)?.slice() ?? [];
  }

  has(name: string): boolean {
    return this.params.has(name);
  }
}
