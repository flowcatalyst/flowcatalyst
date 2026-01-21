import 'dotenv/config';
import { z } from 'zod';

export { z } from 'zod';

/**
 * Parse environment variables with Zod schema validation.
 * Throws a descriptive error if validation fails.
 */
export function parseEnv<T extends z.ZodRawShape>(
	schema: z.ZodObject<T>,
	env: Record<string, string | undefined> = process.env,
): z.infer<z.ZodObject<T>> {
	const result = schema.safeParse(env);

	if (!result.success) {
		const formatted = result.error.format();
		const errors: string[] = [];

		for (const [key, value] of Object.entries(formatted)) {
			if (key === '_errors') continue;
			const fieldErrors = value as { _errors?: string[] };
			if (fieldErrors._errors?.length) {
				errors.push(`  ${key}: ${fieldErrors._errors.join(', ')}`);
			}
		}

		throw new Error(`Environment validation failed:\n${errors.join('\n')}`);
	}

	return result.data;
}

/**
 * Common environment variable schemas for reuse
 */
export const CommonEnvSchemas = {
	/** Log level enum */
	logLevel: z.enum(['trace', 'debug', 'info', 'warn', 'error', 'fatal']).default('info'),

	/** Port number */
	port: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.pipe(z.number().int().min(1).max(65535))
		.default('3000'),

	/** Boolean from string */
	boolean: z
		.string()
		.transform((v) => v === 'true' || v === '1')
		.default('false'),

	/** Positive integer from string */
	positiveInt: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.pipe(z.number().int().positive()),

	/** URL validation */
	url: z.string().url(),

	/** Optional URL */
	optionalUrl: z.string().url().optional(),

	/** AWS region */
	awsRegion: z.string().default('us-east-1'),

	/** Duration in milliseconds from string */
	durationMs: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.pipe(z.number().int().min(0)),

	/** Comma-separated list to array */
	stringArray: z
		.string()
		.transform((v) => v.split(',').map((s) => s.trim()))
		.default(''),
};

/**
 * Create a configuration object from environment schema.
 * This is a convenience wrapper around parseEnv.
 */
export function createConfig<T extends z.ZodRawShape>(schema: z.ZodObject<T>) {
	return parseEnv(schema);
}

/**
 * Type helper to extract config type from schema
 */
export type ConfigType<T extends z.ZodObject<z.ZodRawShape>> = z.infer<T>;
