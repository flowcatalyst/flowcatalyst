/**
 * Create Email Domain Mapping Command
 */

import type { Command } from '@flowcatalyst/application';
import type { ScopeType } from '../../../domain/index.js';

export interface CreateEmailDomainMappingCommand extends Command {
  readonly emailDomain: string;
  readonly identityProviderId: string;
  readonly scopeType: ScopeType;
  readonly primaryClientId?: string | null | undefined;
  readonly additionalClientIds?: string[] | undefined;
  readonly grantedClientIds?: string[] | undefined;
  readonly requiredOidcTenantId?: string | null | undefined;
  readonly allowedRoleIds?: string[] | undefined;
  readonly syncRolesFromIdp?: boolean | undefined;
}
