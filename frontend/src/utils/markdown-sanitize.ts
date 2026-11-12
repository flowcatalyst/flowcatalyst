/**
 * Markdown → safe HTML for the Documentation page.
 *
 * Documentation pages can be authored by tenant applications and are read by
 * administrators, so the HTML must not carry anything that can act on the
 * administrator's session by itself: DOMPurify's default allow-list keeps
 * <form>, <button>, <input>, <select>, <textarea> and <style>, which together
 * make a click-anywhere same-origin form POST. Those are removed here, along
 * with the attributes that restyle or retarget (style, action, formaction,
 * form, method). The one form control kept is the disabled checkbox that
 * GitHub-style task lists render.
 *
 * The sanitiser is a dedicated DOMPurify instance, so the hook below cannot
 * leak into other DOMPurify users in the bundle (Mermaid uses the shared one).
 */
import { marked } from "marked";
import createDOMPurify from "dompurify";

const FORBID_TAGS = ["style", "form", "button", "select", "option", "optgroup", "textarea"];
const FORBID_ATTR = ["style", "action", "formaction", "form", "method"];
const INPUT_KEEP = new Set(["type", "checked"]);

let purifier: ReturnType<typeof createDOMPurify> | null = null;

function getPurifier() {
	if (purifier) return purifier;
	const p = createDOMPurify(window);
	// An <input> survives only as a disabled checkbox with nothing but
	// type/checked on it.
	p.addHook("afterSanitizeAttributes", (node) => {
		if (node.nodeName !== "INPUT") return;
		const input = node as HTMLInputElement;
		if ((input.getAttribute("type") ?? "").toLowerCase() !== "checkbox") {
			input.remove();
			return;
		}
		for (const name of input.getAttributeNames()) {
			if (!INPUT_KEEP.has(name)) input.removeAttribute(name);
		}
		input.setAttribute("disabled", "");
	});
	purifier = p;
	return p;
}

/** Sanitise an HTML string for display in the Documentation page. */
export function sanitizeDocHtml(html: string): string {
	return getPurifier().sanitize(html, {
		USE_PROFILES: { html: true },
		FORBID_TAGS,
		FORBID_ATTR,
	});
}

/** Render Markdown and sanitise the result. */
export function renderDocMarkdown(markdown: string): string {
	return sanitizeDocHtml(marked.parse(markdown, { async: false }) as string);
}
