import 'dotenv/config';
import { z } from 'zod/v4';

export { z } from 'zod/v4';

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
		const tree = z.treeifyError(result.error);
		const errors: string[] = [];

		if ('properties' in tree && tree.properties) {
			for (const [key, sub] of Object.entries(tree.properties)) {
				const node = sub as { errors?: string[] };
				if (node.errors?.length) {
					errors.push(`  ${key}: ${node.errors.join(', ')}`);
				}
			}
		}

		throw new Error(`Environment validation failed:\n${errors.join('\n')}`);
	}

	return result.data;
}

/**
 * Common environment variable schemas for reuse.
 *
 * Note: In zod v4, .default() on a transformed schema expects the OUTPUT type.
 * Use .prefault() to provide an INPUT default (applied before parsing), which
 * matches the zod v3 .default() behavior for transform chains.
 */
export const CommonEnvSchemas = {
	/** Log level enum */
	logLevel: z.enum(['trace', 'debug', 'info', 'warn', 'error', 'fatal']).default('info'),

	/** Port number */
	port: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.pipe(z.number().int().min(1).max(65535))
		.prefault('3000'),

	/** Boolean from string */
	boolean: z
		.string()
		.transform((v) => v === 'true' || v === '1')
		.prefault('false'),

	/** Positive integer from string */
	positiveInt: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.pipe(z.number().int().positive()),

	/** URL validation */
	url: z.url(),

	/** Optional URL */
	optionalUrl: z.url().optional(),

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
		.prefault(''),
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
