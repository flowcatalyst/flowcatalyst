import { strict as assert } from "node:assert";
import { before, describe, it } from "node:test";
import { Window } from "happy-dom";

type Pad = HTMLElement & { clear(): void };
let doc: Document;
let win: InstanceType<typeof Window>;

before(async () => {
	win = new Window();
	const g = globalThis as Record<string, unknown>;
	for (const k of ["HTMLElement", "customElements", "document", "CustomEvent", "KeyboardEvent", "Event"]) {
		Object.defineProperty(g, k, { value: (win as unknown as Record<string, unknown>)[k], configurable: true, writable: true });
	}
	const { definePinPad } = await import("../../src/pin-pad.js");
	assert.equal(definePinPad(), true);
	doc = win.document as unknown as Document;
});

function mount(attrs: Record<string, string> = {}): Pad {
	const el = doc.createElement("fc-pin-pad") as Pad;
	for (const [k, v] of Object.entries(attrs)) el.setAttribute(k, v);
	doc.body.append(el);
	return el;
}

function keys(el: Pad): HTMLButtonElement[] {
	return [...(el.shadowRoot?.querySelectorAll("button") ?? [])] as HTMLButtonElement[];
}

function press(el: Pad, label: string): void {
	const b = keys(el).find((x) => x.getAttribute("aria-label") === label);
	assert.ok(b, `key ${label}`);
	b.click();
}

describe("<fc-pin-pad>", () => {
	it("renders a 3x4 keypad with dots, in a shadow root", () => {
		const el = mount();
		assert.equal(keys(el).length, 12);
		assert.equal(el.shadowRoot?.querySelectorAll(".dot").length, 6);
		assert.match(el.shadowRoot?.querySelector("style")?.textContent ?? "", /min-height: 64px/);
	});

	it("emits pin when length digits are in, then clears itself", () => {
		const el = mount({ length: "4" });
		const got: string[] = [];
		el.addEventListener("pin", (e) => got.push((e as CustomEvent<{ pin: string }>).detail.pin));
		for (const d of ["1", "2", "3"]) press(el, d);
		assert.deepEqual(got, []);
		assert.equal(el.shadowRoot?.querySelectorAll(".dot.on").length, 3);
		press(el, "4");
		assert.deepEqual(got, ["1234"]);
		assert.equal(el.shadowRoot?.querySelectorAll(".dot.on").length, 0);
	});

	it("Backspace, Clear and keyboard input work", () => {
		const el = mount({ length: "4" });
		const got: string[] = [];
		el.addEventListener("pin", (e) => got.push((e as CustomEvent<{ pin: string }>).detail.pin));
		const key = (k: string) => el.dispatchEvent(new (win.KeyboardEvent as unknown as typeof KeyboardEvent)("keydown", { key: k }));
		key("9");
		key("8");
		key("Backspace");
		assert.equal(el.shadowRoot?.querySelectorAll(".dot.on").length, 1);
		press(el, "Clear");
		assert.equal(el.shadowRoot?.querySelectorAll(".dot.on").length, 0);
		for (const k of ["5", "6", "7", "0"]) key(k);
		assert.deepEqual(got, ["5670"]);
		key("1");
		key("2");
		key("3");
		key("4");
		key("Enter");
		assert.deepEqual(got, ["5670", "1234"]);
	});

	it("never exposes the digits in attributes, DOM text or the accessible status", () => {
		const el = mount({ length: "6" });
		press(el, "7");
		press(el, "7");
		press(el, "3");
		const snapshot = (el.outerHTML + [...(el.shadowRoot?.querySelectorAll(".dot, [role=status], .label") ?? [])].map((n) => n.outerHTML).join(""));
		assert.ok(!/7{2}3/.test(snapshot));
		assert.equal(el.shadowRoot?.querySelector("[role=status]")?.textContent, "3 of 6 digits entered");
		assert.equal(el.getAttributeNames().sort().join(","), "length,tabindex");
	});

	it("disabled ignores input; label shows", () => {
		const el = mount({ length: "4", disabled: "", label: "Enter PIN" });
		const got: string[] = [];
		el.addEventListener("pin", (e) => got.push((e as CustomEvent<{ pin: string }>).detail.pin));
		assert.equal(el.shadowRoot?.querySelector(".label")?.textContent, "Enter PIN");
		assert.ok(keys(el).every((b) => b.disabled));
		el.dispatchEvent(new (win.KeyboardEvent as unknown as typeof KeyboardEvent)("keydown", { key: "1" }));
		assert.equal(el.shadowRoot?.querySelectorAll(".dot.on").length, 0);
		assert.deepEqual(got, []);
	});

	it("scrambled shuffles the digits and reshuffles after each entry", () => {
		const layouts = new Set<string>();
		for (let i = 0; i < 12; i++) {
			const el = mount({ scrambled: "", length: "4" });
			layouts.add(keys(el).filter((b) => /^\d$/.test(b.getAttribute("aria-label") ?? "")).map((b) => b.textContent).join(""));
			assert.equal([...layouts].at(-1)?.split("").sort().join(""), "0123456789");
		}
		assert.ok(layouts.size > 1, "layout varies between showings");
		const el = mount({ scrambled: "", length: "2" });
		const first = keys(el).map((b) => b.textContent).join("");
		const seen = new Set([first]);
		for (let i = 0; i < 10; i++) {
			const ds = keys(el).filter((b) => /^\d$/.test(b.textContent ?? ""));
			ds[0]?.click();
			ds[1]?.click();
			seen.add(keys(el).map((b) => b.textContent).join(""));
		}
		assert.ok(seen.size > 1, "reshuffles after the PIN is emitted");
	});

	it("clear() wipes the entry", () => {
		const el = mount({ length: "6" });
		press(el, "1");
		el.clear();
		assert.equal(el.shadowRoot?.querySelectorAll(".dot.on").length, 0);
	});
});
