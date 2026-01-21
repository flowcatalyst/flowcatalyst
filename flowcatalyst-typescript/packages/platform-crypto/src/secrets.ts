/**
 * Secret Service - Multi-provider secret resolution
 *
 * Supports multiple secret backends:
 * - `encrypted:BASE64` - Local AES-256-GCM encrypted values
 * - `aws-sm://secret-name` - AWS Secrets Manager (requires AWS SDK)
 * - `aws-ps://parameter-name` - AWS Parameter Store (requires AWS SDK)
 * - `vault://path/to/secret#key` - HashiCorp Vault
 *
 * Reference format determines which provider handles the secret.
 */

import { Result, ok, err } from 'neverthrow';
import { EncryptionService } from './encryption.js';

export type SecretError =
	| { type: 'not_configured'; provider: string; message: string }
	| { type: 'not_found'; reference: string; message: string }
	| { type: 'resolution_failed'; reference: string; message: string; cause?: Error | undefined }
	| { type: 'unknown_provider'; reference: string; message: string };

/**
 * Secret provider interface - implement this for each backend
 */
export interface SecretProvider {
	/** Check if this provider can handle the given reference */
	canHandle(reference: string): boolean;

	/** Get the provider type name */
	getProviderType(): string;

	/** Resolve a secret reference to its plaintext value */
	resolve(reference: string): Promise<Result<string, SecretError>>;

	/** Validate that a reference can be resolved (without returning the value) */
	validate(reference: string): Promise<Result<void, SecretError>>;
}

/**
 * Local encrypted secret provider using AES-256-GCM
 */
export class EncryptedSecretProvider implements SecretProvider {
	private static readonly ENCRYPTED_PREFIX = 'encrypted:';
	private static readonly PLAINTEXT_PREFIX = 'encrypt:';

	constructor(private encryptionService: EncryptionService) {}

	canHandle(reference: string): boolean {
		return reference.startsWith(EncryptedSecretProvider.ENCRYPTED_PREFIX);
	}

	getProviderType(): string {
		return 'encrypted';
	}

	async resolve(reference: string): Promise<Result<string, SecretError>> {
		const result = this.encryptionService.decrypt(reference);
		if (result.isErr()) {
			return err({
				type: 'resolution_failed',
				reference: 'encrypted:***',
				message: result.error.message,
			});
		}
		return ok(result.value);
	}

	async validate(reference: string): Promise<Result<void, SecretError>> {
		const result = await this.resolve(reference);
		if (result.isErr()) {
			return err(result.error);
		}
		return ok(undefined);
	}

	/**
	 * Encrypt a plaintext value and return an encrypted reference.
	 *
	 * @param plaintext - The plaintext value to encrypt
	 * @returns Result with `encrypted:BASE64` format string, or error
	 */
	encryptReference(plaintext: string): Result<string, SecretError> {
		const result = this.encryptionService.encrypt(plaintext);
		if (result.isErr()) {
			return err({
				type: 'resolution_failed',
				reference: '***',
				message: result.error.message,
			});
		}
		return ok(result.value);
	}

	/**
	 * Check if a reference is a plaintext reference (needs encryption).
	 *
	 * Plaintext references start with `encrypt:` prefix.
	 *
	 * @param reference - The reference to check
	 * @returns true if the reference needs encryption
	 */
	isPlaintextReference(reference: string): boolean {
		return reference.startsWith(EncryptedSecretProvider.PLAINTEXT_PREFIX);
	}

	/**
	 * Prepare a value for storage, encrypting if needed.
	 *
	 * - If value starts with `encrypt:`, encrypt the plaintext after the prefix
	 * - If value starts with `encrypted:`, return as-is
	 * - Otherwise, return as-is (not an encrypted reference)
	 *
	 * @param reference - The reference to prepare
	 * @returns Result with storage-ready value, or error
	 */
	async prepareForStorage(reference: string): Promise<Result<string, SecretError>> {
		if (this.isPlaintextReference(reference)) {
			const plaintext = reference.slice(EncryptedSecretProvider.PLAINTEXT_PREFIX.length);
			return this.encryptReference(plaintext);
		}
		return ok(reference);
	}
}

/**
 * AWS Secrets Manager provider (placeholder - requires @aws-sdk/client-secrets-manager)
 *
 * Reference format: `aws-sm://secret-name`
 */
export class AwsSecretsManagerProvider implements SecretProvider {
	private enabled: boolean;

	constructor(enabled: boolean = false) {
		this.enabled = enabled;
	}

	canHandle(reference: string): boolean {
		return reference.startsWith('aws-sm://');
	}

	getProviderType(): string {
		return 'aws-secrets-manager';
	}

	async resolve(reference: string): Promise<Result<string, SecretError>> {
		if (!this.enabled) {
			return err({
				type: 'not_configured',
				provider: 'aws-secrets-manager',
				message: 'AWS Secrets Manager is not enabled. Set FLOWCATALYST_AWS_SM_ENABLED=true',
			});
		}

		const secretName = reference.slice('aws-sm://'.length);

		try {
			// Dynamic import to avoid requiring AWS SDK
			const { SecretsManagerClient, GetSecretValueCommand } = await import('@aws-sdk/client-secrets-manager');
			const client = new SecretsManagerClient({});
			const command = new GetSecretValueCommand({ SecretId: secretName });
			const response = await client.send(command);

			if (response.SecretString) {
				return ok(response.SecretString);
			}
			if (response.SecretBinary) {
				return ok(Buffer.from(response.SecretBinary).toString('utf8'));
			}
			return err({
				type: 'not_found',
				reference: `aws-sm://${secretName}`,
				message: 'Secret has no value',
			});
		} catch (e) {
			return err({
				type: 'resolution_failed',
				reference: `aws-sm://${secretName}`,
				message: `Failed to resolve secret: ${e instanceof Error ? e.message : String(e)}`,
				cause: e instanceof Error ? e : undefined,
			});
		}
	}

	async validate(reference: string): Promise<Result<void, SecretError>> {
		const result = await this.resolve(reference);
		if (result.isErr()) {
			return err(result.error);
		}
		return ok(undefined);
	}
}

/**
 * AWS Parameter Store provider (placeholder - requires @aws-sdk/client-ssm)
 *
 * Reference format: `aws-ps://parameter-name`
 */
export class AwsParameterStoreProvider implements SecretProvider {
	private enabled: boolean;

	constructor(enabled: boolean = false) {
		this.enabled = enabled;
	}

	canHandle(reference: string): boolean {
		return reference.startsWith('aws-ps://');
	}

	getProviderType(): string {
		return 'aws-parameter-store';
	}

	async resolve(reference: string): Promise<Result<string, SecretError>> {
		if (!this.enabled) {
			return err({
				type: 'not_configured',
				provider: 'aws-parameter-store',
				message: 'AWS Parameter Store is not enabled. Set FLOWCATALYST_AWS_PS_ENABLED=true',
			});
		}

		const parameterName = reference.slice('aws-ps://'.length);

		try {
			// Dynamic import to avoid requiring AWS SDK
			const { SSMClient, GetParameterCommand } = await import('@aws-sdk/client-ssm');
			const client = new SSMClient({});
			const command = new GetParameterCommand({ Name: parameterName, WithDecryption: true });
			const response = await client.send(command);

			if (!response.Parameter?.Value) {
				return err({
					type: 'not_found',
					reference: `aws-ps://${parameterName}`,
					message: 'Parameter has no value',
				});
			}
			return ok(response.Parameter.Value);
		} catch (e) {
			return err({
				type: 'resolution_failed',
				reference: `aws-ps://${parameterName}`,
				message: `Failed to resolve parameter: ${e instanceof Error ? e.message : String(e)}`,
				cause: e instanceof Error ? e : undefined,
			});
		}
	}

	async validate(reference: string): Promise<Result<void, SecretError>> {
		const result = await this.resolve(reference);
		if (result.isErr()) {
			return err(result.error);
		}
		return ok(undefined);
	}
}

/**
 * HashiCorp Vault provider
 *
 * Reference format: `vault://path/to/secret#key`
 */
export class VaultSecretProvider implements SecretProvider {
	private enabled: boolean;
	private address: string;
	private token: string | undefined;

	constructor(config: { enabled?: boolean | undefined; address?: string | undefined; token?: string | undefined } = {}) {
		this.enabled = config.enabled ?? false;
		this.address = config.address ?? process.env['VAULT_ADDR'] ?? 'http://127.0.0.1:8200';
		this.token = config.token ?? process.env['VAULT_TOKEN'];
	}

	canHandle(reference: string): boolean {
		return reference.startsWith('vault://');
	}

	getProviderType(): string {
		return 'vault';
	}

	async resolve(reference: string): Promise<Result<string, SecretError>> {
		if (!this.enabled) {
			return err({
				type: 'not_configured',
				provider: 'vault',
				message: 'HashiCorp Vault is not enabled. Set FLOWCATALYST_VAULT_ENABLED=true',
			});
		}

		if (!this.token) {
			return err({
				type: 'not_configured',
				provider: 'vault',
				message: 'Vault token not configured. Set VAULT_TOKEN environment variable.',
			});
		}

		const { path, key } = this.parseReference(reference);

		try {
			const url = `${this.address}/v1/${path}`;
			const response = await fetch(url, {
				headers: { 'X-Vault-Token': this.token },
			});

			if (!response.ok) {
				return err({
					type: 'resolution_failed',
					reference: `vault://${path}#${key}`,
					message: `Vault returned ${response.status}: ${response.statusText}`,
				});
			}

			const data = (await response.json()) as { data?: { data?: Record<string, string> } };
			const value = data?.data?.data?.[key];

			if (value === undefined) {
				return err({
					type: 'not_found',
					reference: `vault://${path}#${key}`,
					message: `Key '${key}' not found in secret`,
				});
			}

			return ok(value);
		} catch (e) {
			return err({
				type: 'resolution_failed',
				reference: `vault://${path}#${key}`,
				message: `Failed to resolve secret: ${e instanceof Error ? e.message : String(e)}`,
				cause: e instanceof Error ? e : undefined,
			});
		}
	}

	private parseReference(reference: string): { path: string; key: string } {
		const withoutPrefix = reference.slice('vault://'.length);
		const hashIndex = withoutPrefix.indexOf('#');

		if (hashIndex === -1) {
			return { path: withoutPrefix, key: 'value' };
		}

		return {
			path: withoutPrefix.slice(0, hashIndex),
			key: withoutPrefix.slice(hashIndex + 1),
		};
	}

	async validate(reference: string): Promise<Result<void, SecretError>> {
		const result = await this.resolve(reference);
		if (result.isErr()) {
			return err(result.error);
		}
		return ok(undefined);
	}
}

/**
 * Secret Service - manages multiple providers
 */
export class SecretService {
	private providers: SecretProvider[] = [];
	private encryptedProvider: EncryptedSecretProvider | null = null;

	/**
	 * Register a secret provider
	 */
	registerProvider(provider: SecretProvider): void {
		this.providers.push(provider);
		if (provider instanceof EncryptedSecretProvider) {
			this.encryptedProvider = provider;
		}
	}

	/**
	 * Resolve a secret reference using the appropriate provider
	 */
	async resolve(reference: string): Promise<Result<string, SecretError>> {
		for (const provider of this.providers) {
			if (provider.canHandle(reference)) {
				return provider.resolve(reference);
			}
		}

		return err({
			type: 'unknown_provider',
			reference: this.maskReference(reference),
			message: 'No provider found for reference format',
		});
	}

	/**
	 * Resolve a secret reference, returning null if the reference is null/undefined.
	 *
	 * @param reference - The secret reference, or null/undefined
	 * @returns Result with resolved value (or null if input was null), or error
	 */
	async resolveOptional(reference: string | null | undefined): Promise<Result<string | null, SecretError>> {
		if (reference === null || reference === undefined) {
			return ok(null);
		}
		return this.resolve(reference);
	}

	/**
	 * Prepare a value for storage, encrypting if needed.
	 *
	 * This delegates to the EncryptedSecretProvider if available.
	 * - If value starts with `encrypt:`, encrypt the plaintext after the prefix
	 * - If value starts with `encrypted:`, return as-is
	 * - Otherwise, return as-is (not an encrypted reference)
	 *
	 * @param reference - The reference to prepare
	 * @returns Result with storage-ready value, or error
	 */
	async prepareForStorage(reference: string): Promise<Result<string, SecretError>> {
		if (this.encryptedProvider) {
			return this.encryptedProvider.prepareForStorage(reference);
		}

		// If no encrypted provider, just pass through
		return ok(reference);
	}

	/**
	 * Validate a secret reference without returning its value
	 */
	async validate(reference: string): Promise<Result<void, SecretError>> {
		for (const provider of this.providers) {
			if (provider.canHandle(reference)) {
				return provider.validate(reference);
			}
		}

		return err({
			type: 'unknown_provider',
			reference: this.maskReference(reference),
			message: 'No provider found for reference format',
		});
	}

	/**
	 * Check if a reference format is recognized by any provider
	 */
	isValidFormat(reference: string): boolean {
		return this.providers.some((p) => p.canHandle(reference));
	}

	/**
	 * Get the provider type for a reference
	 */
	getProviderType(reference: string): string | null {
		const provider = this.providers.find((p) => p.canHandle(reference));
		return provider?.getProviderType() ?? null;
	}

	/**
	 * Mask a reference for safe logging
	 */
	private maskReference(reference: string): string {
		if (reference.startsWith('encrypted:')) return 'encrypted:***';
		if (reference.startsWith('aws-sm://')) return 'aws-sm://***';
		if (reference.startsWith('aws-ps://')) return 'aws-ps://***';
		if (reference.startsWith('vault://')) return 'vault://***';
		return '***';
	}
}

/**
 * Create a SecretService with all providers configured from environment
 */
export function createSecretServiceFromEnv(encryptionService: EncryptionService): SecretService {
	const service = new SecretService();

	// Always register encrypted provider (local fallback)
	service.registerProvider(new EncryptedSecretProvider(encryptionService));

	// AWS Secrets Manager
	const awsSmEnabled = process.env['FLOWCATALYST_AWS_SM_ENABLED'] === 'true';
	service.registerProvider(new AwsSecretsManagerProvider(awsSmEnabled));

	// AWS Parameter Store
	const awsPsEnabled = process.env['FLOWCATALYST_AWS_PS_ENABLED'] === 'true';
	service.registerProvider(new AwsParameterStoreProvider(awsPsEnabled));

	// HashiCorp Vault
	const vaultEnabled = process.env['FLOWCATALYST_VAULT_ENABLED'] === 'true';
	service.registerProvider(
		new VaultSecretProvider({
			enabled: vaultEnabled,
		}),
	);

	return service;
}
