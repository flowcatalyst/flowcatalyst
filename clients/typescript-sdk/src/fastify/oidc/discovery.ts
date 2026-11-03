/**
 * OIDC discovery + JWKS cache.
 *
 * Lazy first-use fetch of `{baseUrl}/.well-known/openid-configuration`.
 * JWKS is cached by `jose.createRemoteJWKSet` with its own refresh policy
 * (cooldown + rotation tolerance). We expose only the bits the plugin
 * needs — token, authorize, end-session endpoints + a verifier callable.
 */

import { createRemoteJWKSet, jwtVerify, type JWTPayload } from "jose";

export interface DiscoveryDoc {
	issuer: string;
	authorization_endpoint: string;
	token_endpoint: string;
	jwks_uri: string;
	end_session_endpoint?: string;
	userinfo_endpoint?: string;
	introspection_endpoint?: string;
	revocation_endpoint?: string;
}

/**
 * `token_use` values an access token may carry to be accepted here: `api`
 * (authority-bearing), `identity` (login proof, no authority) or absent
 * (older platform versions). Anything else (notably `field`, the
 * field-worker plane) is refused: a token minted for another plane must
 * never authenticate as a platform principal, whatever its `aud`.
 */
export const ACCEPTED_TOKEN_USE: readonly string[] = ["api", "identity"];

export function assertAccessTokenUse(payload: JWTPayload): void {
	const use = (payload as Record<string, unknown>)["token_use"];
	if (use === undefined) return;
	if (typeof use !== "string" || !ACCEPTED_TOKEN_USE.includes(use)) {
		throw new Error(`token_use ${String(use)} is not an access token for this plane`);
	}
}

export interface OidcEndpoints {
	issuer: string;
	authorizationEndpoint: string;
	tokenEndpoint: string;
	endSessionEndpoint?: string;
	verify: (token: string) => Promise<JWTPayload>;
	/**
	 * Verify an ID token. Distinct from {@link verify} because the two have
	 * different audiences: an access token is addressed to the platform API
	 * (`expectedAudience`), while an ID token is addressed to the relying
	 * party — OIDC Core §3.1.3.7 requires `aud` to contain the client_id.
	 * Passing an ID token to `verify` would fail whenever `expectedAudience`
	 * is configured, and would skip the client check when it is not.
	 */
	verifyIdToken: (token: string, clientId: string) => Promise<JWTPayload>;
}

interface Pending {
	endpoints: Promise<OidcEndpoints>;
}

export function createOidcClient(opts: {
	baseUrl: string;
	expectedAudience?: string;
	/**
	 * Override the discovered authorization endpoint. Portal-plane logins
	 * (docs/portal-identity-plan.md) enter through `/portal/authorize`
	 * instead of the discovered `/oauth/authorize`; the token endpoint and
	 * JWKS stay as discovered.
	 */
	authorizationEndpoint?: string;
}): { endpoints(): Promise<OidcEndpoints> } {
	let pending: Pending | undefined;
	return {
		endpoints() {
			if (!pending) {
				const endpoints = load(
					opts.baseUrl,
					opts.expectedAudience,
					opts.authorizationEndpoint,
				);
				// A rejected discovery must NOT stay memoized: with the platform
				// briefly unreachable, a poisoned memo would 500 every login in
				// this process until restart. Clear the memo on rejection (the
				// caller still sees the rejection) so the next call retries.
				endpoints.catch(() => {
					if (pending?.endpoints === endpoints) {
						pending = undefined;
					}
				});
				pending = { endpoints };
			}
			return pending.endpoints;
		},
	};
}

async function load(
	baseUrl: string,
	expectedAudience: string | undefined,
	authorizationEndpointOverride?: string,
): Promise<OidcEndpoints> {
	const url = `${stripSlash(baseUrl)}/.well-known/openid-configuration`;
	const res = await fetch(url, { headers: { Accept: "application/json" } });
	if (!res.ok) {
		throw new Error(
			`OIDC discovery failed: ${res.status} ${res.statusText} (${url})`,
		);
	}
	const doc = (await res.json()) as DiscoveryDoc;
	if (!doc.issuer || !doc.token_endpoint || !doc.authorization_endpoint || !doc.jwks_uri) {
		throw new Error("OIDC discovery document missing required fields");
	}
	const jwks = createRemoteJWKSet(new URL(doc.jwks_uri));
	return {
		issuer: doc.issuer,
		authorizationEndpoint: authorizationEndpointOverride ?? doc.authorization_endpoint,
		tokenEndpoint: doc.token_endpoint,
		...(doc.end_session_endpoint
			? { endSessionEndpoint: doc.end_session_endpoint }
			: {}),
		async verify(token: string) {
			const { payload } = await jwtVerify(token, jwks, {
				issuer: doc.issuer,
				...(expectedAudience ? { audience: expectedAudience } : {}),
			});
			assertAccessTokenUse(payload);
			return payload;
		},
		async verifyIdToken(token: string, clientId: string) {
			const { payload } = await jwtVerify(token, jwks, {
				issuer: doc.issuer,
				audience: clientId,
			});
			return payload;
		},
	};
}

function stripSlash(s: string): string {
	return s.endsWith("/") ? s.slice(0, -1) : s;
}
