/**
 * JWT Key Service
 *
 * Manages RSA key pairs for RS256 JWT signing and verification.
 * Ports the Java JwtKeyService to TypeScript using the `jose` library.
 *
 * Supports three modes (checked in priority order):
 * 1. Key directory (rotation-capable) — loads all key pairs from a directory,
 *    newest by mtime is the signing key, all public keys appear in JWKS.
 * 2. File-based keys (legacy/production) — loads from configured PEM file paths.
 * 3. Auto-generated keys (development) — generates RSA key pair on startup,
 *    persists to key directory so sessions survive restarts.
 *
 * Provides JWKS for token verification by other services (message router, SDKs).
 */

import * as jose from 'jose';
import { readFile } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import path from 'node:path';
import { computeKeyId, generateKeyPair, writeKeyPair, loadKeyDir } from './key-utils.js';

export interface JwtKeyServiceConfig {
  issuer: string;
  /** Base64-encoded PEM private key content (containers/cloud, highest priority) */
  privateKey?: string | undefined;
  /** Base64-encoded PEM public key content (containers/cloud, highest priority) */
  publicKey?: string | undefined;
  /** Base64-encoded PEM previous public key for zero-downtime key rotation */
  previousPublicKey?: string | undefined;
  /** Directory containing key pairs: {kid}.private.pem + {kid}.public.pem */
  keyDir?: string | undefined;
  /** Path to RSA private key PEM file (legacy/production) */
  privateKeyPath?: string | undefined;
  /** Path to RSA public key PEM file (legacy/production) */
  publicKeyPath?: string | undefined;
  /** Directory for auto-generated dev keys (default: '.jwt-keys'). Deprecated: use keyDir. */
  devKeyDir?: string | undefined;
  /** Session token TTL in seconds (default: 86400 = 24 hours) */
  sessionTokenTtl?: number | undefined;
  /** Access token TTL in seconds (default: 3600 = 1 hour) */
  accessTokenTtl?: number | undefined;
}

export interface JwtKeyService {
  issueSessionToken(
    principalId: string,
    email: string,
    roles: string[],
    clients: string[],
  ): Promise<string>;
  issueAccessToken(principalId: string, clientId: string, roles: string[]): Promise<string>;
  validateAndGetPrincipalId(token: string): Promise<string | null>;
  /** Public-only JWKS for the /.well-known/jwks.json endpoint */
  getJwks(): jose.JSONWebKeySet;
  /** JWKS with private key for oidc-provider (signing) */
  getSigningJwks(): jose.JSONWebKeySet;
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
 * Loads or generates RSA key pair(s), computes key IDs, builds JWKS.
 */
export async function createJwtKeyService(config: JwtKeyServiceConfig): Promise<JwtKeyService> {
  const {
    issuer,
    privateKey,
    publicKey,
    previousPublicKey,
    keyDir,
    privateKeyPath,
    publicKeyPath,
    devKeyDir = '.jwt-keys',
    sessionTokenTtl = 86400,
    accessTokenTtl = 3600,
  } = config;

  type JoseKey = Awaited<ReturnType<typeof jose.importPKCS8>>;

  let signingPrivateKey: JoseKey;
  let signingKeyId: string;
  let jwks: jose.JSONWebKeySet; // public keys only (for JWKS endpoint)
  let signingJwks: jose.JSONWebKeySet; // with private key (for oidc-provider)

  // Resolve the effective key directory
  const effectiveKeyDir = keyDir ?? devKeyDir;

  if (privateKey && publicKey) {
    // Mode 1: Base64-encoded PEM content from env vars (containers/cloud)
    const result = await loadKeysFromEnvVars(privateKey, publicKey, previousPublicKey);
    signingPrivateKey = result.signingPrivateKey;
    signingKeyId = result.signingKeyId;
    jwks = result.jwks;
    signingJwks = result.signingJwks;
  } else if (keyDir) {
    // Mode 2: Directory-based multi-key (rotation-capable)
    const result = await loadOrBootstrapKeyDir(effectiveKeyDir);
    signingPrivateKey = result.signingPrivateKey;
    signingKeyId = result.signingKeyId;
    jwks = result.jwks;
    signingJwks = result.signingJwks;
  } else if (privateKeyPath && publicKeyPath) {
    // Mode 3: Legacy single file-based keys (production)
    const privatePem = await readFile(privateKeyPath, 'utf-8');
    const publicPem = await readFile(publicKeyPath, 'utf-8');
    signingPrivateKey = await jose.importPKCS8(privatePem, 'RS256');
    signingKeyId = computeKeyId(publicPem);

    const publicKey = await jose.importSPKI(publicPem, 'RS256');
    const publicJwk = await jose.exportJWK(publicKey);
    publicJwk.kid = signingKeyId;
    publicJwk.alg = 'RS256';
    publicJwk.use = 'sig';
    jwks = { keys: [publicJwk] };

    const extractablePrivKey = await jose.importPKCS8(privatePem, 'RS256', { extractable: true });
    const privateJwk = await jose.exportJWK(extractablePrivKey);
    privateJwk.kid = signingKeyId;
    privateJwk.alg = 'RS256';
    privateJwk.use = 'sig';
    signingJwks = { keys: [privateJwk] };
  } else {
    // Mode 4: Auto-generate into devKeyDir (development)
    const result = await loadOrBootstrapKeyDir(effectiveKeyDir);
    signingPrivateKey = result.signingPrivateKey;
    signingKeyId = result.signingKeyId;
    jwks = result.jwks;
    signingJwks = result.signingJwks;
  }

  // Create JWKS resolver for multi-key validation
  const jwksGetKey = jose.createLocalJWKSet(jwks);

  return {
    async issueSessionToken(
      principalId: string,
      email: string,
      roles: string[],
      clients: string[],
    ): Promise<string> {
      const applications = extractApplicationCodes(roles);

      return new jose.SignJWT({
        email,
        type: 'USER',
        roles,
        clients,
        applications,
      })
        .setProtectedHeader({ alg: 'RS256', kid: signingKeyId })
        .setIssuer(issuer)
        .setSubject(principalId)
        .setIssuedAt()
        .setExpirationTime(`${sessionTokenTtl}s`)
        .sign(signingPrivateKey);
    },

    async issueAccessToken(
      principalId: string,
      clientId: string,
      roles: string[],
    ): Promise<string> {
      const applications = extractApplicationCodes(roles);

      return new jose.SignJWT({
        client_id: clientId,
        type: 'SERVICE',
        roles,
        applications,
      })
        .setProtectedHeader({ alg: 'RS256', kid: signingKeyId })
        .setIssuer(issuer)
        .setSubject(principalId)
        .setIssuedAt()
        .setExpirationTime(`${accessTokenTtl}s`)
        .sign(signingPrivateKey);
    },

    async validateAndGetPrincipalId(token: string): Promise<string | null> {
      try {
        const { payload } = await jose.jwtVerify(token, jwksGetKey, {
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

    getSigningJwks(): jose.JSONWebKeySet {
      return signingJwks;
    },

    getKeyId(): string {
      return signingKeyId;
    },
  };
}

/**
 * Decode a base64-encoded PEM string back to PEM text.
 * Matches the Java JwtKeyService.loadKeysFromEnvVars() behavior:
 * env vars contain base64(PEM content), so we decode to get the PEM.
 */
function decodeBase64Pem(base64: string): string {
  return Buffer.from(base64, 'base64').toString('utf-8');
}

/**
 * Load keys from base64-encoded PEM content (env vars).
 * Supports an optional previous public key for zero-downtime key rotation.
 */
async function loadKeysFromEnvVars(
  privateKeyBase64: string,
  publicKeyBase64: string,
  previousPublicKeyBase64?: string | undefined,
) {
  const privatePem = decodeBase64Pem(privateKeyBase64);
  const publicPem = decodeBase64Pem(publicKeyBase64);

  const signingPrivateKey = await jose.importPKCS8(privatePem, 'RS256');
  const signingKeyId = computeKeyId(publicPem);

  // Current public key
  const currentPublicKey = await jose.importSPKI(publicPem, 'RS256');
  const currentPublicJwk = await jose.exportJWK(currentPublicKey);
  currentPublicJwk.kid = signingKeyId;
  currentPublicJwk.alg = 'RS256';
  currentPublicJwk.use = 'sig';

  const publicJwkKeys: jose.JWK[] = [currentPublicJwk];

  // Previous public key for rotation (tokens signed with old key remain valid)
  if (previousPublicKeyBase64) {
    const previousPem = decodeBase64Pem(previousPublicKeyBase64);
    const previousPublicKey = await jose.importSPKI(previousPem, 'RS256');
    const previousJwk = await jose.exportJWK(previousPublicKey);
    previousJwk.kid = computeKeyId(previousPem);
    previousJwk.alg = 'RS256';
    previousJwk.use = 'sig';
    publicJwkKeys.push(previousJwk);
  }

  const jwks: jose.JSONWebKeySet = { keys: publicJwkKeys };

  // Signing JWKS (private key for oidc-provider)
  const extractablePrivKey = await jose.importPKCS8(privatePem, 'RS256', { extractable: true });
  const signingPrivateJwk = await jose.exportJWK(extractablePrivKey);
  signingPrivateJwk.kid = signingKeyId;
  signingPrivateJwk.alg = 'RS256';
  signingPrivateJwk.use = 'sig';
  const signingJwks: jose.JSONWebKeySet = { keys: [signingPrivateJwk] };

  return { signingPrivateKey, signingKeyId, jwks, signingJwks };
}

/**
 * Load key pairs from a directory, or generate a new one if none exist.
 * Returns the signing key (newest) and JWKS (all public keys).
 */
async function loadOrBootstrapKeyDir(dir: string) {
  type JoseKey = Awaited<ReturnType<typeof jose.importPKCS8>>;

  let pairs = await loadKeyDir(dir);

  // Migrate legacy dev keys (private.pem + public.pem) to {kid}.* format
  if (pairs.length === 0) {
    const legacyPriv = path.join(dir, 'private.pem');
    const legacyPub = path.join(dir, 'public.pem');
    if (existsSync(legacyPriv) && existsSync(legacyPub)) {
      const publicPem = await readFile(legacyPub, 'utf-8');
      const privatePem = await readFile(legacyPriv, 'utf-8');
      const kid = computeKeyId(publicPem);
      await writeKeyPair(dir, kid, privatePem, publicPem);
      pairs = await loadKeyDir(dir);
    }
  }

  // No keys found — generate an initial key pair
  if (pairs.length === 0) {
    const { kid, privatePem, publicPem } = await generateKeyPair();
    await writeKeyPair(dir, kid, privatePem, publicPem);
    pairs = await loadKeyDir(dir);
  }

  // Newest key pair (last after sort by mtime) is the signing key
  const signingPair = pairs[pairs.length - 1]!;
  const signingPrivateKey: JoseKey = await jose.importPKCS8(signingPair.privatePem, 'RS256');
  const signingKeyId = signingPair.kid;

  // Build public JWKS (for JWKS endpoint) and signing JWKS (for oidc-provider)
  const publicJwkKeys: jose.JWK[] = [];
  for (const pair of pairs) {
    const publicKey = await jose.importSPKI(pair.publicPem, 'RS256');
    const jwk = await jose.exportJWK(publicKey);
    jwk.kid = pair.kid;
    jwk.alg = 'RS256';
    jwk.use = 'sig';
    publicJwkKeys.push(jwk);
  }
  const jwks: jose.JSONWebKeySet = { keys: publicJwkKeys };

  // Signing JWKS includes the private key (needed by oidc-provider)
  // Re-import as extractable so we can export to JWK format
  const extractableKey = await jose.importPKCS8(signingPair.privatePem, 'RS256', {
    extractable: true,
  });
  const signingPrivateJwk = await jose.exportJWK(extractableKey);
  signingPrivateJwk.kid = signingKeyId;
  signingPrivateJwk.alg = 'RS256';
  signingPrivateJwk.use = 'sig';
  const signingJwks: jose.JSONWebKeySet = { keys: [signingPrivateJwk] };

  return { signingPrivateKey, signingKeyId, jwks, signingJwks };
}
