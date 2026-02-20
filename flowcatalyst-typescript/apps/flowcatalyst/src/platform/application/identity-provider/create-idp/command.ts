/**
 * Create Identity Provider Command
 */

import type { Command } from "@flowcatalyst/application";
import type { IdentityProviderType } from "../../../domain/index.js";

export interface CreateIdentityProviderCommand extends Command {
	readonly code: string;
	readonly name: string;
	readonly type: IdentityProviderType;
	readonly oidcIssuerUrl?: string | null | undefined;
	readonly oidcClientId?: string | null | undefined;
	readonly oidcClientSecretRef?: string | null | undefined;
	readonly oidcMultiTenant?: boolean | undefined;
	readonly oidcIssuerPattern?: string | null | undefined;
	readonly allowedEmailDomains?: string[] | undefined;
}
