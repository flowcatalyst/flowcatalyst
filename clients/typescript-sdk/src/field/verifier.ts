/**
 * Field-worker token verification.
 *
 * A field worker signs in from an enrolled device with a PIN at the
 * platform's `/field/*`; the platform issues a short RS256 access token
 * (JOSE `typ: fc-field+jwt`, `token_use: "field"`, `aud` = app code) bound to
 * the device key (`cnf.jkt`). The app backend verifies, per request, the token
 * AND a DPoP proof (ES256 JWS) signed by the same device key:
 *
 *   Authorization: DPoP <token>
 *   DPoP: <proof>
 *
 * PINs never reach app backends; this module only verifies. Mirrors the Go
 * SDK's `auth.FieldTokenVerifier` (same checks, same refusal reasons). Nonce
 * checking is not done here; the proof's iat window is the replay bound.
 */

import {
	createHash,
	createPublicKey,
	verify as cryptoVerify,
	type KeyObject,
} from "node:crypto";
import {
	createRemoteJWKSet,
	decodeProtectedHeader,
	errors as joseErrors,
	jwtVerify,
	type JWTPayload,
	type JWTVerifyGetKey,
} from "jose";

/** Stable refusal reasons; the shared fixtures (api/field-fixtures) expect these strings. */
export const FieldReason = {
	Malformed: "malformed",
	WrongTyp: "wrong_typ",
	WrongTokenUse: "wrong_token_use",
	WrongAud: "wrong_aud",
	Expired: "expired",
	InvalidToken: "invalid_token",
	PCR: "pcr",
	MissingProof: "missing_proof",
	InvalidProof: "invalid_proof",
	JKTMismatch: "jkt_mismatch",
	HTMMismatch: "htm_mismatch",
	HTUMismatch: "htu_mismatch",
	ATHMismatch: "ath_mismatch",
	StaleProof: "stale_proof",
} as const;

export type FieldReasonCode = (typeof FieldReason)[keyof typeof FieldReason];

/** A refused field credential. `reason` is one of {@link FieldReason}. */
export class FieldError extends Error {
	readonly reason: FieldReasonCode;
	readonly detail: string;
	constructor(reason: FieldReasonCode, detail = "") {
		super(
			detail
				? `field credential refused: ${reason}: ${detail}`
				: `field credential refused: ${reason}`,
		);
		this.name = "FieldError";
		this.reason = reason;
		this.detail = detail;
	}
}

/**
 * Discovery or key fetch failed: not a verdict on the credential. Callers
 * should answer 503, not 401.
 */
export class FieldVerifierUnavailableError extends Error {
	constructor(message: string, options?: { cause?: unknown }) {
		super(message);
		this.name = "FieldVerifierUnavailableError";
		if (options?.cause !== undefined) {
			(this as { cause?: unknown }).cause = options.cause;
		}
	}
}

export interface FieldTokenVerifierConfig {
	/** FlowCatalyst base URL and the tokens' `iss` claim. */
	issuer: string;
	/** This application's code; tokens must be addressed to it (`aud`). */
	appCode: string;
	/** Skip OIDC discovery and fetch keys from this URL. */
	jwksUrl?: string;
	/**
	 * Supply the key resolver directly (e.g. `createLocalJWKSet`), bypassing
	 * discovery and fetching. Used by tests and offline fixtures.
	 */
	keys?: JWTVerifyGetKey;
	/** Transport for discovery (default: global `fetch`). */
	fetch?: typeof fetch;
	/** exp/nbf skew in seconds (default 60). */
	clockSkewSeconds?: number;
	/**
	 * How far a proof's iat may be from now, either way, in seconds
	 * (default 300). Widen it for devices with bad clocks.
	 */
	proofMaxAgeSeconds?: number;
	/** Override the clock (tests, fixtures). */
	now?: () => Date;
}

/**
 * Who is signed in as the worker (the RFC 8693 `act` claim): a supervisor
 * (`field_worker`) or a platform user (`principal`).
 */
export interface FieldActor {
	id: string;
	type: string;
	name: string;
}

/**
 * A verified field worker on a verified key. When `actor` is set, someone else
 * is signed in as the worker: every other field is still the WORKER's, and an
 * app's audit trail records the worker as the subject and `actor` as the
 * operator.
 */
export interface FieldPrincipal {
	workerId: string;
	clientId: string;
	clientIdentifier: string;
	workerCode: string;
	name: string;
	appCode: string;
	/** Empty when the key is not an enrolled device (`deviceMode` is then `UNENROLLED`). */
	deviceId: string;
	/** `PERSONAL`, `SHARED` or `UNENROLLED`. */
	deviceMode: string;
	/** The login family (`sid`). */
	sessionId: string;
	amr: string[];
	authTime: Date;
	expiresAt: Date;
	/** Thumbprint of the device key the request was proven with. */
	jkt: string;
	tokenId: string;
	/** Set when someone other than the worker is signed in as them. */
	actor: FieldActor | null;
	/** True for a test identity. */
	test: boolean;
}

const FIELD_TYP = "fc-field+jwt";

export class FieldTokenVerifier {
	private readonly cfg: Required<
		Pick<
			FieldTokenVerifierConfig,
			"appCode" | "clockSkewSeconds" | "proofMaxAgeSeconds"
		>
	> & { issuer: string; now: () => Date };
	private readonly fetchFn: typeof fetch;
	private readonly explicitJwksUrl: string | undefined;
	private keys: JWTVerifyGetKey | undefined;
	private pendingKeys: Promise<JWTVerifyGetKey> | undefined;

	constructor(config: FieldTokenVerifierConfig) {
		this.cfg = {
			issuer: config.issuer.replace(/\/+$/, ""),
			appCode: config.appCode,
			clockSkewSeconds: config.clockSkewSeconds ?? 60,
			proofMaxAgeSeconds: config.proofMaxAgeSeconds ?? 300,
			now: config.now ?? (() => new Date()),
		};
		this.fetchFn = config.fetch ?? fetch;
		this.explicitJwksUrl = config.jwksUrl;
		this.keys = config.keys;
	}

	private async resolveKeys(): Promise<JWTVerifyGetKey> {
		if (this.keys) return this.keys;
		if (!this.pendingKeys) {
			const p = (async () => {
				let url = this.explicitJwksUrl;
				if (!url) {
					const disco = `${this.cfg.issuer}/.well-known/openid-configuration`;
					try {
						const res = await this.fetchFn(disco, {
							headers: { Accept: "application/json" },
						});
						if (!res.ok) throw new Error(`HTTP ${res.status}`);
						const doc = (await res.json()) as { jwks_uri?: string };
						if (!doc.jwks_uri) throw new Error("no jwks_uri");
						url = doc.jwks_uri;
					} catch (e) {
						throw new FieldVerifierUnavailableError(
							`OIDC discovery failed (${disco}): ${e instanceof Error ? e.message : String(e)}`,
							{ cause: e },
						);
					}
				}
				// jose's remote set caches keys and refreshes on kid miss.
				return createRemoteJWKSet(new URL(url));
			})();
			// A failed discovery must not stay memoized.
			p.catch(() => {
				if (this.pendingKeys === p) this.pendingKeys = undefined;
			});
			this.pendingKeys = p;
		}
		this.keys = await this.pendingKeys;
		return this.keys;
	}

	/**
	 * Check a request's credentials: the `Authorization` header (`DPoP
	 * <token>`), the `DPoP` proof header, and the request's method and full
	 * URL (scheme, host and path; any query is ignored). Resolves with the
	 * verified principal, or rejects with a {@link FieldError} naming the
	 * reason. Discovery/key-fetch failures reject with
	 * {@link FieldVerifierUnavailableError}.
	 */
	async verify(
		authorizationHeader: string | undefined,
		dpopHeader: string | undefined,
		method: string,
		requestUrl: string,
	): Promise<FieldPrincipal> {
		const m = /^\s*(\S+)\s+(\S+)\s*$/.exec(authorizationHeader ?? "");
		if (!m || m[1]!.toLowerCase() !== "dpop") {
			throw new FieldError(
				FieldReason.Malformed,
				'Authorization must be "DPoP <token>"',
			);
		}
		const token = m[2]!;

		let header;
		try {
			header = decodeProtectedHeader(token);
		} catch {
			throw new FieldError(FieldReason.Malformed, "not a compact JWS");
		}
		if (token.split(".").length !== 3) {
			throw new FieldError(FieldReason.Malformed, "not a compact JWS");
		}
		if (header.typ !== FIELD_TYP) {
			throw new FieldError(FieldReason.WrongTyp, `typ is ${header.typ ?? ""}`);
		}
		// RS256 only; refuse HS256, none, ES256, ... before touching the keys.
		if (header.alg !== "RS256") {
			throw new FieldError(
				FieldReason.InvalidToken,
				`alg must be RS256, got ${header.alg}`,
			);
		}

		const keys = await this.resolveKeys();
		const now = this.cfg.now();
		let payload: JWTPayload;
		try {
			({ payload } = await jwtVerify(token, keys, {
				issuer: this.cfg.issuer,
				algorithms: ["RS256"],
				requiredClaims: ["exp"],
				clockTolerance: this.cfg.clockSkewSeconds,
				currentDate: now,
			}));
		} catch (e) {
			if (e instanceof joseErrors.JWTExpired) {
				throw new FieldError(FieldReason.Expired);
			}
			// Key-fetch trouble (timeout, network) is not a verdict on the token.
			if (
				e instanceof joseErrors.JWKSTimeout ||
				(e instanceof Error && e.name === "TypeError")
			) {
				throw new FieldVerifierUnavailableError(
					`fetch JWKS: ${(e as Error).message}`,
					{ cause: e },
				);
			}
			throw new FieldError(
				FieldReason.InvalidToken,
				e instanceof Error ? e.message : String(e),
			);
		}

		const claims = payload as JWTPayload & Record<string, unknown>;
		if (claims["token_use"] !== "field") {
			throw new FieldError(
				FieldReason.WrongTokenUse,
				`token_use is ${String(claims["token_use"] ?? "")}`,
			);
		}
		const aud = Array.isArray(claims.aud)
			? claims.aud.length === 1
				? claims.aud[0]
				: undefined
			: claims.aud;
		if (aud !== this.cfg.appCode) {
			throw new FieldError(FieldReason.WrongAud, `aud is ${String(aud ?? "")}`);
		}
		if (claims["pcr"] === true) {
			throw new FieldError(
				FieldReason.PCR,
				"the token may only change the PIN",
			);
		}
		const cnf = claims["cnf"] as { jkt?: unknown } | undefined;
		const cnfJkt = typeof cnf?.jkt === "string" ? cnf.jkt : "";
		if (!cnfJkt) {
			throw new FieldError(
				FieldReason.JKTMismatch,
				"the token carries no cnf.jkt",
			);
		}
		if (!dpopHeader || dpopHeader.trim() === "") {
			throw new FieldError(FieldReason.MissingProof);
		}
		const proof = parseProof(dpopHeader);
		if (proof.jkt !== cnfJkt) throw new FieldError(FieldReason.JKTMismatch);
		if (proof.htm !== method) throw new FieldError(FieldReason.HTMMismatch);
		if (!sameUrl(proof.htu, requestUrl)) {
			throw new FieldError(FieldReason.HTUMismatch);
		}
		const ath = createHash("sha256").update(token).digest("base64url");
		if (proof.ath !== ath) throw new FieldError(FieldReason.ATHMismatch);
		const driftMs = Math.abs(now.getTime() - proof.iat * 1000);
		if (driftMs > this.cfg.proofMaxAgeSeconds * 1000) {
			throw new FieldError(FieldReason.StaleProof);
		}

		const str = (k: string): string =>
			typeof claims[k] === "string" ? (claims[k] as string) : "";
		const num = (k: string): number =>
			typeof claims[k] === "number" ? (claims[k] as number) : 0;
		const act = claims["act"] as
			| { sub?: unknown; typ?: unknown; name?: unknown }
			| undefined;
		const actor: FieldActor | null =
			act && typeof act.sub === "string" && act.sub !== ""
				? {
						id: act.sub,
						type: typeof act.typ === "string" ? act.typ : "",
						name: typeof act.name === "string" ? act.name : "",
					}
				: null;
		return {
			workerId: claims.sub ?? "",
			clientId: str("client_id"),
			clientIdentifier: str("client_identifier"),
			workerCode: str("worker_code"),
			name: str("name"),
			appCode: this.cfg.appCode,
			deviceId: str("device_id"),
			deviceMode: str("device_mode"),
			sessionId: str("sid"),
			amr: Array.isArray(claims["amr"])
				? (claims["amr"] as unknown[]).filter(
						(x): x is string => typeof x === "string",
					)
				: [],
			authTime: new Date(num("auth_time") * 1000),
			expiresAt: new Date(num("exp") * 1000),
			jkt: proof.jkt,
			tokenId: claims.jti ?? "",
			actor,
			test: claims["test"] === true,
		};
	}
}

/** Convenience: build a verifier and verify once. */
export function verifyFieldToken(
	config: FieldTokenVerifierConfig,
	authorizationHeader: string | undefined,
	dpopHeader: string | undefined,
	method: string,
	requestUrl: string,
): Promise<FieldPrincipal> {
	return new FieldTokenVerifier(config).verify(
		authorizationHeader,
		dpopHeader,
		method,
		requestUrl,
	);
}

/** scheme + host (case-insensitive) + path; query and fragment ignored. */
function sameUrl(a: string, b: string): boolean {
	let ua: URL, ub: URL;
	try {
		ua = new URL(a);
		ub = new URL(b);
	} catch {
		return false;
	}
	return (
		ua.protocol === ub.protocol && ua.host === ub.host && ua.pathname === ub.pathname
	);
}

interface Proof {
	jkt: string;
	htm: string;
	htu: string;
	ath: string;
	iat: number;
}

function parseProof(compact: string): Proof {
	const bad = (detail: string) => new FieldError(FieldReason.InvalidProof, detail);
	const parts = compact.trim().split(".");
	if (parts.length !== 3) throw bad("not a compact JWS");
	let hdr: {
		typ?: string;
		alg?: string;
		jwk?: { kty?: string; crv?: string; x?: string; y?: string; d?: string };
	};
	try {
		hdr = JSON.parse(Buffer.from(parts[0]!, "base64url").toString("utf8"));
	} catch {
		throw bad("header is not JSON");
	}
	if (!hdr || typeof hdr !== "object") throw bad("header is not JSON");
	if (hdr.typ !== "dpop+jwt" || hdr.alg !== "ES256") {
		throw bad("typ must be dpop+jwt and alg ES256");
	}
	const jwk = hdr.jwk;
	if (!jwk || jwk.kty !== "EC" || jwk.crv !== "P-256" || jwk.d) {
		throw bad("jwk must be a public EC P-256 key");
	}
	const x = Buffer.from(jwk.x ?? "", "base64url");
	const y = Buffer.from(jwk.y ?? "", "base64url");
	if (x.length !== 32 || y.length !== 32) {
		throw bad("jwk coordinates are malformed");
	}
	const xs = x.toString("base64url");
	const ys = y.toString("base64url");
	let pub: KeyObject;
	try {
		pub = createPublicKey({
			key: { kty: "EC", crv: "P-256", x: xs, y: ys },
			format: "jwk",
		});
	} catch {
		throw bad("jwk point is not on the curve");
	}
	const sig = Buffer.from(parts[2]!, "base64url");
	if (sig.length !== 64) throw bad("signature is malformed");
	const ok = cryptoVerify(
		"sha256",
		Buffer.from(`${parts[0]}.${parts[1]}`),
		{ key: pub, dsaEncoding: "ieee-p1363" },
		sig,
	);
	if (!ok) throw bad("signature does not verify");
	let c: { htm?: unknown; htu?: unknown; iat?: unknown; jti?: unknown; ath?: unknown };
	try {
		c = JSON.parse(Buffer.from(parts[1]!, "base64url").toString("utf8"));
	} catch {
		throw bad("claims are not JSON");
	}
	if (
		!c ||
		typeof c !== "object" ||
		typeof c.iat !== "number" ||
		typeof c.jti !== "string" ||
		c.jti === ""
	) {
		throw bad("claims need iat and jti");
	}
	// RFC 7638 thumbprint: required members, lexicographic order, no whitespace.
	const canonical = `{"crv":"P-256","kty":"EC","x":"${xs}","y":"${ys}"}`;
	return {
		jkt: createHash("sha256").update(canonical).digest("base64url"),
		htm: typeof c.htm === "string" ? c.htm : "",
		htu: typeof c.htu === "string" ? c.htu : "",
		ath: typeof c.ath === "string" ? c.ath : "",
		iat: c.iat,
	};
}
