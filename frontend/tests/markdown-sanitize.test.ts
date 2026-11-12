// @vitest-environment jsdom
/**
 * The Documentation page renders tenant-authored Markdown for administrators.
 * The sanitiser must keep documentation content and drop everything that can
 * submit a form, restyle the page or run script.
 */

import { describe, expect, it } from "vitest";
import { renderDocMarkdown } from "@/utils/markdown-sanitize";

function dom(markdown: string): { html: string; root: HTMLElement } {
	const html = renderDocMarkdown(markdown);
	const root = document.createElement("div");
	root.innerHTML = html;
	return { html, root };
}

const FORM = `<form action="/ui/users/x/roles" method="post"><input type="hidden" name="roles" value="platform:super-admin"><button>Go</button></form>`;

describe("renderDocMarkdown: removed", () => {
	const cases: Array<[string, string, (r: HTMLElement, html: string) => void]> = [
		[
			"form with hidden input and button",
			FORM,
			(r, html) => {
				for (const sel of ["form", "input", "button"]) expect(r.querySelector(sel)).toBeNull();
				expect(html).not.toMatch(/action|method|platform:super-admin|formaction/i);
			},
		],
		[
			"style element",
			"<style>button{position:fixed;inset:0}</style>\n\ntext",
			(r, html) => {
				expect(r.querySelector("style")).toBeNull();
				expect(html).not.toContain("position:fixed");
			},
		],
		[
			"style attribute",
			`<div style="position:fixed">x</div>`,
			(r) => {
				expect(r.querySelector("div")).not.toBeNull();
				expect(r.querySelector("[style]")).toBeNull();
			},
		],
		[
			"button formaction",
			`<button formaction="/x">b</button>`,
			(r, html) => {
				expect(r.querySelector("button")).toBeNull();
				expect(html).not.toContain("formaction");
			},
		],
		[
			"select/option/textarea",
			`<select><option>a</option></select><textarea>t</textarea>`,
			(r) => {
				for (const sel of ["select", "option", "textarea"]) expect(r.querySelector(sel)).toBeNull();
			},
		],
		[
			"form attribute on a surviving element",
			`<a form="f" href="https://example.com/">l</a>`,
			(r) => expect(r.querySelector("[form]")).toBeNull(),
		],
		[
			"text input",
			`<input type="text" name="n" value="v">`,
			(r) => expect(r.querySelector("input")).toBeNull(),
		],
		[
			"onerror",
			`<img src=x onerror=alert(1)>`,
			(_r, html) => expect(html).not.toContain("onerror"),
		],
		[
			"javascript: href",
			`<a href="javascript:alert(1)">x</a>`,
			(_r, html) => expect(html).not.toContain("javascript:"),
		],
	];
	it.each(cases)("%s", (_name, input, assert) => {
		const { html, root } = dom(input);
		assert(root, html);
	});
});

describe("renderDocMarkdown: kept", () => {
	it("task list renders a disabled checkbox and nothing else on it", () => {
		const { root } = dom("- [x] done\n- [ ] todo");
		const boxes = root.querySelectorAll("input");
		expect(boxes).toHaveLength(2);
		for (const b of Array.from(boxes)) {
			expect(b.getAttribute("type")).toBe("checkbox");
			expect(b.hasAttribute("disabled")).toBe(true);
			expect(b.getAttributeNames().sort()).toEqual(
				b.hasAttribute("checked") ? ["checked", "disabled", "type"] : ["disabled", "type"],
			);
		}
		expect(boxes[0].hasAttribute("checked")).toBe(true);
	});

	it("a checkbox written as raw HTML is forced disabled and loses extra attributes", () => {
		const { root } = dom(`<input type="checkbox" name="roles" value="x" form="f" formaction="/x">`);
		const b = root.querySelector("input")!;
		expect(b.getAttributeNames().sort()).toEqual(["disabled", "type"]);
	});

	it("mermaid fence keeps pre code.language-mermaid", () => {
		const { root } = dom("```mermaid\ngraph TD; A-->B\n```");
		expect(root.querySelector("pre code.language-mermaid")?.textContent).toContain("A-->B");
	});

	it("table, link and image survive", () => {
		const { root } = dom(
			"| a | b |\n|---|---|\n| 1 | 2 |\n\n[l](https://example.com/p) ![i](https://example.com/i.png)",
		);
		expect(root.querySelector("table td")?.textContent).toBe("1");
		expect(root.querySelector("a")?.getAttribute("href")).toBe("https://example.com/p");
		expect(root.querySelector("img")?.getAttribute("src")).toBe("https://example.com/i.png");
	});

	it("headings, lists and blockquotes survive", () => {
		const { root } = dom("# H\n\n> q\n\n1. a\n2. b");
		expect(root.querySelector("h1")?.textContent).toBe("H");
		expect(root.querySelector("blockquote")).not.toBeNull();
		expect(root.querySelectorAll("ol li")).toHaveLength(2);
	});
});
