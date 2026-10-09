// Return-to for the server-rendered UI (/ui). When /ui finds no session it
// sends the browser to /auth/login?redirect=<the /ui page>; after signing in
// the SPA goes back there. Only a same-origin path under /ui is accepted, so
// the parameter can never send the user to another site (no open redirect)
// or to an arbitrary SPA route.

// safeUiRedirect returns the path to navigate to for a ?redirect= value, or
// null when it is missing or not a same-origin /ui path. The value is parsed
// as a URL against the page's origin (which normalises "..", backslashes and
// percent-encoded dots) and then checked: same origin, path "/ui" or "/ui/...".
export function safeUiRedirect(
	raw: string | null,
	origin: string,
): string | null {
	if (!raw || !raw.startsWith("/")) return null;
	let url: URL;
	try {
		url = new URL(raw, origin);
	} catch {
		return null;
	}
	if (url.origin !== origin) return null;
	if (url.pathname !== "/ui" && !url.pathname.startsWith("/ui/")) return null;
	return url.pathname + url.search + url.hash;
}
