import { b64uEncode, randomId, utf8 } from "./encoding.js";
import { FieldProviderError } from "./errors.js";
import type { KeyProvider, PublicEcJwk } from "./types.js";

export interface ProofInput {
	htm: string;
	/** The request URL without query or fragment. */
	htu: string;
	nonce?: string | undefined;
	/** The access token the proof is bound to (adds `ath`). */
	accessToken?: string | undefined;
	/** Seconds since the epoch. */
	iat: number;
}

function jsonB64u(v: unknown): string {
	return b64uEncode(utf8(JSON.stringify(v)));
}

/** base64url(SHA-256(token)), the proof's `ath`. */
export async function accessTokenHash(token: string): Promise<string> {
	const d = await crypto.subtle.digest("SHA-256", utf8(token) as BufferSource);
	return b64uEncode(new Uint8Array(d));
}

/** Builds a DPoP proof (RFC 9449 shape, ES256) signed through the key provider. */
export async function buildProof(
	keys: KeyProvider,
	jwk: PublicEcJwk,
	p: ProofInput,
): Promise<string> {
	const header = {
		typ: "dpop+jwt",
		alg: "ES256",
		jwk: { kty: jwk.kty, crv: jwk.crv, x: jwk.x, y: jwk.y },
	};
	const claims: Record<string, unknown> = {
		htm: p.htm,
		htu: p.htu,
		iat: p.iat,
		jti: randomId(16),
	};
	if (p.nonce) claims["nonce"] = p.nonce;
	if (p.accessToken) claims["ath"] = await accessTokenHash(p.accessToken);
	const input = `${jsonB64u(header)}.${jsonB64u(claims)}`;
	const sig = await keys.sign(utf8(input));
	if (sig.length !== 64) {
		throw new FieldProviderError("INTERNAL", "the key provider returned a malformed signature");
	}
	return `${input}.${b64uEncode(sig)}`;
}

/** RFC 7638 thumbprint of an EC P-256 public key. */
export async function jwkThumbprint(jwk: PublicEcJwk): Promise<string> {
	const canonical = `{"crv":"${jwk.crv}","kty":"${jwk.kty}","x":"${jwk.x}","y":"${jwk.y}"}`;
	const d = await crypto.subtle.digest("SHA-256", utf8(canonical) as BufferSource);
	return b64uEncode(new Uint8Array(d));
}
