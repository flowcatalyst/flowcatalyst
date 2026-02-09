/**
 * Update Identity Provider Command
 */

import type { Command } from '@flowcatalyst/application';
import type { IdentityProviderType } from '../../../domain/index.js';

export interface UpdateIdentityProviderCommand extends Command {
  readonly identityProviderId: string;
  readonly name?: string | undefined;
  readonly type?: IdentityProviderType | undefined;
  readonly oidcIssuerUrl?: string | null | undefined;
  readonly oidcClientId?: string | null | undefined;
  readonly oidcClientSecretRef?: string | null | undefined;
  readonly oidcMultiTenant?: boolean | undefined;
  readonly oidcIssuerPattern?: string | null | undefined;
  readonly allowedEmailDomains?: string[] | undefined;
}
