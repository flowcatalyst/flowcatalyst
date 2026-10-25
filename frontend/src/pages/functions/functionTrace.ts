/**
 * Non-reactive logic behind the function Diagnostics section (trace logs;
 * docs/function-trace-design.md §10): permission gating, the live-window
 * countdown, filter-to-query mapping and the display shape of `http`
 * records. Kept out of the trace components so it can be pinned in
 * vitest (no @vue/test-utils in this repo).
 */

import type { TraceContext, TraceLogQuery, TraceLogRecord } from "@/api/functions";

/** The API's permission codes (internal/platform/functions/api). */
export const FUNCTION_TRACE_VIEW = "platform:function:trace:view";
export const FUNCTION_TRACE_MANAGE = "platform:function:trace:manage";
export const FUNCTION_TRACE_VIEW_BODIES = "platform:function:trace:view-bodies";
export const FUNCTION_TRACE_INDEFINITE = "platform:function:trace:indefinite";

/** The built-in context: request/response capture, always listed first. */
export const HTTP_CONTEXT = "http";

export interface TraceGates {
	view: boolean;
	manage: boolean;
	viewBodies: boolean;
	/** May leave a declared context on until it is switched off. */
	indefinite: boolean;
}

export function traceGates(has: (permission: string) => boolean): TraceGates {
	return {
		view: has(FUNCTION_TRACE_VIEW),
		manage: has(FUNCTION_TRACE_MANAGE),
		viewBodies: has(FUNCTION_TRACE_VIEW_BODIES),
		indefinite: has(FUNCTION_TRACE_INDEFINITE),
	};
}

/**
 * Whether the enable dialog offers "until switched off" for a context: it has a
 * ceiling (never http, whose minute cap always applies) and the viewer holds
 * platform:function:trace:indefinite.
 */
export function indefiniteAllowed(c: Pick<TraceContext, "indefiniteMaxDays">, gates: TraceGates): boolean {
	return gates.indefinite && (c.indefiniteMaxDays ?? 0) > 0;
}

/** The choice's label: "Until switched off (max 7 days)". */
export function indefiniteLabel(maxDays: number): string {
	return `Until switched off (max ${maxDays} days)`;
}

/** What the confirmation asks the operator to accept. */
export function indefiniteWarning(maxDays: number): string {
	return `This keeps recording every sampled call until someone switches it off, for at most ${maxDays} days; then it switches off by itself.`;
}

/**
 * Why a context's Enable control is unavailable, or null when it is usable.
 * Without manage the controls are not offered at all (hidden, not disabled);
 * `http` without view-bodies is shown disabled with this explanation.
 */
export function enableBlockedReason(contextName: string, gates: TraceGates): string | null {
	if (!gates.manage) return "You do not have permission to change tracing.";
	if (contextName === HTTP_CONTEXT && !gates.viewBodies) {
		return "Capturing http records request and response bodies, which needs the view-bodies permission (platform:function:trace:view-bodies).";
	}
	return null;
}

export interface Countdown {
	live: boolean;
	remainingMs: number;
	/** `m:ss`, or `h:mm:ss` from an hour up; empty when not live. */
	label: string;
}

/**
 * The live state of a context at `nowMs`. Mirrors the server (live iff
 * enabledUntil > now) so a window flips to off on the client the second it
 * passes, without a refetch.
 */
export function countdown(enabledUntil: string | null | undefined, nowMs: number): Countdown {
	const until = enabledUntil ? Date.parse(enabledUntil) : Number.NaN;
	const remainingMs = until - nowMs;
	if (Number.isNaN(until) || remainingMs <= 0) return { live: false, remainingMs: 0, label: "" };
	const total = Math.ceil(remainingMs / 1000);
	const h = Math.floor(total / 3600);
	const m = Math.floor((total % 3600) / 60);
	const s = total % 60;
	const ss = String(s).padStart(2, "0");
	return {
		live: true,
		remainingMs,
		label: h > 0 ? `${h}:${String(m).padStart(2, "0")}:${ss}` : `${m}:${ss}`,
	};
}

const DURATION_CHOICES = [5, 15, 30, 60];

/** 5/15/30/60 minutes limited to the context's cap; a cap below 5 offers just the cap. */
export function durationChoices(maxMinutes: number): number[] {
	const choices = DURATION_CHOICES.filter((m) => m <= maxMinutes);
	return choices.length ? choices : [Math.max(1, maxMinutes)];
}

export const SAMPLE_RATE_MIN = 1;
export const SAMPLE_RATE_MAX = 1000;

export function isValidSampleRate(n: unknown): n is number {
	return typeof n === "number" && Number.isInteger(n) && n >= SAMPLE_RATE_MIN && n <= SAMPLE_RATE_MAX;
}

/** One-line state for the contexts table: "Off" or "Live · 1 in 5". */
export function sampleLabel(sampleRate: number): string {
	return sampleRate > 1 ? `1 in ${sampleRate}` : "all invocations";
}

/** A context's state now: live (with a countdown, or "until switched off") or off. */
export interface ContextState {
	live: boolean;
	indefinite: boolean;
	/** Countdown text; for an indefinite window, when it switches off by itself; empty when off. */
	label: string;
}

export function contextState(c: TraceContext, nowMs: number): ContextState {
	const cd = countdown(c.enabledUntil, nowMs);
	if (!cd.live) return { live: false, indefinite: false, label: "" };
	if (c.indefinite) return { live: true, indefinite: true, label: localTime(c.enabledUntil ?? "") };
	return { live: true, indefinite: false, label: cd.label };
}

export function anyLive(contexts: TraceContext[], nowMs: number): boolean {
	return contexts.some((c) => contextState(c, nowMs).live);
}

export const SINCE_CHOICES = [
	{ label: "Any time", minutes: 0 },
	{ label: "Last 5 minutes", minutes: 5 },
	{ label: "Last hour", minutes: 60 },
	{ label: "Last 24 hours", minutes: 24 * 60 },
] as const;

export const KIND_CHOICES = ["trace", "http", "log"] as const;
export const LEVEL_CHOICES = ["debug", "info", "warn", "error"] as const;

export interface TraceFilters {
	context: string;
	kind: string;
	level: string;
	reference: string;
	correlationId: string;
	invocationId: string;
	/** Quick choice: records from the last N minutes; 0 = no lower bound. */
	sinceMinutes: number;
}

export function emptyFilters(): TraceFilters {
	return { context: "", kind: "", level: "", reference: "", correlationId: "", invocationId: "", sinceMinutes: 0 };
}

export const TRACE_PAGE_SIZE = 100;

/**
 * The trace-log query for a filter set: blank fields are left out, `since` is
 * an RFC 3339 instant computed from the quick choice at `nowMs`.
 */
export function buildTraceQuery(
	filters: TraceFilters,
	nowMs: number,
	opts: { cursor?: string | null; limit?: number } = {},
): TraceLogQuery {
	const q: TraceLogQuery = { limit: opts.limit ?? TRACE_PAGE_SIZE };
	if (filters.context) q.context = filters.context;
	if (filters.kind) q.kind = filters.kind as TraceLogQuery["kind"];
	if (filters.level) q.level = filters.level;
	if (filters.reference.trim()) q.reference = filters.reference.trim();
	if (filters.correlationId.trim()) q.correlationId = filters.correlationId.trim();
	if (filters.invocationId.trim()) q.invocationId = filters.invocationId.trim();
	if (filters.sinceMinutes > 0) q.since = new Date(nowMs - filters.sinceMinutes * 60_000).toISOString();
	if (opts.cursor) q.cursor = opts.cursor;
	return q;
}

export function hasActiveFilters(filters: TraceFilters): boolean {
	return JSON.stringify(filters) !== JSON.stringify(emptyFilters());
}

/** Records per invocation id in the loaded page, for the "N records" badge. */
export function invocationCounts(records: TraceLogRecord[]): Map<string, number> {
	const counts = new Map<string, number>();
	for (const r of records) counts.set(r.invocationId, (counts.get(r.invocationId) ?? 0) + 1);
	return counts;
}

/** Shortened id for table cells; the full id is the tooltip and the filter value. */
export function shortId(id: string): string {
	return id.length > 12 ? `${id.slice(0, 8)}…${id.slice(-3)}` : id;
}

export function localTime(iso: string): string {
	const t = Date.parse(iso);
	return Number.isNaN(t) ? iso : new Date(t).toLocaleString();
}

export function levelSeverity(level: string): "danger" | "warn" | "info" | "secondary" {
	switch (level) {
		case "error":
			return "danger";
		case "warn":
			return "warn";
		case "info":
			return "info";
		default:
			return "secondary";
	}
}

export interface HttpBodyData {
	body: string | null;
	bodyEncoding?: string;
	bodyBytes?: number;
	bodyTruncated?: boolean;
}

export interface HttpRequestData extends HttpBodyData {
	method?: string;
	path?: string;
	rawQuery?: string;
	headers?: Record<string, string[] | string>;
	caller?: unknown;
}

export interface HttpResponseData extends HttpBodyData {
	status?: number;
	headers?: Record<string, string[] | string>;
}

export interface HttpTraceData {
	request: HttpRequestData;
	response: HttpResponseData;
	durationMs?: number;
	outcome?: string;
}

/** The `data` of a kind:"http" record, or null when it is not the expected shape. */
export function parseHttpData(data: unknown): HttpTraceData | null {
	if (!data || typeof data !== "object") return null;
	const d = data as Partial<HttpTraceData>;
	if (!d.request || typeof d.request !== "object") return null;
	if (!d.response || typeof d.response !== "object") return null;
	return d as HttpTraceData;
}

/** Header map as sorted [name, value] rows; multi-value headers join with ", ". */
export function headerRows(headers: Record<string, string[] | string> | undefined | null): [string, string][] {
	return Object.entries(headers ?? {})
		.map(([k, v]): [string, string] => [k, Array.isArray(v) ? v.join(", ") : String(v)])
		.sort((a, b) => a[0].localeCompare(b[0]));
}

export interface BodyView {
	/** "hidden": viewer lacks view-bodies; "empty": no body captured; "shown": `text` is set. */
	state: "hidden" | "empty" | "shown";
	text: string;
	/** "JSON" (pretty-printed), "text", or "base64". */
	label: string;
	/** "truncated (N bytes total)" when the capture was cut short. */
	note: string;
}

export const BODIES_HIDDEN_MESSAGE = "Bodies, headers and query string hidden: you do not have permission to view them";

/** Shown on a log or trace record whose line or data was withheld (no view-bodies). */
export const DETAIL_HIDDEN_MESSAGE =
	"Details hidden: you do not have permission to view what the function logged (platform:function:trace:view-bodies)";

/** A record's headline in the table; "(hidden)" for a log line withheld from the viewer. */
export function recordMessage(r: Pick<TraceLogRecord, "message" | "redacted">): string {
	return !r.message && r.redacted ? "(hidden)" : r.message;
}

/** How a request or response body is displayed (pretty JSON, labelled base64, truncation note, redaction). */
export function bodyView(part: HttpBodyData | undefined, redacted: boolean): BodyView {
	if (redacted) return { state: "hidden", text: "", label: "", note: "" };
	if (!part || part.body == null || part.body === "") {
		return { state: "empty", text: "", label: "", note: truncationNote(part) };
	}
	const note = truncationNote(part);
	if (part.bodyEncoding === "base64") return { state: "shown", text: part.body, label: "base64", note };
	// A truncated capture cuts the document mid-way, so it will not parse: shown raw.
	if (!part.bodyTruncated) {
		try {
			return { state: "shown", text: JSON.stringify(JSON.parse(part.body), null, 2), label: "JSON", note };
		} catch {
			// not JSON
		}
	}
	return { state: "shown", text: part.body, label: "text", note };
}

function truncationNote(part: HttpBodyData | undefined): string {
	if (!part?.bodyTruncated) return "";
	return part.bodyBytes != null ? `truncated (${part.bodyBytes.toLocaleString()} bytes total)` : "truncated";
}

/** Pretty JSON for a trace/log record's `data`; null when there is none. */
export function dataText(data: unknown): string | null {
	if (data == null) return null;
	return typeof data === "string" ? data : JSON.stringify(data, null, 2);
}

/** Headline of an http record: "POST /orders → 200 · 12 ms · ok". */
export function httpSummary(data: HttpTraceData): string {
	const req = data.request;
	const path = `${req.path ?? ""}${req.rawQuery ? `?${req.rawQuery}` : ""}`;
	const parts = [`${req.method ?? "?"} ${path}`.trim()];
	if (data.response.status != null) parts[0] += ` → ${data.response.status}`;
	if (data.durationMs != null) parts.push(`${data.durationMs} ms`);
	if (data.outcome) parts.push(data.outcome);
	return parts.join(" · ");
}
