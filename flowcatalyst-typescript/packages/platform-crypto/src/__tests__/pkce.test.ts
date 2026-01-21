import { describe, it, expect } from 'vitest';
import {
	generateCodeVerifier,
	generateCodeChallenge,
	verifyCodeChallenge,
	isValidCodeVerifier,
	isValidCodeChallenge,
} from '../pkce.js';

describe('generateCodeVerifier', () => {
	it('should generate a 64-character string', () => {
		const verifier = generateCodeVerifier();
		expect(verifier.length).toBe(64);
	});

	it('should generate unique verifiers', () => {
		const verifiers = new Set<string>();
		for (let i = 0; i < 100; i++) {
			verifiers.add(generateCodeVerifier());
		}
		expect(verifiers.size).toBe(100);
	});

	it('should generate valid verifiers', () => {
		for (let i = 0; i < 10; i++) {
			const verifier = generateCodeVerifier();
			expect(isValidCodeVerifier(verifier)).toBe(true);
		}
	});

	it('should only contain unreserved URI characters', () => {
		const verifier = generateCodeVerifier();
		// base64url uses A-Za-z0-9-_ which are all unreserved URI chars
		expect(verifier).toMatch(/^[A-Za-z0-9_-]+$/);
	});
});

describe('generateCodeChallenge', () => {
	it('should generate a 43-character challenge from SHA-256', () => {
		const verifier = generateCodeVerifier();
		const challenge = generateCodeChallenge(verifier);
		// SHA-256 produces 32 bytes, base64url encoded = 43 chars (32 * 8 / 6 = 42.67 rounded up)
		expect(challenge.length).toBe(43);
	});

	it('should generate deterministic challenges for same verifier', () => {
		const verifier = generateCodeVerifier();
		const challenge1 = generateCodeChallenge(verifier);
		const challenge2 = generateCodeChallenge(verifier);
		expect(challenge1).toBe(challenge2);
	});

	it('should generate different challenges for different verifiers', () => {
		const verifier1 = generateCodeVerifier();
		const verifier2 = generateCodeVerifier();
		const challenge1 = generateCodeChallenge(verifier1);
		const challenge2 = generateCodeChallenge(verifier2);
		expect(challenge1).not.toBe(challenge2);
	});

	it('should generate valid challenges', () => {
		const verifier = generateCodeVerifier();
		const challenge = generateCodeChallenge(verifier);
		expect(isValidCodeChallenge(challenge)).toBe(true);
	});

	// RFC 7636 Appendix B test vector
	it('should match RFC 7636 test vector', () => {
		const verifier = 'dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk';
		const expectedChallenge = 'E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM';
		const challenge = generateCodeChallenge(verifier);
		expect(challenge).toBe(expectedChallenge);
	});
});

describe('verifyCodeChallenge', () => {
	describe('S256 method', () => {
		it('should verify matching verifier and challenge', () => {
			const verifier = generateCodeVerifier();
			const challenge = generateCodeChallenge(verifier);
			expect(verifyCodeChallenge(verifier, challenge, 'S256')).toBe(true);
		});

		it('should reject non-matching verifier and challenge', () => {
			const verifier = generateCodeVerifier();
			const challenge = generateCodeChallenge(generateCodeVerifier());
			expect(verifyCodeChallenge(verifier, challenge, 'S256')).toBe(false);
		});

		it('should default to S256 method', () => {
			const verifier = generateCodeVerifier();
			const challenge = generateCodeChallenge(verifier);
			expect(verifyCodeChallenge(verifier, challenge)).toBe(true);
		});

		it('should reject empty verifier', () => {
			const challenge = generateCodeChallenge(generateCodeVerifier());
			expect(verifyCodeChallenge('', challenge, 'S256')).toBe(false);
		});

		it('should reject empty challenge', () => {
			const verifier = generateCodeVerifier();
			expect(verifyCodeChallenge(verifier, '', 'S256')).toBe(false);
		});
	});

	describe('plain method', () => {
		it('should verify matching verifier and challenge', () => {
			const verifier = generateCodeVerifier();
			expect(verifyCodeChallenge(verifier, verifier, 'plain')).toBe(true);
		});

		it('should reject non-matching verifier and challenge', () => {
			const verifier1 = generateCodeVerifier();
			const verifier2 = generateCodeVerifier();
			expect(verifyCodeChallenge(verifier1, verifier2, 'plain')).toBe(false);
		});

		it('should reject S256 challenge with plain method', () => {
			const verifier = generateCodeVerifier();
			const s256Challenge = generateCodeChallenge(verifier);
			// S256 challenge should not match verifier with plain method
			expect(verifyCodeChallenge(verifier, s256Challenge, 'plain')).toBe(false);
		});
	});

	describe('constant-time comparison', () => {
		it('should reject challenges of different lengths', () => {
			const verifier = generateCodeVerifier();
			const challenge = generateCodeChallenge(verifier);
			expect(verifyCodeChallenge(verifier, challenge + 'x', 'S256')).toBe(false);
		});

		it('should reject challenges with single character difference', () => {
			const verifier = generateCodeVerifier();
			const challenge = generateCodeChallenge(verifier);
			// Change first character
			const modifiedChallenge = 'X' + challenge.slice(1);
			expect(verifyCodeChallenge(verifier, modifiedChallenge, 'S256')).toBe(false);
		});
	});
});

describe('isValidCodeVerifier', () => {
	it('should accept valid verifiers', () => {
		expect(isValidCodeVerifier(generateCodeVerifier())).toBe(true);
	});

	it('should accept minimum length (43 chars)', () => {
		const verifier = 'a'.repeat(43);
		expect(isValidCodeVerifier(verifier)).toBe(true);
	});

	it('should accept maximum length (128 chars)', () => {
		const verifier = 'a'.repeat(128);
		expect(isValidCodeVerifier(verifier)).toBe(true);
	});

	it('should reject too short (< 43 chars)', () => {
		const verifier = 'a'.repeat(42);
		expect(isValidCodeVerifier(verifier)).toBe(false);
	});

	it('should reject too long (> 128 chars)', () => {
		const verifier = 'a'.repeat(129);
		expect(isValidCodeVerifier(verifier)).toBe(false);
	});

	it('should accept all unreserved URI characters', () => {
		// Test all valid characters: A-Z, a-z, 0-9, -, ., _, ~
		const validChars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~';
		const verifier = validChars.repeat(2).slice(0, 64); // Make 64 chars
		expect(isValidCodeVerifier(verifier)).toBe(true);
	});

	it('should reject invalid characters', () => {
		// Space
		expect(isValidCodeVerifier('a'.repeat(42) + ' ')).toBe(false);
		// Plus (used in base64 but not base64url)
		expect(isValidCodeVerifier('a'.repeat(42) + '+')).toBe(false);
		// Slash
		expect(isValidCodeVerifier('a'.repeat(42) + '/')).toBe(false);
		// Equals (padding)
		expect(isValidCodeVerifier('a'.repeat(42) + '=')).toBe(false);
	});

	it('should reject null/undefined/empty', () => {
		expect(isValidCodeVerifier('')).toBe(false);
		expect(isValidCodeVerifier(null as unknown as string)).toBe(false);
		expect(isValidCodeVerifier(undefined as unknown as string)).toBe(false);
	});
});

describe('isValidCodeChallenge', () => {
	it('should accept valid challenges', () => {
		const verifier = generateCodeVerifier();
		const challenge = generateCodeChallenge(verifier);
		expect(isValidCodeChallenge(challenge)).toBe(true);
	});

	it('should accept minimum length (43 chars)', () => {
		const challenge = 'a'.repeat(43);
		expect(isValidCodeChallenge(challenge)).toBe(true);
	});

	it('should accept maximum length (128 chars)', () => {
		const challenge = 'a'.repeat(128);
		expect(isValidCodeChallenge(challenge)).toBe(true);
	});

	it('should reject too short (< 43 chars)', () => {
		const challenge = 'a'.repeat(42);
		expect(isValidCodeChallenge(challenge)).toBe(false);
	});

	it('should reject too long (> 128 chars)', () => {
		const challenge = 'a'.repeat(129);
		expect(isValidCodeChallenge(challenge)).toBe(false);
	});

	it('should reject null/undefined/empty', () => {
		expect(isValidCodeChallenge('')).toBe(false);
		expect(isValidCodeChallenge(null as unknown as string)).toBe(false);
		expect(isValidCodeChallenge(undefined as unknown as string)).toBe(false);
	});
});

describe('PKCE flow integration', () => {
	it('should complete full PKCE flow', () => {
		// Client generates verifier and challenge
		const verifier = generateCodeVerifier();
		const challenge = generateCodeChallenge(verifier);

		// Client stores verifier locally
		// Client sends challenge to authorization server in auth request

		// Later, client sends verifier in token request
		// Server verifies the verifier matches the stored challenge
		expect(verifyCodeChallenge(verifier, challenge)).toBe(true);
	});

	it('should prevent code interception attack', () => {
		// Legitimate client generates verifier and challenge
		const legitimateVerifier = generateCodeVerifier();
		const challenge = generateCodeChallenge(legitimateVerifier);

		// Attacker intercepts auth code but doesn't have the verifier
		// Attacker tries to use their own verifier
		const attackerVerifier = generateCodeVerifier();

		// Server should reject attacker's verifier
		expect(verifyCodeChallenge(attackerVerifier, challenge)).toBe(false);
	});
});
