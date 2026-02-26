/**
 * JWT Key Utilities
 *
 * Shared functions for RSA key pair generation, loading, and management.
 * Used by both the JWT key service (runtime) and the rotate-keys CLI command.
 */

import * as jose from "jose";
import { createHash } from "node:crypto";
import {
	readFile,
	writeFile,
	mkdir,
	readdir,
	stat,
	unlink,
} from "node:fs/promises";
import { existsSync } from "node:fs";
import path from "node:path";

export interface KeyPairFiles {
	kid: string;
	privatePem: string;
	publicPem: string;
	mtime: Date;
}

/**
 * Compute a key ID from an RSA public key PEM.
 * SHA-256(SPKI DER bytes) → base64url → first 8 chars.
 * Matches the Java platform's generateKeyId().
 */
export function computeKeyId(publicPem: string): string {
	const base64 = publicPem
		.replace(/-----BEGIN [A-Z ]+-----/g, "")
		.replace(/-----END [A-Z ]+-----/g, "")
		.replace(/\s/g, "");
	const derBytes = Buffer.from(base64, "base64");
	const hash = createHash("sha256").update(derBytes).digest();
	return jose.base64url.encode(hash).substring(0, 8);
}

/**
 * Generate a new 2048-bit RSA key pair and compute its kid.
 */
export async function generateKeyPair(): Promise<{
	kid: string;
	privatePem: string;
	publicPem: string;
}> {
	const { privateKey, publicKey } = await jose.generateKeyPair("RS256", {
		modulusLength: 2048,
		extractable: true,
	});
	const privatePem = await jose.exportPKCS8(privateKey);
	const publicPem = await jose.exportSPKI(publicKey);
	const kid = computeKeyId(publicPem);
	return { kid, privatePem, publicPem };
}

/**
 * Write a key pair to a directory as {kid}.private.pem and {kid}.public.pem.
 */
export async function writeKeyPair(
	dir: string,
	kid: string,
	privatePem: string,
	publicPem: string,
): Promise<void> {
	await mkdir(dir, { recursive: true });
	await writeFile(path.join(dir, `${kid}.private.pem`), privatePem, "utf-8");
	await writeFile(path.join(dir, `${kid}.public.pem`), publicPem, "utf-8");
}

/**
 * Load all key pairs from a directory.
 * Looks for files matching {kid}.public.pem and {kid}.private.pem.
 * Returns sorted by mtime ascending (oldest first, newest last).
 */
export async function loadKeyDir(dir: string): Promise<KeyPairFiles[]> {
	if (!existsSync(dir)) return [];

	const files = await readdir(dir);
	const publicFiles = files.filter((f) => f.endsWith(".public.pem"));

	const pairs: KeyPairFiles[] = [];

	for (const pubFile of publicFiles) {
		const kid = pubFile.replace(".public.pem", "");
		const privFile = `${kid}.private.pem`;

		const pubPath = path.join(dir, pubFile);
		const privPath = path.join(dir, privFile);

		if (!files.includes(privFile)) continue;

		const [publicPem, privatePem, pubStat] = await Promise.all([
			readFile(pubPath, "utf-8"),
			readFile(privPath, "utf-8"),
			stat(pubPath),
		]);

		pairs.push({ kid, privatePem, publicPem, mtime: pubStat.mtime });
	}

	// Sort by mtime ascending — last element is the newest (signing key)
	pairs.sort((a, b) => a.mtime.getTime() - b.mtime.getTime());

	return pairs;
}

/**
 * Remove a key pair from a directory.
 */
export async function removeKeyPair(dir: string, kid: string): Promise<void> {
	await Promise.all([
		unlink(path.join(dir, `${kid}.private.pem`)).catch(() => {}),
		unlink(path.join(dir, `${kid}.public.pem`)).catch(() => {}),
	]);
}
