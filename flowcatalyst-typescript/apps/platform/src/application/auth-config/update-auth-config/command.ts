/**
 * Update Auth Config Command
 */

import type { Command } from '@flowcatalyst/application';
import type { AuthConfigType } from '../../../domain/index.js';

/**
 * Command to update an auth config's OIDC settings.
 */
export interface UpdateOidcSettingsCommand extends Command {
	readonly authConfigId: string;
	readonly oidcIssuerUrl: string;
	readonly oidcClientId: string;
	readonly oidcClientSecretRef?: string | null | undefined;
	readonly oidcMultiTenant?: boolean | undefined;
	readonly oidcIssuerPattern?: string | null | undefined;
}

/**
 * Command to update an auth config's config type.
 */
export interface UpdateConfigTypeCommand extends Command {
	readonly authConfigId: string;
	readonly configType: AuthConfigType;
	readonly primaryClientId?: string | null | undefined;
}

/**
 * Command to update additional clients for a CLIENT type config.
 */
export interface UpdateAdditionalClientsCommand extends Command {
	readonly authConfigId: string;
	readonly additionalClientIds: string[];
}

/**
 * Command to update granted clients for a PARTNER type config.
 */
export interface UpdateGrantedClientsCommand extends Command {
	readonly authConfigId: string;
	readonly grantedClientIds: string[];
}
