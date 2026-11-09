/**
 * Push tickets: short-lived grants to listen on topics, minted by the
 * application after its own access check (push contract section 6.4,
 * amendment 17.16; docs/push-gateway-spec.md section 9).
 *
 * A ticket is a JWS compact token, `alg` `EdDSA` (Ed25519), header `kid`.
 * The gateway verifies it with the public key the application synced
 * (`TicketMinter.jwks()` through `client.definitions().syncPush(...)`).
 *
 * ```ts
 * const minter = TicketMinter.fromConfig("shop", process.env.PUSH_TICKET_KEY!);
 * const ticket = minter.mint({ client: "acme", topics: ["shop:orders:{id}"], sub: "prn_1" });
 * ```
 */

import {
	createPrivateKey,
	createPublicKey,
	sign as edSign,
	type KeyObject,
} from "node:crypto";

/** The audience every ticket carries. */
export const TICKET_AUDIENCE = "fw-push";
/** A ticket's `exp` is at most this many seconds after its `iat`. */
export const MAX_TICKET_TTL_SECONDS = 300;
/** The default lifetime of a minted ticket. */
export const DEFAULT_TICKET_TTL_SECONDS = 60;
/** A ticket names at most this many topic patterns. */
export const MAX_TICKET_TOPICS = 100;

/** An Ed25519 public key as a JWK, the shape the gateway stores and `/push/keys` serves. */
export interface TicketPublicJwk {
	kty: "OKP";
	crv: "Ed25519";
	x: string;
	kid: string;
	use: "sig";
	alg: "EdDSA";
}

/** A JWKS of ticket keys: what sync sends and `GET /push/keys` answers. */
export interface TicketJwks {
	keys: TicketPublicJwk[];
}

export class TicketError extends Error {
	constructor(message: string) {
		super(message);
		this.name = "TicketError";
	}
}

const b64u = (b: Uint8Array | Buffer): string =>
	Buffer.from(b).toString("base64url");

// PKCS#8 prefix of a raw Ed25519 seed (RFC 8410).
const PKCS8_ED25519_PREFIX = Buffer.from("302e020100300506032b657004220420", "hex");

/**
 * An Ed25519 ticket signing key: `kid` plus the 32-byte seed. Configure it as
 * `kid:base64url-seed` ({@link TicketKey.parse}); the seed is a secret, the
 * derived public JWK is what the gateway needs.
 */
export class TicketKey {
	readonly kid: string;
	private readonly privateKey: KeyObject;
	private readonly publicJwk: TicketPublicJwk;

	private constructor(kid: string, privateKey: KeyObject, publicJwk: TicketPublicJwk) {
		this.kid = kid;
		this.privateKey = privateKey;
		this.publicJwk = publicJwk;
	}

	/** From a `kid:base64url-seed` string (the seed is the 32 raw bytes, base64url without padding). */
	static parse(config: string): TicketKey {
		const i = config.indexOf(":");
		if (i <= 0 || i === config.length - 1) {
			throw new TicketError("a ticket key is kid:base64url-seed");
		}
		return TicketKey.fromSeed(config.slice(0, i).trim(), config.slice(i + 1).trim());
	}

	static fromSeed(kid: string, seedBase64Url: string): TicketKey {
		if (kid === "" || kid.length > 64) {
			throw new TicketError("a ticket key id is 1 to 64 characters");
		}
		const seed = Buffer.from(seedBase64Url, "base64url");
		if (seed.length !== 32 || b64u(seed) !== seedBase64Url.replace(/=+$/, "")) {
			throw new TicketError("a ticket key seed is 32 bytes, base64url");
		}
		const privateKey = createPrivateKey({
			key: Buffer.concat([PKCS8_ED25519_PREFIX, seed]),
			format: "der",
			type: "pkcs8",
		});
		const x = createPublicKey(privateKey).export({ format: "jwk" }).x;
		if (!x) throw new TicketError("could not derive the public key");
		return new TicketKey(kid, privateKey, {
			kty: "OKP",
			crv: "Ed25519",
			x,
			kid,
			use: "sig",
			alg: "EdDSA",
		});
	}

	/** The public key as a JWK (no private member). */
	toPublicJwk(): TicketPublicJwk {
		return { ...this.publicJwk };
	}

	/** @internal Ed25519 signature (64 bytes) of `data`. */
	signBytes(data: Buffer): Buffer {
		return edSign(null, data, this.privateKey);
	}
}

/** What a ticket grants. */
export interface MintTicketOptions {
	/** The stream's client (the platform client's value, `_platform`, for platform-wide). */
	client: string;
	/** Topic patterns of the application (placeholders `{x}` match one segment), at most 100. */
	topics: string[];
	/** The principal, recorded in the stream token for logs. */
	sub?: string;
	/** Bind the ticket to a DPoP key: the subscribe must prove it (`cnf.jkt`). */
	jkt?: string;
	/** Lifetime in seconds: 1 to 300 (default 60). The time to subscribe with it. */
	ttlSeconds?: number;
	/** The latest the stream may live, as a Date or epoch seconds. Default: `exp` + the gateway's max lifetime. */
	streamExp?: Date | number;
	/** Issued-at (default now), as a Date or epoch seconds. For tests. */
	now?: Date | number;
}

const epochSeconds = (v: Date | number): number =>
	v instanceof Date ? Math.floor(v.getTime() / 1000) : Math.floor(v);

/**
 * Mints tickets for one application with its current key, and exports the
 * JWKS (the current key then the previous ones during a rotation) that sync
 * sends to the gateway.
 */
export class TicketMinter {
	readonly applicationCode: string;
	private readonly current: TicketKey;
	private readonly previous: Array<TicketKey | TicketPublicJwk>;

	/**
	 * @param applicationCode the `iss` of every ticket; each topic's first segment must equal it.
	 * @param current the key tickets are signed with.
	 * @param previous the keys of a rotation in progress: published in the JWKS so
	 *   tickets signed with them keep verifying, never used to sign. A public JWK is enough.
	 */
	constructor(
		applicationCode: string,
		current: TicketKey,
		previous: Array<TicketKey | TicketPublicJwk> = [],
	) {
		if (!/^[a-z][a-z0-9-]*$/.test(applicationCode)) {
			throw new TicketError(`not an application code: "${applicationCode}"`);
		}
		if (previous.length > 3) {
			throw new TicketError("at most 4 ticket keys (the current and 3 previous)");
		}
		this.applicationCode = applicationCode;
		this.current = current;
		this.previous = previous;
	}

	/**
	 * From configuration: the current key as `kid:base64url-seed`, and the
	 * previous ones (during a rotation) as `kid:base64url-seed` strings, for
	 * example split from a comma-separated setting.
	 */
	static fromConfig(
		applicationCode: string,
		current: string,
		previous: string[] = [],
	): TicketMinter {
		return new TicketMinter(
			applicationCode,
			TicketKey.parse(current),
			previous.filter((p) => p.trim() !== "").map((p) => TicketKey.parse(p)),
		);
	}

	/** The JWKS to sync: the current key, then the previous ones. */
	jwks(): TicketJwks {
		const keys = [this.current.toPublicJwk()];
		for (const p of this.previous) {
			keys.push(p instanceof TicketKey ? p.toPublicJwk() : { ...p });
		}
		const kids = new Set(keys.map((k) => k.kid));
		if (kids.size !== keys.length) throw new TicketError("duplicate ticket key ids");
		return { keys };
	}

	/**
	 * Mint a ticket. Claims, in order: `iss`, `aud`, `client`, `topics`, `sub`,
	 * `cnf`, `iat`, `exp`, `stream-exp`.
	 *
	 * @throws {TicketError} when the request cannot be a valid ticket.
	 */
	mint(options: MintTicketOptions): string {
		const { client, topics } = options;
		if (!/^[A-Za-z0-9._-]{1,100}$/.test(client)) {
			throw new TicketError("client is [A-Za-z0-9._-]{1,100}");
		}
		if (topics.length === 0 || topics.length > MAX_TICKET_TOPICS) {
			throw new TicketError(`a ticket names 1 to ${MAX_TICKET_TOPICS} topics`);
		}
		for (const t of topics) {
			const first = t.split(":")[0];
			if (first !== this.applicationCode || !t.includes(":")) {
				throw new TicketError(
					`the topic "${t}" is not a topic of the application "${this.applicationCode}"`,
				);
			}
		}
		const ttl = options.ttlSeconds ?? DEFAULT_TICKET_TTL_SECONDS;
		if (!Number.isInteger(ttl) || ttl < 1 || ttl > MAX_TICKET_TTL_SECONDS) {
			throw new TicketError(`ttlSeconds is 1 to ${MAX_TICKET_TTL_SECONDS}`);
		}
		const iat = epochSeconds(options.now ?? new Date());
		const exp = iat + ttl;

		const claims: Record<string, unknown> = {
			iss: this.applicationCode,
			aud: TICKET_AUDIENCE,
			client,
			topics,
		};
		if (options.sub !== undefined && options.sub !== "") claims["sub"] = options.sub;
		if (options.jkt !== undefined && options.jkt !== "") claims["cnf"] = { jkt: options.jkt };
		claims["iat"] = iat;
		claims["exp"] = exp;
		if (options.streamExp !== undefined) claims["stream-exp"] = epochSeconds(options.streamExp);

		const header = b64u(Buffer.from(JSON.stringify({ alg: "EdDSA", kid: this.current.kid })));
		const body = b64u(Buffer.from(JSON.stringify(claims)));
		const signingInput = `${header}.${body}`;
		const sig = this.current.signBytes(Buffer.from(signingInput, "ascii"));
		return `${signingInput}.${b64u(sig)}`;
	}
}
