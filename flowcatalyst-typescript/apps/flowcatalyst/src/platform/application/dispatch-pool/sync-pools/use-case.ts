/**
 * Sync Dispatch Pools Use Case
 *
 * Bulk create/update/archive anchor-level dispatch pools from SDK.
 */

import type { UseCase } from '@flowcatalyst/application';
import { Result, UseCaseError } from '@flowcatalyst/application';
import type { ExecutionContext, UnitOfWork } from '@flowcatalyst/domain-core';

import type { DispatchPoolRepository } from '../../../infrastructure/persistence/index.js';
import {
  createDispatchPool,
  updateDispatchPool,
  DispatchPoolsSynced,
} from '../../../domain/index.js';

import type { SyncDispatchPoolsCommand } from './command.js';

export interface SyncDispatchPoolsUseCaseDeps {
  readonly dispatchPoolRepository: DispatchPoolRepository;
  readonly unitOfWork: UnitOfWork;
}

const CODE_PATTERN = /^[a-z][a-z0-9_-]*$/;

export function createSyncDispatchPoolsUseCase(
  deps: SyncDispatchPoolsUseCaseDeps,
): UseCase<SyncDispatchPoolsCommand, DispatchPoolsSynced> {
  const { dispatchPoolRepository, unitOfWork } = deps;

  return {
    async execute(
      command: SyncDispatchPoolsCommand,
      context: ExecutionContext,
    ): Promise<Result<DispatchPoolsSynced>> {
      if (!command.pools || command.pools.length === 0) {
        return Result.failure(
          UseCaseError.validation('POOLS_REQUIRED', 'At least one pool must be provided'),
        );
      }

      // Validate all pool items and check for duplicates
      const seenCodes = new Set<string>();
      for (const item of command.pools) {
        if (!item.code || !item.code.trim()) {
          return Result.failure(UseCaseError.validation('CODE_REQUIRED', 'Pool code is required'));
        }
        if (!CODE_PATTERN.test(item.code)) {
          return Result.failure(
            UseCaseError.validation(
              'INVALID_CODE_FORMAT',
              'Pool code must start with a lowercase letter and contain only lowercase letters, numbers, hyphens, and underscores',
              { code: item.code },
            ),
          );
        }
        if (!item.name || !item.name.trim()) {
          return Result.failure(
            UseCaseError.validation('NAME_REQUIRED', 'Pool name is required', { code: item.code }),
          );
        }
        if (item.rateLimit !== undefined && item.rateLimit < 1) {
          return Result.failure(
            UseCaseError.validation('INVALID_RATE_LIMIT', 'Rate limit must be at least 1', {
              code: item.code,
            }),
          );
        }
        if (item.concurrency !== undefined && item.concurrency < 1) {
          return Result.failure(
            UseCaseError.validation('INVALID_CONCURRENCY', 'Concurrency must be at least 1', {
              code: item.code,
            }),
          );
        }
        if (seenCodes.has(item.code)) {
          return Result.failure(
            UseCaseError.validation('DUPLICATE_CODE', 'Duplicate pool code in sync request', {
              code: item.code,
            }),
          );
        }
        seenCodes.add(item.code);
      }

      let created = 0;
      let updated = 0;
      let deleted = 0;
      const syncedCodes: string[] = [];

      // Process each pool item (anchor-level: clientId = null)
      for (const item of command.pools) {
        const existing = await dispatchPoolRepository.findByCodeAndClientId(item.code, null);

        if (existing) {
          // Update existing
          const updatedPool = updateDispatchPool(existing, {
            name: item.name,
            description: item.description ?? null,
            rateLimit: item.rateLimit ?? 100,
            concurrency: item.concurrency ?? 10,
            status: 'ACTIVE',
          });
          await dispatchPoolRepository.update(updatedPool);
          updated++;
        } else {
          // Create new
          const pool = createDispatchPool({
            code: item.code,
            name: item.name,
            description: item.description ?? null,
            rateLimit: item.rateLimit ?? 100,
            concurrency: item.concurrency ?? 10,
          });
          await dispatchPoolRepository.insert(pool);
          created++;
        }

        syncedCodes.push(item.code);
      }

      // Remove unlisted pools if requested
      if (command.removeUnlisted) {
        const anchorPools = await dispatchPoolRepository.findAnchorLevel();
        for (const pool of anchorPools) {
          if (!seenCodes.has(pool.code) && pool.status !== 'ARCHIVED') {
            const archived = updateDispatchPool(pool, { status: 'ARCHIVED' });
            await dispatchPoolRepository.update(archived);
            deleted++;
          }
        }
      }

      const syncEvent = new DispatchPoolsSynced(context, {
        applicationCode: command.applicationCode,
        poolsCreated: created,
        poolsUpdated: updated,
        poolsDeleted: deleted,
        syncedPoolCodes: syncedCodes,
      });

      // Use synthetic aggregate to commit the sync event
      const syncResult = {
        id: command.applicationCode,
        createdAt: new Date(),
        updatedAt: new Date(),
      };

      return unitOfWork.commit(syncResult, syncEvent, command);
    },
  };
}
