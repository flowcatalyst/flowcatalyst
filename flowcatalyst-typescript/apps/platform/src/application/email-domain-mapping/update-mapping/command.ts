/**
 * Update Email Domain Mapping Command
 */

import type { Command } from '@flowcatalyst/application';
import type { ScopeType } from '../../../domain/index.js';

export interface UpdateEmailDomainMappingCommand extends Command {
  readonly emailDomainMappingId: string;
  readonly identityProviderId?: string | undefined;
  readonly scopeType?: ScopeType | undefined;
  readonly primaryClientId?: string | null | undefined;
  readonly additionalClientIds?: string[] | undefined;
  readonly grantedClientIds?: string[] | undefined;
  readonly requiredOidcTenantId?: string | null | undefined;
  readonly allowedRoleIds?: string[] | undefined;
  readonly syncRolesFromIdp?: boolean | undefined;
}
