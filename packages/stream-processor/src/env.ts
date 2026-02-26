import { parseEnv, z } from "@flowcatalyst/config";

export const envSchema = z.object({
	// Database
	DATABASE_URL: z.string(),

	// General
	NODE_ENV: z
		.enum(["development", "production", "test"])
		.default("development"),
	LOG_LEVEL: z
		.enum(["trace", "debug", "info", "warn", "error", "fatal"])
		.default("info"),

	// Event Projection Service
	STREAM_PROCESSOR_EVENTS_ENABLED: z
		.string()
		.transform((v) => v === "true")
		.prefault("true"),
	STREAM_PROCESSOR_EVENTS_BATCH_SIZE: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.prefault("100"),

	// Dispatch Job Projection Service
	STREAM_PROCESSOR_DISPATCH_JOBS_ENABLED: z
		.string()
		.transform((v) => v === "true")
		.prefault("true"),
	STREAM_PROCESSOR_DISPATCH_JOBS_BATCH_SIZE: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.prefault("100"),
});

export type Env = z.infer<typeof envSchema>;

export const env = parseEnv(envSchema);
