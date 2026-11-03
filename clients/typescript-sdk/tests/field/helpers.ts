/** Test helpers: mint field access tokens (RS256) and DPoP proofs (ES256). */
import { createHash, randomUUID } from "node:crypto";
import {
	SignJWT,
	exportJWK,
	generateKeyPair,
	createLocalJWKSet,
	type JWK,
	type JWTVerifyGetKey,
	type KeyLike,
} from "jose";

export const ISSUER = "https://fc.example.test";
export const APP = "test-app";

export interface Platform {
	privateKey: KeyLike;
	keys: JWTVerifyGetKey;
}

export async function newPlatform(): Promise<Platform> {
	const { publicKey, privateKey } = await generateKeyPair("RS256", {
		modulusLength: 2048,
		extractable: true,
	});
	const jwk: JWK = { ...(await exportJWK(publicKey)), kid: "k1", alg: "RS256" };
	return {
		privateKey: privateKey as KeyLike,
		keys: createLocalJWKSet({ keys: [jwk] }),
	};
}

export interface Device {
	privateKey: KeyLike;
	publicJwk: JWK;
	jkt: string;
}

export async function newDevice(): Promise<Device> {
	const { publicKey, privateKey } = await generateKeyPair("ES256", {
		extractable: true,
	});
	const publicJwk = await exportJWK(publicKey);
	const canonical = `{"crv":"${publicJwk.crv}","kty":"EC","x":"${publicJwk.x}","y":"${publicJwk.y}"}`;
	return {
		privateKey: privateKey as KeyLike,
		publicJwk,
		jkt: createHash("sha256").update(canonical).digest("base64url"),
	};
}

export async function mintToken(
	platform: Platform,
	device: Device,
	over: Record<string, unknown> = {},
	header: Record<string, unknown> = {},
): Promise<string> {
	const now = Math.floor(Date.now() / 1000);
	const claims = {
		iss: ISSUER,
		sub: "fwu_1",
		aud: APP,
		exp: now + 600,
		iat: now,
		nbf: now,
		jti: randomUUID(),
		token_use: "field",
		client_id: "clt_1",
		client_identifier: "acme",
		worker_code: "W001",
		name: "Wendy",
		device_id: "fwd_1",
		device_mode: "PERSONAL",
		sid: "fws_1",
		amr: ["pin"],
		auth_time: now,
		cnf: { jkt: device.jkt },
		...over,
	};
	return new SignJWT(claims)
		.setProtectedHeader({
			alg: "RS256",
			kid: "k1",
			typ: "fc-field+jwt",
			...header,
		})
		.sign(platform.privateKey);
}

export async function mintProof(
	device: Device,
	token: string,
	method: string,
	url: string,
	over: Record<string, unknown> = {},
): Promise<string> {
	return new SignJWT({
		htm: method,
		htu: url,
		ath: createHash("sha256").update(token).digest("base64url"),
		jti: randomUUID(),
		iat: Math.floor(Date.now() / 1000),
		...over,
	})
		.setProtectedHeader({ alg: "ES256", typ: "dpop+jwt", jwk: device.publicJwk })
		.sign(device.privateKey);
}
