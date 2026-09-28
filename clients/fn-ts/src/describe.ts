// Builds the fc_describe document (plan §5.4) from the registry. Must not
// call any host import — and doesn't: it only reads in-memory registration
// state, mirroring clients/fn-go/describe.go / clients/fn-rust/src/app.rs.
import { registry } from "./registry";

export interface DescribeDoc {
  abi: 1;
  endpoints: Record<string, unknown>[];
  subscriptions?: Record<string, unknown>[];
  schedules?: Record<string, unknown>[];
  config?: string[];
  secrets?: string[];
  db?: string[];
  httpAllow?: string[];
  emits?: string[];
}

export function buildDescribeDoc(): DescribeDoc {
  const doc: DescribeDoc = {
    abi: 1,
    endpoints: registry.endpoints.map((e) => {
      const ej: Record<string, unknown> = { method: e.method, path: e.path, auth: e.auth };
      if (e.options.maxBodyBytes !== undefined) ej.maxBodyBytes = e.options.maxBodyBytes;
      if (e.options.timeoutMs !== undefined) ej.timeoutMs = e.options.timeoutMs;
      if (e.options.cors) {
        const c = e.options.cors;
        const corsJSON: Record<string, unknown> = { origins: c.origins };
        if (c.methods) corsJSON.methods = c.methods;
        if (c.headers) corsJSON.headers = c.headers;
        if (c.allowCredentials) corsJSON.allowCredentials = true;
        ej.cors = corsJSON;
      }
      return ej;
    }),
  };
  if (registry.subs.length > 0) {
    doc.subscriptions = registry.subs.map((s) => {
      const sj: Record<string, unknown> = { eventType: s.eventType, path: s.path };
      if (s.options.mode) sj.mode = s.options.mode;
      if (s.options.maxRetries !== undefined) sj.maxRetries = s.options.maxRetries;
      if (s.options.dataOnly) sj.dataOnly = true;
      if (s.options.timeoutSeconds !== undefined) sj.timeoutSeconds = s.options.timeoutSeconds;
      return sj;
    });
  }
  if (registry.scheds.length > 0) {
    doc.schedules = registry.scheds.map((s) => {
      const sj: Record<string, unknown> = { cron: s.cron, path: s.path };
      if (s.options.timezone) sj.timezone = s.options.timezone;
      if (s.options.payload !== undefined) sj.payload = s.options.payload;
      return sj;
    });
  }
  if (registry.config.length > 0) doc.config = registry.config.slice();
  if (registry.secret.length > 0) doc.secrets = registry.secret.slice();
  if (registry.db.length > 0) doc.db = registry.db.slice();
  if (registry.httpAllow.length > 0) doc.httpAllow = registry.httpAllow.slice();
  if (registry.emits.length > 0) doc.emits = registry.emits.slice();
  return doc;
}

/** Renders the current registry as the describe document string returned
 * by globalThis.__fc.describe(). */
export function describeJSON(): string {
  return JSON.stringify(buildDescribeDoc());
}
