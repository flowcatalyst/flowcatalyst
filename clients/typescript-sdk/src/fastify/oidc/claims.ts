/**
 * Shape of FlowCatalyst token claims, and the mapping to a principal.
 *
 * Source of truth: the Go platform's
 * `internal/platform/auth/authservice` — `AccessTokenClaims` and
 * `IDTokenClaims`. Layout note: the tenancy tier lives in `tier`
 * ("ANCHOR" | "PARTNER" | "CLIENT"); `scope` is the OPTIONAL granted OAuth
 * scope — a space-delimited permission list. Legacy tokens from older
 * platform versions carried the tier IN `scope`; the mapper below accepts
 * both layouts.
 *
 * ## The identity/API token split
 *
 * An `authorization_code` login mints an IDENTITY access token by default:
 * it proves who signed in and carries NO authority — no scope, no roles, no
 * applications, no client access. The platform API rejects it, so a user
 * signing in cannot hand the app an API credential. The user's real
 * authority travels on the **ID token**, narrowed to the applications the
 * OAuth client is scoped to.
 *
 * A first-party client flagged `apiAccess` mints an authority-bearing access
 * token on the same flow (also narrowed to the client's applications). Both
 * shapes are handled: {@link mergeIdTokenAuthority} overlays the ID token's
 * authority onto the access token's identity, so the resulting principal is
 * correct either way, and `client_credentials` (which has no ID token) is
 * untouched.
 */

import type { JWTPayload } from "jose";
import type { PrincipalScope, PrincipalSnapshot, PrincipalType } from "../principal.js";

export interface FcAccessTokenClaims extends JWTPayload {
	sub: string;
	iss: string;
	aud: string | string[];
	exp: number;
	iat: number;
	type: PrincipalType;
	/** Tenancy tier. Legacy tokens carried this in `scope`. */
	tier?: "ANCHOR" | "PARTNER" | "CLIENT";
	/** Granted OAuth scope: space-delimited permission codes. Often absent. */
	scope?: string;
	name: string;
	email?: string;
	clients: string[];
	roles: string[];
	applications: string[];
}

/**
 * Shape of FlowCatalyst ID-token claims — the identity proof addressed to
 * this relying party. `roles`, `applications` and `clients` are always
 * emitted (possibly empty) and are the authoritative copy for an interactive
 * login; `name` and `email` are optional here (the access token always
 * carries `name`). It carries no `scope`: permission scopes belong to the
 * API plane, not to an identity proof.
 */
export interface FcIdTokenClaims extends JWTPayload {
	sub: string;
	iss: string;
	aud: string | string[];
	exp: number;
	iat: number;
	type: PrincipalType;
	tier?: "ANCHOR" | "PARTNER" | "CLIENT";
	name?: string;
	email?: string;
	clients?: string[];
	roles?: string[];
	applications?: string[];
	nonce?: string;
	auth_time?: number;
}

/**
 * Overlay an interactive login's ID-token authority onto its access-token
 * identity, yielding the claims the principal should be built from.
 *
 * The ID token wins on tier and on every authority list, because on the
 * default identity access token those are empty and on an `apiAccess` client
 * both tokens carry the same narrowed set. The access token keeps `name`
 * (always present there, optional on the ID token) and `scope` (an API-plane
 * concept the ID token does not carry).
 *
 * @throws if the two tokens describe different subjects — they are minted
 * from one principal in the same exchange, so a mismatch means they did not
 * come from the same login and neither can be trusted.
 */
export function mergeIdTokenAuthority(
	access: FcAccessTokenClaims,
	id: FcIdTokenClaims,
): FcAccessTokenClaims {
	if (access.sub !== id.sub) {
		throw new Error(
			"OIDC token mismatch: id_token subject differs from access_token subject",
		);
	}
	return {
		...access,
		...(id.tier ? { tier: id.tier } : {}),
		...(id.name ? { name: id.name } : {}),
		...(id.email ? { email: id.email } : {}),
		clients: id.clients ?? access.clients,
		roles: id.roles ?? access.roles,
		applications: id.applications ?? access.applications,
	};
}

const TIERS = new Set(["ANCHOR", "PARTNER", "CLIENT"]);

export function claimsToSnapshot(
	claims: FcAccessTokenClaims,
	mechanism: "session" | "bearer",
): Omit<PrincipalSnapshot, "sessionData"> {
	// Tier: the platform's `tier` claim; fall back to a legacy
	// `scope`-as-tier value; a "*" clients entry also marks anchor. Default
	// CLIENT (the least authority) when nothing identifies the tier —
	// `scope` on modern tokens holds permission scopes, not the tier, and
	// may be absent entirely.
	const legacyTier =
		typeof claims.scope === "string" && TIERS.has(claims.scope)
			? (claims.scope as "ANCHOR" | "PARTNER" | "CLIENT")
			: undefined;
	const tier =
		claims.tier ??
		legacyTier ??
		((claims.clients ?? []).includes("*") ? "ANCHOR" : "CLIENT");
	const scope = tier.toLowerCase() as PrincipalScope;
	return {
		id: claims.sub,
		type: claims.type,
		scope,
		name: claims.name,
		...(claims.email ? { email: claims.email } : {}),
		clients: claims.clients ?? [],
		roles: claims.roles ?? [],
		applications: claims.applications ?? [],
		mechanism,
	};
}
