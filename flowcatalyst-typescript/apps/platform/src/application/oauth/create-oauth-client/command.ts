/**
 * Create OAuth Client Command
 */

import type { Command } from '@flowcatalyst/application';
import type { OAuthClientType, OAuthGrantType } from '../../../domain/index.js';

/**
 * Command to create an OAuth client.
 */
export interface CreateOAuthClientCommand extends Command {
	readonly clientId: string;
	readonly clientName: string;
	readonly clientType: OAuthClientType;
	readonly clientSecretRef?: string | null | undefined;
	readonly redirectUris?: string[] | undefined;
	readonly allowedOrigins?: string[] | undefined;
	readonly grantTypes?: OAuthGrantType[] | undefined;
	readonly defaultScopes?: string | null | undefined;
	readonly pkceRequired?: boolean | undefined;
	readonly applicationIds?: string[] | undefined;
}
