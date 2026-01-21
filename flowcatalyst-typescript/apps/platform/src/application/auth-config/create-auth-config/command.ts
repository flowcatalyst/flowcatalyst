/**
 * Create Auth Config Command
 */

import type { Command } from '@flowcatalyst/application';
import type { AuthConfigType } from '../../../domain/index.js';

/**
 * Command to create an INTERNAL auth config.
 */
export interface CreateInternalAuthConfigCommand extends Command {
	readonly emailDomain: string;
	readonly configType: AuthConfigType;
	readonly primaryClientId?: string | null | undefined;
	readonly additionalClientIds?: string[] | undefined;
	readonly grantedClientIds?: string[] | undefined;
}

/**
 * Command to create an OIDC auth config.
 */
export interface CreateOidcAuthConfigCommand extends Command {
	readonly emailDomain: string;
	readonly configType: AuthConfigType;
	readonly primaryClientId?: string | null | undefined;
	readonly additionalClientIds?: string[] | undefined;
	readonly grantedClientIds?: string[] | undefined;
	readonly oidcIssuerUrl: string;
	readonly oidcClientId: string;
	readonly oidcClientSecretRef?: string | null | undefined;
	readonly oidcMultiTenant?: boolean | undefined;
	readonly oidcIssuerPattern?: string | null | undefined;
}
