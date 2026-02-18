/**
 * Block on Error Checker
 *
 * Checks if message groups are blocked due to FAILED status dispatch jobs.
 * For BLOCK_ON_ERROR mode, any error in a message group blocks further
 * dispatching for that group until the error is resolved.
 */

import { eq, and, inArray } from 'drizzle-orm';
import { dispatchJobs } from '@flowcatalyst/persistence';
import type { PostgresJsDatabase } from 'drizzle-orm/postgres-js';

export interface BlockOnErrorChecker {
  /** Check if a message group has any FAILED dispatch jobs. */
  isGroupBlocked(messageGroup: string): Promise<boolean>;
  /** Get the set of blocked groups from a list of groups. */
  getBlockedGroups(messageGroups: string[]): Promise<Set<string>>;
}

export function createBlockOnErrorChecker(db: PostgresJsDatabase): BlockOnErrorChecker {
  return {
    async isGroupBlocked(messageGroup) {
      const rows = await db
        .select({ id: dispatchJobs.id })
        .from(dispatchJobs)
        .where(and(eq(dispatchJobs.messageGroup, messageGroup), eq(dispatchJobs.status, 'FAILED')))
        .limit(1);

      return rows.length > 0;
    },

    async getBlockedGroups(messageGroups) {
      if (messageGroups.length === 0) return new Set();

      const rows = await db
        .selectDistinct({ messageGroup: dispatchJobs.messageGroup })
        .from(dispatchJobs)
        .where(
          and(inArray(dispatchJobs.messageGroup, messageGroups), eq(dispatchJobs.status, 'FAILED')),
        );

      return new Set(rows.map((r) => r.messageGroup).filter(Boolean) as string[]);
    },
  };
}
