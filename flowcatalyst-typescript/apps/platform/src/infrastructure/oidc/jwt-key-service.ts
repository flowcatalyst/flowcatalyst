/**
 * JWT Key Service
 *
 * Manages RSA key pairs for RS256 JWT signing and verification.
 * Ports the Java JwtKeyService to TypeScript using the `jose` library.
 *
 * Supports two modes:
 * 1. Auto-generated keys (development) - generates RSA key pair on startup,
 *    persists to disk so sessions survive restarts.
 * 2. File-based keys (production) - loads from configured PEM file paths.
 *
 * Provides JWKS for token verification by other services (message router, SDKs).
 */

import * as jose from 'jose';
import { createHash } from 'crypto';
import { readFile, writeFile, mkdir } from 'fs/promises';
import { existsSync } from 'fs';
import path from 'path';

export interface JwtKeyServiceConfig {
	issuer: string;
	/** Path to RSA private key PEM file (production) */
	privateKeyPath?: string | undefined;
	/** Path to RSA public key PEM file (production) */
	publicKeyPath?: string | undefined;
	/** Directory for auto-generated dev keys (default: '.jwt-keys') */
	devKeyDir?: string | undefined;
	/** Session token TTL in seconds (default: 86400 = 24 hours) */
	sessionTokenTtl?: number | undefined;
	/** Access token TTL in seconds (default: 3600 = 1 hour) */
	accessTokenTtl?: number | undefined;
}

export interface JwtKeyService {
	issueSessionToken(principalId: string, email: string, roles: string[], clients: string[]): Promise<string>;
	issueAccessToken(principalId: string, clientId: string, roles: string[]): Promise<string>;
	validateAndGetPrincipalId(token: string): Promise<string | null>;
	getJwks(): jose.JSONWebKeySet;
	getKeyId(): string;
}

/**
 * Extract unique application codes from role strings.
 * Role format: "{application}:{role-name}" (e.g. "operant:admin" -> "operant").
 * Roles without a colon are treated as the application code themselves.
 */
export function extractApplicationCodes(roles: string[]): string[] {
	const codes = new Set<string>();
	for (const role of roles) {
		const idx = role.indexOf(':');
		const code = idx > 0 ? role.substring(0, idx) : role;
		if (code) {
			codes.add(code);
		}
	}
	return [...codes];
}

/**
 * Create and initialize a JwtKeyService.
 * Loads or generates RSA key pair, computes key ID.
 */
export async function createJwtKeyService(config: JwtKeyServiceConfig): Promise<JwtKeyService> {
	const {
		issuer,
		privateKeyPath,
		publicKeyPath,
		devKeyDir = '.jwt-keys',
		sessionTokenTtl = 86400,
		accessTokenTtl = 3600,
	} = config;

	type JoseKey = Awaited<ReturnType<typeof jose.importPKCS8>>;

	let privateKey: JoseKey;
	let publicKey: JoseKey;

	if (privateKeyPath && publicKeyPath) {
		// Production: load from PEM files
		const privatePem = await readFile(privateKeyPath, 'utf-8');
		const publicPem = await readFile(publicKeyPath, 'utf-8');
		privateKey = await jose.importPKCS8(privatePem, 'RS256');
		publicKey = await jose.importSPKI(publicPem, 'RS256');
	} else {
		// Development: load persisted keys or generate new ones
		const privKeyFile = path.join(devKeyDir, 'private.pem');
		const pubKeyFile = path.join(devKeyDir, 'public.pem');

		if (existsSync(privKeyFile) && existsSync(pubKeyFile)) {
			const privatePem = await readFile(privKeyFile, 'utf-8');
			const publicPem = await readFile(pubKeyFile, 'utf-8');
			privateKey = await jose.importPKCS8(privatePem, 'RS256');
			publicKey = await jose.importSPKI(publicPem, 'RS256');
		} else {
			const keyPair = await jose.generateKeyPair('RS256', {
				modulusLength: 2048,
				extractable: true,
			});
			privateKey = keyPair.privateKey;
			publicKey = keyPair.publicKey;

			// Persist for session survival across restarts
			await mkdir(devKeyDir, { recursive: true });
			const privatePem = await jose.exportPKCS8(privateKey);
			const publicPem = await jose.exportSPKI(publicKey);
			await writeFile(privKeyFile, privatePem, 'utf-8');
			await writeFile(pubKeyFile, publicPem, 'utf-8');
		}
	}

	// Generate key ID: SHA-256(SPKI DER bytes) -> base64url -> first 8 chars
	// Matches Java's generateKeyId() which uses key.getEncoded() (X.509/SPKI DER)
	const spkiPem = await jose.exportSPKI(publicKey);
	const derBytes = pemToBuffer(spkiPem);
	const hash = createHash('sha256').update(derBytes).digest();
	const keyId = jose.base64url.encode(hash).substring(0, 8);

	// Export public key as JWK for JWKS endpoint
	const publicJwk = await jose.exportJWK(publicKey);
	publicJwk.kid = keyId;
	publicJwk.alg = 'RS256';
	publicJwk.use = 'sig';

	const jwks: jose.JSONWebKeySet = { keys: [publicJwk] };

	return {
		async issueSessionToken(principalId: string, email: string, roles: string[], clients: string[]): Promise<string> {
			const applications = extractApplicationCodes(roles);

			return new jose.SignJWT({
				email,
				type: 'USER',
				roles,
				clients,
				applications,
			})
				.setProtectedHeader({ alg: 'RS256', kid: keyId })
				.setIssuer(issuer)
				.setSubject(principalId)
				.setIssuedAt()
				.setExpirationTime(`${sessionTokenTtl}s`)
				.sign(privateKey);
		},

		async issueAccessToken(principalId: string, clientId: string, roles: string[]): Promise<string> {
			const applications = extractApplicationCodes(roles);

			return new jose.SignJWT({
				client_id: clientId,
				type: 'SERVICE',
				roles,
				applications,
			})
				.setProtectedHeader({ alg: 'RS256', kid: keyId })
				.setIssuer(issuer)
				.setSubject(principalId)
				.setIssuedAt()
				.setExpirationTime(`${accessTokenTtl}s`)
				.sign(privateKey);
		},

		async validateAndGetPrincipalId(token: string): Promise<string | null> {
			try {
				const { payload } = await jose.jwtVerify(token, publicKey, {
					issuer,
				});
				return typeof payload.sub === 'string' ? payload.sub : null;
			} catch {
				return null;
			}
		},

		getJwks(): jose.JSONWebKeySet {
			return jwks;
		},

		getKeyId(): string {
			return keyId;
		},
	};
}

/**
 * Strip PEM headers/footers and decode base64 to get DER bytes.
 */
function pemToBuffer(pem: string): Buffer {
	const base64 = pem
		.replace(/-----BEGIN [A-Z ]+-----/g, '')
		.replace(/-----END [A-Z ]+-----/g, '')
		.replace(/\s/g, '');
	return Buffer.from(base64, 'base64');
}
