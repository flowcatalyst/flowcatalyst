/**
 * Environment Configuration
 *
 * Loads and validates environment variables for the platform service.
 */

import { z } from 'zod';

const envSchema = z.object({
	// Server
	PORT: z.coerce.number().default(3000),
	HOST: z.string().default('0.0.0.0'),
	NODE_ENV: z.enum(['development', 'production', 'test']).default('development'),

	// Database
	DATABASE_URL: z.string().default('postgres://localhost:5432/flowcatalyst'),

	// Logging
	LOG_LEVEL: z.enum(['trace', 'debug', 'info', 'warn', 'error', 'fatal']).default('info'),
	LOG_PRETTY: z
		.string()
		.transform((v) => v === 'true')
		.default('true'),

	// Auth / OIDC
	JWT_SECRET: z.string().optional(),
	JWT_ISSUER: z.string().default('flowcatalyst'),
	JWT_AUDIENCE: z.string().default('flowcatalyst'),

	// External base URL (for OAuth callbacks behind a proxy)
	EXTERNAL_BASE_URL: z.string().optional(),

	// OIDC Provider configuration
	OIDC_ISSUER: z.string().optional(), // Defaults to EXTERNAL_BASE_URL or http://localhost:PORT
	OIDC_COOKIES_KEYS: z
		.string()
		.optional()
		.transform((v) => (v ? v.split(',') : undefined)), // Comma-separated cookie signing keys

	// Encryption key for secrets (Base64-encoded 32-byte key)
	FLOWCATALYST_APP_KEY: z.string().optional(),

	// OIDC token expiry (in seconds)
	OIDC_ACCESS_TOKEN_TTL: z.coerce.number().default(3600), // 1 hour
	OIDC_ID_TOKEN_TTL: z.coerce.number().default(3600), // 1 hour
	OIDC_REFRESH_TOKEN_TTL: z.coerce.number().default(2592000), // 30 days
	OIDC_SESSION_TTL: z.coerce.number().default(86400), // 24 hours
	OIDC_AUTH_CODE_TTL: z.coerce.number().default(600), // 10 minutes
});

export type Env = z.infer<typeof envSchema>;

let cachedEnv: Env | null = null;

export function getEnv(): Env {
	if (!cachedEnv) {
		cachedEnv = envSchema.parse(process.env);
	}
	return cachedEnv;
}

export function isDevelopment(): boolean {
	return getEnv().NODE_ENV === 'development';
}

export function isProduction(): boolean {
	return getEnv().NODE_ENV === 'production';
}

export function isTest(): boolean {
	return getEnv().NODE_ENV === 'test';
}
