/**
 * CookieSessionStore — default backend. The encrypted cookie IS the session.
 *
 * Pros: zero infra, stateless, no DB hop on every request.
 * Cons: ~4KB hard browser cookie limit. Once `sessionData` gets fat, switch
 * to {@link PgSessionStore} or {@link RedisSessionStore}.
 *
 * Cookie shape: `<envelope>` produced by {@link SessionCrypto}.
 */

import type { FastifyReply, FastifyRequest } from "fastify";
import { createSessionCrypto, type SessionCrypto } from "../crypto.js";
import type { SessionPayload, SessionStore } from "./types.js";

export interface CookieSessionStoreOptions {
	cookieName: string;
	secret: string | readonly string[];
	cookieOptions: CookieAttrs;
	/**
	 * What this store's cookie is for. Written into the encrypted payload and
	 * enforced on read, so a cookie minted for one purpose (e.g. the OAuth
	 * state stash) can never be replayed as another (a session). Default
	 * `"session"`.
	 */
	purpose?: CookiePurpose;
}

export type CookiePurpose = "session" | "oauth_state";

/** `principal.id` of the OAuth state stash (see the plugin's `bagToSession`). */
export const OAUTH_STATE_PRINCIPAL_ID = "_oauth_state";

export interface CookieAttrs {
	path: string;
	domain?: string;
	httpOnly: boolean;
	secure: boolean;
	sameSite: "lax" | "strict" | "none";
	maxAge: number; // seconds
}

export class CookieSessionStore implements SessionStore {
	private readonly crypto: SessionCrypto;
	private readonly opts: CookieSessionStoreOptions;

	constructor(opts: CookieSessionStoreOptions) {
		this.opts = opts;
		this.crypto = createSessionCrypto(opts.secret);
	}

	async read<TData>(
		req: FastifyRequest,
	): Promise<SessionPayload<TData> | null> {
		const raw = req.cookies?.[this.opts.cookieName];
		if (!raw) return null;
		const json = await this.crypto.decrypt(raw);
		if (!json) return null;
		try {
			const parsed = JSON.parse(json) as SessionPayload<TData>;
			if (!parsed || typeof parsed !== "object") return null;
			if (typeof parsed.expiresAt !== "number" || parsed.expiresAt < Date.now()) {
				return null;
			}
			if (!this.acceptsPayload(parsed)) return null;
			// The binding fields are the store's business; callers get the
			// payload they wrote.
			const { purpose: _purpose, cookie: _cookie, ...payload } = parsed;
			return payload as SessionPayload<TData>;
		} catch {
			return null;
		}
	}

	/**
	 * Purpose / cookie-name binding. The binding fields live inside the
	 * encrypted JSON (the `v1.<iv>.<ct>` envelope is unchanged, so old SDK
	 * versions still read what we write and ignore the extra fields). Legacy
	 * payloads written before the binding existed carry neither field and are
	 * told apart by shape: the state stash has `principal.id === "_oauth_state"`
	 * and an empty access token; a real session has neither.
	 */
	private acceptsPayload(p: SessionPayload<unknown>): boolean {
		const want = this.purpose();
		if (p.purpose !== undefined) {
			if (p.purpose !== want) return false;
		} else {
			const isStateShape = p.principal?.id === OAUTH_STATE_PRINCIPAL_ID;
			if (want === "oauth_state") {
				if (!isStateShape) return false;
			} else if (isStateShape || !p.tokens?.accessToken) {
				return false;
			}
		}
		if (p.cookie !== undefined && p.cookie !== this.opts.cookieName) return false;
		return true;
	}

	private purpose(): CookiePurpose {
		return this.opts.purpose ?? "session";
	}

	async write<TData>(
		reply: FastifyReply,
		session: SessionPayload<TData>,
	): Promise<void> {
		const bound: SessionPayload<TData> = {
			...session,
			purpose: this.purpose(),
			cookie: this.opts.cookieName,
		};
		const envelope = await this.crypto.encrypt(JSON.stringify(bound));
		reply.setCookie(this.opts.cookieName, envelope, this.opts.cookieOptions);
	}

	async clear(_req: FastifyRequest, reply: FastifyReply): Promise<void> {
		reply.clearCookie(this.opts.cookieName, {
			path: this.opts.cookieOptions.path,
			...(this.opts.cookieOptions.domain
				? { domain: this.opts.cookieOptions.domain }
				: {}),
		});
	}
}
