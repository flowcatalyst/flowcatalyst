/**
 * Collapsible sidebar sections: which are collapsed (persisted per viewer in
 * localStorage) and which must stay open because they hold the current page.
 * Every storage access is wrapped: without storage everything is expanded.
 */

export const NAV_CLOSED_KEY = "fc:nav-closed";

interface NavSectionLike {
	label: string;
	items: { route?: string; children?: { route?: string }[] }[];
}

/** Read the set of collapsed section labels; any failure means none. */
export function loadClosedSections(
	storage: Pick<Storage, "getItem"> | undefined = safeStorage(),
): Set<string> {
	try {
		const raw = storage?.getItem(NAV_CLOSED_KEY);
		if (!raw) return new Set();
		const parsed: unknown = JSON.parse(raw);
		if (!Array.isArray(parsed)) return new Set();
		return new Set(parsed.filter((v): v is string => typeof v === "string"));
	} catch {
		return new Set();
	}
}

/** Persist the collapsed set; failures (full, blocked) are ignored. */
export function saveClosedSections(
	closed: Set<string>,
	storage: Pick<Storage, "setItem"> | undefined = safeStorage(),
): void {
	try {
		storage?.setItem(NAV_CLOSED_KEY, JSON.stringify([...closed].sort()));
	} catch {
		// unavailable: the in-memory state still works for this page load
	}
}

function safeStorage(): Storage | undefined {
	try {
		return localStorage;
	} catch {
		return undefined;
	}
}

/** True when `path` is the route or beneath it. */
export function routeMatches(path: string, route: string | undefined): boolean {
	return !!route && (path === route || path.startsWith(route + "/"));
}

/** True when the section holds the page at `path`. */
export function sectionHasActive(section: NavSectionLike, path: string): boolean {
	return section.items.some(
		(it) =>
			routeMatches(path, it.route) ||
			(it.children ?? []).some((c) => routeMatches(path, c.route)),
	);
}

/** A section is shown open unless collapsed AND it does not hold the current page. */
export function isSectionOpen(
	section: NavSectionLike,
	closed: Set<string>,
	path: string,
): boolean {
	return !closed.has(section.label) || sectionHasActive(section, path);
}

/** Toggle a section; the active section cannot be collapsed. Returns a new set. */
export function toggleSection(
	section: NavSectionLike,
	closed: Set<string>,
	path: string,
): Set<string> {
	const next = new Set(closed);
	if (sectionHasActive(section, path)) return next;
	if (next.has(section.label)) next.delete(section.label);
	else next.add(section.label);
	return next;
}
