/**
 * Shape of FlowCatalyst access-token claims (both `authorization_code` and
 * `client_credentials` grants — they share the same claim envelope, so the
 * Fastify plugin treats both flows identically).
 *
 * Source of truth: `crates/fc-platform/src/auth/auth_service.rs::AccessTokenClaims`.
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
	scope: "ANCHOR" | "PARTNER" | "CLIENT";
	name: string;
	email?: string;
	clients: string[];
	roles: string[];
	applications: string[];
}

export function claimsToSnapshot(
	claims: FcAccessTokenClaims,
	mechanism: "session" | "bearer",
): Omit<PrincipalSnapshot, "sessionData"> {
	const scope = claims.scope.toLowerCase() as PrincipalScope;
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
