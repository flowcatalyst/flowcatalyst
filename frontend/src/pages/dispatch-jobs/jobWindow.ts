/**
 * The dispatch job list's time window. The server holds every list-shaped
 * read to a created_at window (default 24 hours, at most 31 days) and echoes
 * the one it applied; this is the picker's logic, kept out of the component
 * so tests/dispatch-jobs-window.test.ts can pin it.
 */

const HOUR = 3_600_000;
const DAY = 24 * HOUR;

export interface WindowPreset {
	key: string;
	label: string;
	ms: number;
}

/** The server's default window; "24h" sends nothing and lets the server apply it. */
export const DEFAULT_WINDOW_KEY = "24h";
/** The server's cap (FC_DISPATCH_JOB_MAX_WINDOW_DAYS default); not echoed, so held here. */
export const MAX_WINDOW_MS = 31 * DAY;

export const WINDOW_PRESETS: readonly WindowPreset[] = [
	{ key: "1h", label: "Last hour", ms: HOUR },
	{ key: "24h", label: "Last 24 hours", ms: DAY },
	{ key: "3d", label: "Last 3 days", ms: 3 * DAY },
	{ key: "7d", label: "Last 7 days", ms: 7 * DAY },
	{ key: "31d", label: "Last 31 days", ms: MAX_WINDOW_MS },
];

/** The picker's state: a preset, or a custom whole-day range (YYYY-MM-DD) when from/to are set. */
export interface WindowSelection {
	preset: string;
	from: string;
	to: string;
}

/** Unknown or empty presets mean the default. */
export function normalizePreset(preset: string | undefined): string {
	return WINDOW_PRESETS.some((p) => p.key === preset) ? (preset as string) : DEFAULT_WINDOW_KEY;
}

export function isCustomWindow(sel: Pick<WindowSelection, "from" | "to">): boolean {
	return !!sel.from || !!sel.to;
}

/** Choosing a preset drops any custom dates. */
export function selectPreset(preset: string): WindowSelection {
	return { preset: normalizePreset(preset), from: "", to: "" };
}

/** Choosing custom dates drops the preset (back to the default key, which is then unused). */
export function selectCustom(from: string, to: string): WindowSelection {
	return { preset: DEFAULT_WINDOW_KEY, from, to };
}

function parseDay(v: string): Date | null {
	if (!v) return null;
	const d = new Date(`${v}T00:00:00`);
	return Number.isNaN(d.getTime()) ? null : d;
}

export interface WindowBounds {
	since?: string;
	until?: string;
}

/**
 * since / until to send. Custom dates are whole days in the viewer's zone
 * (since = start of "from", until = end of "to"); a preset is relative to
 * `now`, except the default, which sends nothing so the server's own default
 * (and its "defaulted" echo) applies. `untilCap` (the Stuck lens: nothing
 * newer than 5 minutes) only ever lowers `until`.
 */
export function windowBounds(sel: WindowSelection, now: Date, untilCap?: Date): WindowBounds {
	const out: WindowBounds = {};
	const from = parseDay(sel.from);
	const to = parseDay(sel.to);
	if (from || to) {
		if (from) out.since = from.toISOString();
		if (to) out.until = new Date(to.getFullYear(), to.getMonth(), to.getDate() + 1).toISOString();
	} else {
		const preset = WINDOW_PRESETS.find((p) => p.key === normalizePreset(sel.preset))!;
		if (preset.key !== DEFAULT_WINDOW_KEY) out.since = new Date(now.getTime() - preset.ms).toISOString();
	}
	if (untilCap && (!out.until || untilCap.getTime() < new Date(out.until).getTime())) {
		out.until = untilCap.toISOString();
	}
	return out;
}

/** A custom range wider than the server's cap, caught before the request. */
export function customRangeTooLarge(sel: Pick<WindowSelection, "from" | "to">): boolean {
	const from = parseDay(sel.from);
	const to = parseDay(sel.to);
	if (!from || !to) return false;
	// Whole calendar days (rounded: a DST change makes a day 23 or 25 hours).
	return Math.round((to.getTime() + DAY - from.getTime()) / DAY) > MAX_WINDOW_MS / DAY;
}

function humanSpan(ms: number): string {
	const hours = Math.round(ms / HOUR);
	if (ms < 90 * 60_000) {
		const mins = Math.max(1, Math.round(ms / 60_000));
		return mins === 60 ? "hour" : `${mins} minutes`;
	}
	if (hours < 72 || hours % 24 !== 0) return hours === 1 ? "hour" : `${hours} hours`;
	return `${hours / 24} days`;
}

export interface WindowEcho {
	from: string;
	to: string;
	defaulted: boolean;
}

/**
 * The line above the table: "Showing the last 24 hours" when the window ends
 * about now, otherwise the dates. `defaulted` says the server chose it
 * because none was given.
 */
export function describeWindow(
	w: WindowEcho,
	now: Date,
	fmt: (d: Date) => string = (d) => d.toLocaleString(),
): { text: string; defaulted: boolean } {
	const from = new Date(w.from);
	const to = new Date(w.to);
	const span = to.getTime() - from.getTime();
	const endsNow = Math.abs(now.getTime() - to.getTime()) < 5 * 60_000;
	const text = endsNow ? `Showing the last ${humanSpan(span)}` : `Showing ${fmt(from)} to ${fmt(to)}`;
	return { text, defaulted: w.defaulted };
}
