/**
 * Secret Provider Implementations
 *
 * Cloud-specific providers are opt-in and live here in the app layer so that
 * cloud SDKs are only loaded when the feature is actually configured.
 *
 * Supported providers (set via DB_SECRET_PROVIDER):
 *   env  — DATABASE_URL environment variable (default, no rotation support)
 *   aws  — AWS Secrets Manager  (requires DB_SECRET_ARN)
 *   gcp  — GCP Secret Manager   (requires DB_SECRET_NAME)
 */

import type { SecretProvider } from "@flowcatalyst/persistence";
import { parseSecretToDbUrl } from "@flowcatalyst/persistence";

// ---------------------------------------------------------------------------
// Env provider (default — plain DATABASE_URL, no rotation)
// ---------------------------------------------------------------------------

export function createEnvSecretProvider(databaseUrl: string): SecretProvider {
	return {
		name: "env",
		async getDbUrl() {
			return databaseUrl;
		},
	};
}

// ---------------------------------------------------------------------------
// AWS Secrets Manager provider
// ---------------------------------------------------------------------------

export interface AwsSecretProviderConfig {
	/** Full ARN or secret name. */
	secretArn: string;
	/** AWS region — defaults to AWS_REGION env var or eu-west-1. */
	region?: string | undefined;
}

export function createAwsSecretProvider(
	config: AwsSecretProviderConfig,
): SecretProvider {
	const { secretArn, region = process.env["AWS_REGION"] ?? "eu-west-1" } =
		config;

	return {
		name: "aws-secrets-manager",
		async getDbUrl() {
			const { SecretsManagerClient, GetSecretValueCommand } = await import(
				"@aws-sdk/client-secrets-manager"
			);
			const client = new SecretsManagerClient({ region });
			const response = await client.send(
				new GetSecretValueCommand({ SecretId: secretArn }),
			);
			if (!response.SecretString) {
				throw new Error(
					`AWS Secrets Manager secret ${secretArn} has no SecretString value`,
				);
			}
			// AWS RDS managed secrets omit dbname — fall back to DB_NAME env var.
			return parseSecretToDbUrl(
				response.SecretString,
				process.env["DB_NAME"],
			);
		},
	};
}

// ---------------------------------------------------------------------------
// GCP Secret Manager provider
// ---------------------------------------------------------------------------

export interface GcpSecretProviderConfig {
	/**
	 * Full resource name of the secret version.
	 * Format: projects/{project}/secrets/{secret}/versions/{version|latest}
	 */
	secretName: string;
}

export function createGcpSecretProvider(
	config: GcpSecretProviderConfig,
): SecretProvider {
	const { secretName } = config;

	return {
		name: "gcp-secret-manager",
		async getDbUrl() {
			const { SecretManagerServiceClient } = await import(
				"@google-cloud/secret-manager"
			);
			const client = new SecretManagerServiceClient();
			const [version] = await client.accessSecretVersion({ name: secretName });
			const payload = version.payload?.data;
			if (!payload) {
				throw new Error(
					`GCP Secret Manager secret ${secretName} returned empty payload`,
				);
			}
			const raw =
				typeof payload === "string"
					? payload
					: Buffer.from(payload as Uint8Array).toString("utf8");
			return parseSecretToDbUrl(raw);
		},
	};
}

// ---------------------------------------------------------------------------
// Factory — reads DB_SECRET_PROVIDER and related env vars
// ---------------------------------------------------------------------------

export interface SecretProviderEnvConfig {
	/** Explicit DATABASE_URL to use when provider is 'env'. */
	databaseUrl: string;
}

/**
 * Build the correct SecretProvider from environment variables.
 *
 *   DB_SECRET_PROVIDER=env|aws|gcp   (default: env)
 *   DB_SECRET_ARN=<arn>              (aws only)
 *   DB_SECRET_REGION=<region>        (aws only, default: AWS_REGION)
 *   DB_SECRET_NAME=<resource-name>   (gcp only)
 */
export function createSecretProviderFromEnv(
	config: SecretProviderEnvConfig,
): SecretProvider {
	const provider = (process.env["DB_SECRET_PROVIDER"] ?? "env").toLowerCase();

	switch (provider) {
		case "aws": {
			const secretArn = process.env["DB_SECRET_ARN"];
			if (!secretArn) {
				throw new Error(
					"DB_SECRET_PROVIDER=aws requires DB_SECRET_ARN to be set",
				);
			}
			return createAwsSecretProvider({
				secretArn,
				region: process.env["DB_SECRET_REGION"],
			});
		}

		case "gcp": {
			const secretName = process.env["DB_SECRET_NAME"];
			if (!secretName) {
				throw new Error(
					"DB_SECRET_PROVIDER=gcp requires DB_SECRET_NAME to be set " +
						"(format: projects/{project}/secrets/{secret}/versions/latest)",
				);
			}
			return createGcpSecretProvider({ secretName });
		}

		case "env":
		default:
			return createEnvSecretProvider(config.databaseUrl);
	}
}
