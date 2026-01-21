/**
 * Update OAuth Client Command
 */

import type { Command } from '@flowcatalyst/application';
import type { OAuthGrantType } from '../../../domain/index.js';

/**
 * Command to update an OAuth client.
 */
export interface UpdateOAuthClientCommand extends Command {
	readonly oauthClientId: string;
	readonly clientName?: string | undefined;
	readonly redirectUris?: string[] | undefined;
	readonly allowedOrigins?: string[] | undefined;
	readonly grantTypes?: OAuthGrantType[] | undefined;
	readonly defaultScopes?: string | null | undefined;
	readonly pkceRequired?: boolean | undefined;
	readonly applicationIds?: string[] | undefined;
	readonly active?: boolean | undefined;
}

/**
 * Command to regenerate an OAuth client secret.
 */
export interface RegenerateOAuthClientSecretCommand extends Command {
	readonly oauthClientId: string;
	readonly newSecretRef: string;
}
