// Principal / Caller (plan §5.2): a verified caller identity, mirroring
// internal/platform/shared/auth.AuthContext's semantics exactly, the same
// contract clients/fn-go/caller.go and clients/fn-rust/src/types.rs
// implement — including HasPermission's `*` segment-wildcard matching.

export interface PrincipalData {
  id?: string;
  type?: string;
  tier?: string;
  clients?: string[];
  roles?: string[];
  applications?: string[];
  allApplications?: boolean;
  permissions?: string[];
}

/** A held permission may contain `*` segment wildcards (e.g.
 * "platform:messaging:*:*" or the super-admin "platform:*:*:*"): such a
 * permission matches any code with the same colon-separated segment count
 * whose non-wildcard segments are equal. Mirrors the Go/Rust SDKs' (and the
 * platform's) permissionMatches exactly. */
export function permissionMatches(held: string, required: string): boolean {
  if (held === required) return true;
  const h = held.split(":");
  const r = required.split(":");
  if (h.length !== r.length) return false;
  for (let i = 0; i < h.length; i++) {
    if (h[i] !== "*" && h[i] !== r[i]) return false;
  }
  return true;
}

export class Principal {
  readonly id: string;
  readonly type: string;
  readonly tier: string;
  readonly clients: readonly string[];
  readonly roles: readonly string[];
  readonly applications: readonly string[];
  readonly allApplications: boolean;
  readonly permissions: readonly string[];

  constructor(data: PrincipalData) {
    this.id = data.id ?? "";
    this.type = data.type ?? "";
    this.tier = data.tier ?? "";
    this.clients = data.clients ?? [];
    this.roles = data.roles ?? [];
    this.applications = data.applications ?? [];
    this.allApplications = data.allApplications ?? false;
    this.permissions = data.permissions ?? [];
  }

  /** True when the principal has anchor (platform-wide) scope. */
  isAnchor(): boolean {
    return this.tier === "ANCHOR";
  }

  /** True when the principal carries the exact role code (no wildcarding). */
  hasRole(role: string): boolean {
    return this.roles.includes(role);
  }

  /** True for anchor principals, or when clientId is in `clients`. */
  canAccessClient(clientId: string): boolean {
    return this.isAnchor() || this.clients.includes(clientId);
  }

  /** True when the principal holds all-applications access, or when
   * applicationId is in its explicit `applications` list. */
  canAccessApplication(applicationId: string): boolean {
    return this.allApplications || this.applications.includes(applicationId);
  }

  /** True when the principal carries a permission satisfying `code`. */
  hasPermission(code: string): boolean {
    return this.permissions.some((held) => permissionMatches(held, code));
  }
}

/** Who made this invocation (plan §5.2). */
export interface Caller {
  readonly kind: "webhook" | "anonymous" | "principal" | string;
  /** The verified principal, set only when kind === "principal" (the
   * endpoint's auth is "platform"). */
  readonly principal?: Principal;
}

export function callerFromMeta(cm: unknown): Caller {
  if (!cm || typeof cm !== "object") return { kind: "anonymous" };
  const obj = cm as Record<string, unknown>;
  const kind = typeof obj.kind === "string" && obj.kind ? obj.kind : "anonymous";
  if (kind === "principal") {
    return { kind, principal: new Principal(obj as PrincipalData) };
  }
  return { kind };
}
