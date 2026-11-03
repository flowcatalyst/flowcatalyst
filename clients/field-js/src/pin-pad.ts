// <fc-pin-pad>: a framework-free PIN keypad. It never writes the PIN to an
// attribute, the DOM text, storage or the console; the digits live in one
// private array until `pin` is emitted, then are wiped.

const STYLE = `
:host { display: inline-block; font-family: var(--fc-pin-font, system-ui, sans-serif);
  color: var(--fc-pin-fg, #111); user-select: none; -webkit-user-select: none; touch-action: manipulation; }
:host([hidden]) { display: none; }
.wrap { display: flex; flex-direction: column; align-items: center; gap: 16px;
  background: var(--fc-pin-bg, transparent); padding: 8px; }
.label { font-size: 1.1rem; min-height: 1.4em; text-align: center; }
.dots { display: flex; gap: 12px; min-height: 20px; }
.dot { width: 18px; height: 18px; border-radius: 50%; box-sizing: border-box;
  border: 2px solid var(--fc-pin-dot, currentColor); background: transparent; }
.dot.on { background: var(--fc-pin-dot, currentColor); }
.keys { display: grid; grid-template-columns: repeat(3, minmax(64px, 1fr)); gap: 12px; }
button { min-width: 64px; min-height: 64px; font: inherit; font-size: 1.6rem; border-radius: 12px;
  border: 1px solid var(--fc-pin-key-border, #bbb); background: var(--fc-pin-key-bg, #f4f4f4);
  color: var(--fc-pin-key-fg, inherit); cursor: pointer; }
button:active { background: var(--fc-pin-key-active-bg, #ddd); }
button:focus-visible { outline: 3px solid var(--fc-pin-focus, #2563eb); outline-offset: 2px; }
button[disabled] { opacity: 0.45; cursor: not-allowed; }
.fn { font-size: 1.1rem; }
`;

const MIN_EARLY_ENTER = 4;

function shuffled(): string[] {
	const d = ["0", "1", "2", "3", "4", "5", "6", "7", "8", "9"];
	const r = new Uint32Array(d.length);
	crypto.getRandomValues(r);
	for (let i = d.length - 1; i > 0; i--) {
		const j = (r[i] ?? 0) % (i + 1);
		const t = d[i] as string;
		d[i] = d[j] as string;
		d[j] = t;
	}
	return d;
}

export interface FcPinPadElement extends HTMLElement {
	length: number;
	scrambled: boolean;
	disabled: boolean;
	label: string;
	clear(): void;
}

export interface PinEventDetail {
	pin: string;
}

/** Registers `<fc-pin-pad>` (or `name`); false when there is no DOM. */
export function definePinPad(name = "fc-pin-pad"): boolean {
	if (typeof customElements === "undefined" || typeof HTMLElement === "undefined") {
		return false;
	}
	if (customElements.get(name)) return true;

	class PinPad extends HTMLElement {
		static get observedAttributes(): string[] {
			return ["length", "scrambled", "disabled", "label"];
		}

		private digits: string[] = [];
		private layout: string[] = [];
		private dotsEl: HTMLElement | null = null;
		private labelEl: HTMLElement | null = null;
		private keysEl: HTMLElement | null = null;
		private statusEl: HTMLElement | null = null;
		private readonly onKey = (e: KeyboardEvent): void => this.keydown(e);

		constructor() {
			super();
			const root = this.attachShadow({ mode: "open" });
			const style = document.createElement("style");
			style.textContent = STYLE;
			const wrap = document.createElement("div");
			wrap.className = "wrap";
			this.labelEl = document.createElement("div");
			this.labelEl.className = "label";
			this.dotsEl = document.createElement("div");
			this.dotsEl.className = "dots";
			this.statusEl = document.createElement("div");
			this.statusEl.setAttribute("role", "status");
			this.statusEl.setAttribute("aria-live", "polite");
			this.statusEl.style.cssText =
				"position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0 0 0 0)";
			this.keysEl = document.createElement("div");
			this.keysEl.className = "keys";
			wrap.append(this.labelEl, this.dotsEl, this.keysEl, this.statusEl);
			root.append(style, wrap);
		}

		get length(): number {
			const n = Number.parseInt(this.getAttribute("length") ?? "", 10);
			return Number.isFinite(n) && n >= 1 && n <= 12 ? n : 6;
		}
		set length(n: number) {
			this.setAttribute("length", String(n));
		}
		get scrambled(): boolean {
			return this.hasAttribute("scrambled");
		}
		set scrambled(v: boolean) {
			this.toggleAttribute("scrambled", v);
		}
		get disabled(): boolean {
			return this.hasAttribute("disabled");
		}
		set disabled(v: boolean) {
			this.toggleAttribute("disabled", v);
		}
		get label(): string {
			return this.getAttribute("label") ?? "";
		}
		set label(v: string) {
			this.setAttribute("label", v);
		}

		connectedCallback(): void {
			if (!this.hasAttribute("tabindex")) this.setAttribute("tabindex", "0");
			this.addEventListener("keydown", this.onKey);
			this.reshuffle();
			this.render();
		}

		disconnectedCallback(): void {
			this.removeEventListener("keydown", this.onKey);
			this.digits = [];
		}

		attributeChangedCallback(name: string): void {
			if (name === "length" && this.digits.length > this.length) this.digits = [];
			if (name === "scrambled") this.reshuffle();
			this.render();
		}

		/** Wipes the entered digits (and reshuffles when scrambled). */
		clear(): void {
			this.digits = [];
			this.reshuffle();
			this.render();
		}

		private reshuffle(): void {
			this.layout = this.scrambled
				? shuffled()
				: ["1", "2", "3", "4", "5", "6", "7", "8", "9", "0"];
		}

		private press(d: string): void {
			if (this.disabled || this.digits.length >= this.length) return;
			this.digits.push(d);
			if (this.digits.length >= this.length) this.submit();
			else this.render();
		}

		private backspace(): void {
			if (this.disabled) return;
			this.digits.pop();
			this.render();
		}

		private submit(): void {
			const pin = this.digits.join("");
			this.digits = [];
			this.reshuffle();
			this.render();
			this.dispatchEvent(
				new CustomEvent<PinEventDetail>("pin", { detail: { pin }, bubbles: true, composed: true }),
			);
		}

		private keydown(e: KeyboardEvent): void {
			if (this.disabled) return;
			if (/^[0-9]$/.test(e.key)) {
				e.preventDefault();
				this.press(e.key);
			} else if (e.key === "Backspace") {
				e.preventDefault();
				this.backspace();
			} else if (e.key === "Enter") {
				e.preventDefault();
				if (this.digits.length >= Math.min(MIN_EARLY_ENTER, this.length)) this.submit();
			}
		}

		private button(text: string, aria: string, onClick: () => void, fn = false): HTMLButtonElement {
			const b = document.createElement("button");
			b.type = "button";
			b.textContent = text;
			b.setAttribute("aria-label", aria);
			if (fn) b.className = "fn";
			b.disabled = this.disabled;
			b.addEventListener("click", onClick);
			return b;
		}

		private render(): void {
			if (!this.dotsEl || !this.keysEl || !this.labelEl || !this.statusEl) return;
			this.labelEl.textContent = this.label;
			this.dotsEl.replaceChildren();
			for (let i = 0; i < this.length; i++) {
				const dot = document.createElement("span");
				dot.className = i < this.digits.length ? "dot on" : "dot";
				this.dotsEl.append(dot);
			}
			// Counts only: never the digits.
			this.statusEl.textContent = `${this.digits.length} of ${this.length} digits entered`;
			if (this.layout.length !== 10) this.reshuffle();
			const keys: HTMLButtonElement[] = [];
			for (const d of this.layout.slice(0, 9)) keys.push(this.button(d, d, () => this.press(d)));
			keys.push(this.button("C", "Clear", () => this.clear(), true));
			const last = this.layout[9] as string;
			keys.push(this.button(last, last, () => this.press(last)));
			keys.push(this.button("⌫", "Delete", () => this.backspace(), true));
			this.keysEl.replaceChildren(...keys);
		}
	}

	customElements.define(name, PinPad);
	return true;
}
