/**
 * Sync Roles Use Case
 *
 * Syncs roles from an application SDK. Creates new ones, updates existing
 * SDK-sourced ones, and optionally removes unlisted SDK-sourced ones.
 * CODE and DATABASE-sourced roles are never modified.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, UseCaseError } from '@flowcatalyst/application';
import type { ExecutionContext, UnitOfWork } from '@flowcatalyst/domain-core';
import type { TransactionContext } from '@flowcatalyst/persistence';

import type {
  RoleRepository,
  ApplicationRepository,
} from '../../../infrastructure/persistence/index.js';
import {
  createAuthRole,
  updateAuthRole,
  RoleSource,
  RolesSynced,
} from '../../../domain/index.js';

import type { SyncRolesCommand } from './command.js';

export interface SyncRolesUseCaseDeps {
  readonly roleRepository: RoleRepository;
  readonly applicationRepository: ApplicationRepository;
  readonly unitOfWork: UnitOfWork;
}

export function createSyncRolesUseCase(
  deps: SyncRolesUseCaseDeps,
): UseCase<SyncRolesCommand, RolesSynced> {
  const { roleRepository, applicationRepository, unitOfWork } = deps;

  return {
    async execute(
      command: SyncRolesCommand,
      context: ExecutionContext,
    ): Promise<Result<RolesSynced>> {
      const appCodeResult = validateRequired(
        command.applicationCode,
        'applicationCode',
        'APPLICATION_CODE_REQUIRED',
      );
      if (Result.isFailure(appCodeResult)) return appCodeResult;

      if (!command.roles || command.roles.length === 0) {
        return Result.failure(
          UseCaseError.validation('ROLES_REQUIRED', 'At least one role must be provided'),
        );
      }

      // Look up the application to get its ID
      const application = await applicationRepository.findByCode(command.applicationCode);
      if (!application) {
        return Result.failure(
          UseCaseError.notFound(
            'APPLICATION_NOT_FOUND',
            `Application not found: ${command.applicationCode}`,
          ),
        );
      }

      let created = 0;
      let updated = 0;
      let deleted = 0;
      const syncedNames: string[] = [];

      const eventData = {
        applicationCode: command.applicationCode,
        rolesCreated: 0,
        rolesUpdated: 0,
        rolesDeleted: 0,
        syncedRoleNames: [] as string[],
      };

      const event = new RolesSynced(context, eventData);

      return unitOfWork.commitOperations(event, command, async (tx) => {
        const txCtx = tx as TransactionContext;

        // Process each role item
        for (const item of command.roles) {
          const fullName = `${command.applicationCode}:${item.name.toLowerCase()}`;
          syncedNames.push(fullName);

          const existing = await roleRepository.findByName(fullName, txCtx);

          if (!existing) {
            // Create new SDK-sourced role
            const newRole = createAuthRole({
              applicationId: application.id,
              applicationCode: command.applicationCode,
              shortName: item.name,
              displayName: item.displayName ?? item.name,
              description: item.description ?? null,
              permissions: item.permissions ?? [],
              source: RoleSource.SDK,
              clientManaged: item.clientManaged ?? false,
            });
            await roleRepository.insert(newRole, txCtx);
            created++;
          } else if (existing.source === RoleSource.SDK) {
            // Update existing SDK-sourced role
            const updatedRole = updateAuthRole(existing, {
              displayName: item.displayName ?? item.name,
              description: item.description ?? null,
              permissions: item.permissions ?? existing.permissions,
              clientManaged: item.clientManaged ?? existing.clientManaged,
            });
            await roleRepository.update(updatedRole, txCtx);
            updated++;
          }
          // Skip CODE and DATABASE-sourced roles
        }

        // Remove unlisted SDK-sourced roles for this application
        if (command.removeUnlisted) {
          const appRoles = await roleRepository.findByApplicationId(application.id, txCtx);
          for (const role of appRoles) {
            if (role.source === RoleSource.SDK && !syncedNames.includes(role.name)) {
              await roleRepository.deleteById(role.id, txCtx);
              deleted++;
            }
          }
        }

        // Update event data with final counts (mutate the same object reference held by the event)
        eventData.rolesCreated = created;
        eventData.rolesUpdated = updated;
        eventData.rolesDeleted = deleted;
        eventData.syncedRoleNames = syncedNames;
      });
    },
  };
}
