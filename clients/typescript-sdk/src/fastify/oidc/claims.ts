/**
 * Shape of FlowCatalyst access-token claims (both `authorization_code` and
 * `client_credentials` grants — they share the same claim envelope, so the
 * Fastify plugin treats both flows identically).
 *
 * Source of truth: the Go platform's
 * `internal/platform/auth/authservice.AccessTokenClaims`. Layout note: the
 * tenancy tier lives in `tier` ("ANCHOR" | "PARTNER" | "CLIENT"); `scope`
 * is the OPTIONAL granted OAuth scope — a space-delimited permission list —
 * and is absent on tokens that carry no scope claim (e.g. identity tokens
 * from permission-less interactive logins). Legacy Rust-era tokens carried
 * the tier IN `scope`; the mapper below accepts both layouts.
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
	/** Tenancy tier (Go platform). Legacy Rust tokens carried this in `scope`. */
	tier?: "ANCHOR" | "PARTNER" | "CLIENT";
	/** Granted OAuth scope: space-delimited permission codes. Often absent. */
	scope?: string;
	name: string;
	email?: string;
	clients: string[];
	roles: string[];
	applications: string[];
}

const TIERS = new Set(["ANCHOR", "PARTNER", "CLIENT"]);

export function claimsToSnapshot(
	claims: FcAccessTokenClaims,
	mechanism: "session" | "bearer",
): Omit<PrincipalSnapshot, "sessionData"> {
	// Tier: the Go platform's `tier` claim; fall back to a legacy Rust-era
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
