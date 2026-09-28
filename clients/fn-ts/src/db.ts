// A database binding over host ops 6-10 (query/exec/begin/commit/rollback,
// plan §5.3), mirroring clients/fn-go/db.go's DBBinding.SQL() / the Rust
// SDK's Context::db_* methods. Unlike config/secret, these go through real
// I/O (Postgres over the wire), so the surface is async.
import { registry } from "./registry";
import { callHost, utf8Decode } from "./native";
import { OpDBBegin, OpDBCommit, OpDBExec, OpDBQuery, OpDBRollback } from "./ops";
import { ErrCodeBadRequest, FnError } from "./errors";

export type DBParam = string | number | boolean | null | undefined | Uint8Array | Date;
export type Row = Record<string, unknown>;

const B64_CHARS = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
const b64 = (n: number): string => B64_CHARS.charAt(n);

/**
 * Decision (plan §5.3 leaves this unspecified, matching clients/fn-go/db.go
 * exactly): Uint8Array params are base64-encoded (standard encoding) into
 * the JSON params array; Date uses toISOString() (RFC3339 with millisecond
 * precision — the JS analogue of the Go SDK's RFC3339Nano). The host side
 * must mirror the Uint8Array choice for bytea columns, since the wire
 * carries no per-parameter type tag.
 */
export function base64Encode(bytes: Uint8Array): string {
  let out = "";
  let i = 0;
  for (; i + 3 <= bytes.length; i += 3) {
    const n = (bytes[i]! << 16) | (bytes[i + 1]! << 8) | bytes[i + 2]!;
    out += b64((n >> 18) & 63) + b64((n >> 12) & 63) + b64((n >> 6) & 63) + b64(n & 63);
  }
  const rem = bytes.length - i;
  if (rem === 1) {
    const n = bytes[i]! << 16;
    out += b64((n >> 18) & 63) + b64((n >> 12) & 63) + "==";
  } else if (rem === 2) {
    const n = (bytes[i]! << 16) | (bytes[i + 1]! << 8);
    out += b64((n >> 18) & 63) + b64((n >> 12) & 63) + b64((n >> 6) & 63) + "=";
  }
  return out;
}

function convertParams(params?: DBParam[]): unknown[] | undefined {
  if (!params || params.length === 0) return undefined;
  return params.map((p) => {
    if (p === null || p === undefined) return null;
    if (p instanceof Uint8Array) return base64Encode(p);
    if (p instanceof Date) return p.toISOString();
    if (typeof p === "number" || typeof p === "string" || typeof p === "boolean") return p;
    throw new FnError(ErrCodeBadRequest, `fn: db: unsupported parameter type ${typeof p}`, false);
  });
}

/**
 * Decodes the host's row JSON (plan §5.3 op 6: "the host will emit keys in
 * select order"). Unlike the Go SDK (which needs a hand-rolled
 * order-preserving decoder because encoding/json's map mode does not
 * preserve key order) and the Rust SDK (which turns on serde_json's
 * preserve_order feature), a bare JSON.parse already preserves the
 * insertion order of an object's own string keys per ECMA-262 — so no
 * special decoder is needed here. The one edge case this does not cover: a
 * column literally named as an unsigned integer (e.g. "0") would sort
 * first under JS's integer-key-ordering rule, same as any other JS object;
 * not worth working around for a column name that is already unusual SQL
 * style.
 */
function decodeRows(body: Uint8Array): Row[] {
  const text = utf8Decode(body);
  if (!text) return [];
  const parsed: unknown = JSON.parse(text);
  return Array.isArray(parsed) ? (parsed as Row[]) : [];
}

async function dbQuery(dbName: string, sql: string, params: DBParam[] | undefined, tx: string | undefined): Promise<Row[]> {
  const { body } = callHost(OpDBQuery, { db: dbName, sql, params: convertParams(params), tx });
  return decodeRows(body);
}

async function dbExec(
  dbName: string,
  sql: string,
  params: DBParam[] | undefined,
  tx: string | undefined,
): Promise<{ rowsAffected: number }> {
  const { meta } = callHost(OpDBExec, { db: dbName, sql, params: convertParams(params), tx });
  return { rowsAffected: typeof meta.rowsAffected === "number" ? meta.rowsAffected : 0 };
}

/** A database binding's connection, scoped to one open transaction. */
export class DBTx {
  constructor(
    private readonly dbName: string,
    private readonly tx: string,
  ) {}
  query(sql: string, params?: DBParam[]): Promise<Row[]> {
    return dbQuery(this.dbName, sql, params, this.tx);
  }
  exec(sql: string, params?: DBParam[]): Promise<{ rowsAffected: number }> {
    return dbExec(this.dbName, sql, params, this.tx);
  }
}

/** A declared database binding (plan §5.3 ops 6-10; plan §4: the platform
 * resolves `name` to a DSN in settings). */
export class DBBinding {
  constructor(private readonly _name: string) {}
  get name(): string {
    return this._name;
  }
  query(sql: string, params?: DBParam[]): Promise<Row[]> {
    return dbQuery(this._name, sql, params, undefined);
  }
  exec(sql: string, params?: DBParam[]): Promise<{ rowsAffected: number }> {
    return dbExec(this._name, sql, params, undefined);
  }
  /** Runs fn inside a transaction: commits when fn's promise resolves,
   * rolls back (and rethrows) when it rejects or throws. */
  async transaction<T>(fn: (tx: DBTx) => Promise<T> | T): Promise<T> {
    const { meta } = callHost(OpDBBegin, { db: this._name });
    const txId = typeof meta.tx === "string" ? meta.tx : "";
    const tx = new DBTx(this._name, txId);
    let result: T;
    try {
      result = await fn(tx);
    } catch (err) {
      try {
        callHost(OpDBRollback, { tx: txId });
      } catch {
        // The original error is what the caller needs to see, not a
        // rollback failure (the runner rolls back an abandoned tx anyway
        // when the invocation ends; plan §6.3).
      }
      throw err;
    }
    callHost(OpDBCommit, { tx: txId });
    return result;
  }
}

/** Declares that this function uses database binding `name`. */
export function db(name: string): DBBinding {
  registry.db.push(name);
  return new DBBinding(name);
}
