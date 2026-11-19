/**
 * The checks every verifier of a signed token runs (docs/wire-contract.md,
 * "Token spelling"): the issued value spelled other ways that a lenient
 * decoder reads as the same bytes, and the issued value changed in ways that
 * certainly alter its content or its signature bytes. Each case is checked to
 * differ from the issued value, and each re-spelling to decode to the issued
 * signature, so no case passes by chance.
 */

export const ALPHABET = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_";

const lenient = (s: string): Buffer => Buffer.from(s.replace(/=+$/, ""), "base64url");
const enc = (b: Buffer): string => b.toString("base64url");

/** Spare low bits of the last character of an unpadded base64url string of n characters. */
export function spareBits(n: number): number {
	return n % 4 === 2 ? 4 : n % 4 === 3 ? 2 : 0;
}

/** Every other last character of `seg` with the same significant bits. */
export function nonCanonicalSpellings(seg: string): string[] {
	const mask = (1 << spareBits(seg.length)) - 1;
	const last = ALPHABET.indexOf(seg[seg.length - 1]!);
	const out: string[] = [];
	for (let low = 1; low <= mask; low++) {
		out.push(seg.slice(0, -1) + ALPHABET[(last & ~mask) | low]);
	}
	return out;
}

/**
 * The issued compact JWS re-spelled: every other last character of the
 * signature, padded ('==' and, where two belong, '='), the standard alphabet,
 * LF / CR / CRLF inside the signature and, when `surroundingWhitespace`, a
 * space before or after it and CRLF after it.
 */
export function respellings(issued: string, surroundingWhitespace = true): Map<string, string> {
	const [h, p, sig] = issued.split(".") as [string, string, string];
	const head = `${h}.${p}.`;
	const bytes = lenient(sig);
	const out = new Map<string, string>();
	for (const v of nonCanonicalSpellings(sig)) {
		if (!lenient(v).equals(bytes)) throw new Error(`test bug: ${v} is not another spelling`);
		out.set(`signature ends in ${v[v.length - 1]}`, head + v);
	}
	const pad = "=".repeat((4 - (sig.length % 4)) % 4);
	if (pad) out.set("signature padded", head + sig + pad);
	if (pad.length === 2) out.set("signature padded once", `${head + sig}=`);
	const std = sig.replace(/-/g, "+").replace(/_/g, "/");
	if (std !== sig) out.set("signature in the standard alphabet", head + std);
	const mid = Math.floor(sig.length / 2);
	out.set("line feed inside the signature", `${head + sig.slice(0, mid)}\n${sig.slice(mid)}`);
	out.set("carriage return inside the signature", `${head + sig.slice(0, mid)}\r${sig.slice(mid)}`);
	out.set("CRLF inside the signature", `${head + sig.slice(0, mid)}\r\n${sig.slice(mid)}`);
	if (surroundingWhitespace) {
		out.set("leading space", ` ${issued}`);
		out.set("trailing space", `${issued} `);
		out.set("CRLF after it", `${issued}\r\n`);
	}
	return out;
}

/** The issued compact JWS changed in content. */
export function tampered(issued: string, claim: string, otherClaim: string): Map<string, string> {
	const [h, p, s] = issued.split(".") as [string, string, string];
	const sig = lenient(s);
	const payload = lenient(p).toString("utf8");
	if (!payload.includes(claim) || claim === otherClaim) {
		throw new Error(`test bug: ${claim} must occur in the payload and differ from ${otherClaim}`);
	}
	const first = Buffer.from(sig);
	first[0]! ^= 0x80;
	const last = Buffer.from(sig);
	last[last.length - 1]! ^= 0x01;
	const swapped = p[0] === "A" ? "B" : "A";
	return new Map([
		["first signature bit flipped", `${h}.${p}.${enc(first)}`],
		["last signature bit flipped", `${h}.${p}.${enc(last)}`],
		["payload claim changed", `${h}.${enc(Buffer.from(payload.replace(claim, otherClaim)))}.${s}`],
		["payload character changed", `${h}.${swapped}${p.slice(1)}.${s}`],
		["signature truncated by one byte", `${h}.${p}.${enc(sig.subarray(0, sig.length - 1))}`],
		["truncated by one character", issued.slice(0, -1)],
		["signature removed", `${h}.${p}.`],
		["signature segment removed", `${h}.${p}`],
	]);
}
