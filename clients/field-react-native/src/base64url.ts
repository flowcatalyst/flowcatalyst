const ALPHABET = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_";
const LOOKUP: Record<string, number> = {};
for (let i = 0; i < ALPHABET.length; i++) LOOKUP[ALPHABET.charAt(i)] = i;

export function toBase64Url(bytes: Uint8Array): string {
	let out = "";
	for (let i = 0; i < bytes.length; i += 3) {
		const b0 = bytes[i] ?? 0;
		const b1 = bytes[i + 1] ?? 0;
		const b2 = bytes[i + 2] ?? 0;
		out += ALPHABET.charAt(b0 >> 2) + ALPHABET.charAt(((b0 & 3) << 4) | (b1 >> 4));
		if (i + 1 < bytes.length) out += ALPHABET.charAt(((b1 & 15) << 2) | (b2 >> 6));
		if (i + 2 < bytes.length) out += ALPHABET.charAt(b2 & 63);
	}
	return out;
}

export function fromBase64Url(s: string): Uint8Array {
	const clean = s.replace(/=+$/, "");
	const out = new Uint8Array(Math.floor((clean.length * 6) / 8));
	let acc = 0;
	let bits = 0;
	let o = 0;
	for (let i = 0; i < clean.length; i++) {
		const v = LOOKUP[clean.charAt(i)];
		if (v === undefined) throw new Error("invalid base64url");
		acc = (acc << 6) | v;
		bits += 6;
		if (bits >= 8) {
			bits -= 8;
			out[o++] = (acc >> bits) & 0xff;
		}
	}
	return out;
}
