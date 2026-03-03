/**
 * Event Dispatch Service
 *
 * Builds dispatch jobs for events within the UoW transaction.
 * When a domain event is persisted, this service finds matching active subscriptions
 * and creates dispatch jobs + outbox entries in the same transaction.
 *
 * Returns DispatchJobNotification[] so the UnitOfWork can push
 * MessagePointers to a queue after the transaction commits.
 *
 * Connection lookups go through ConnectionCache (in-memory) to avoid
 * per-event DB queries on the hot path (targeting 10k events/sec).
 */

import type { DomainEvent } from "@flowcatalyst/domain";
import { generateRaw } from "@flowcatalyst/tsid";
import type { PostgresJsDatabase } from "drizzle-orm/postgres-js";
import {
	dispatchJobs,
	dispatchJobProjectionFeed,
} from "@flowcatalyst/persistence";

import type { SubscriptionRepository } from "../persistence/repositories/subscription-repository.js";
import type { ConnectionCache } from "./connection-cache.js";

/**
 * Info about a dispatch job created inside the transaction,
 * used to build MessagePointers for queue dispatch after commit.
 */
export interface DispatchJobNotification {
	id: string;
	dispatchPoolId: string | null;
	messageGroup: string;
}

/**
 * Event dispatch service interface for use in UnitOfWork.
 */
export interface EventDispatchService {
	buildDispatchJobsForEvent(
		event: DomainEvent,
		clientId: string | null,
		db: PostgresJsDatabase,
	): Promise<DispatchJobNotification[]>;
}

/**
 * Dependencies for creating the EventDispatchService.
 */
export interface EventDispatchServiceDeps {
	readonly subscriptionRepository: SubscriptionRepository;
	readonly connectionCache: ConnectionCache;
}

/**
 * Create an EventDispatchService.
 */
export function createEventDispatchService(
	deps: EventDispatchServiceDeps,
): EventDispatchService {
	const { subscriptionRepository, connectionCache } = deps;

	return {
		async buildDispatchJobsForEvent(
			event: DomainEvent,
			clientId: string | null,
			db: PostgresJsDatabase,
		): Promise<DispatchJobNotification[]> {
			// Find active subscriptions matching this event type code and client scope
			const matchingSubs =
				await subscriptionRepository.findActiveByEventTypeCode(
					event.eventType,
					clientId,
				);

			if (matchingSubs.length === 0) return [];

			// Resolve connections from in-memory cache (DB fallback only on miss)
			const connectionIds = [
				...new Set(matchingSubs.map((s) => s.connectionId)),
			];
			const connectionMap =
				await connectionCache.resolveMany(connectionIds);

			const now = new Date();
			const notifications: DispatchJobNotification[] = [];

			for (const sub of matchingSubs) {
				const connection = connectionMap.get(sub.connectionId);
				if (!connection) continue;

				const jobId = generateRaw();
				const idempotencyKey = `${event.eventId}:${sub.id}`;
				const messageGroup = `${sub.code}:${event.messageGroup}`;

				// If connection is PAUSED, create as PENDING (held); otherwise QUEUED
				const status =
					connection.status === "PAUSED"
						? ("PENDING" as const)
						: ("QUEUED" as const);

				const jobRecord = {
					id: jobId,
					kind: "EVENT" as const,
					code: event.eventType,
					source: event.source,
					subject: event.subject,
					eventId: event.eventId,
					correlationId: event.correlationId,
					targetUrl: connection.endpoint,
					protocol: "HTTP_WEBHOOK" as const,
					dataOnly: sub.dataOnly,
					serviceAccountId: connection.serviceAccountId,
					clientId: clientId,
					subscriptionId: sub.id,
					connectionId: sub.connectionId,
					mode: sub.mode,
					dispatchPoolId: sub.dispatchPoolId,
					messageGroup,
					sequence: sub.sequence,
					timeoutSeconds: sub.timeoutSeconds,
					status,
					maxRetries: sub.maxRetries,
					attemptCount: 0,
					idempotencyKey,
					createdAt: now,
					updatedAt: now,
				};

				// Insert dispatch job
				await db.insert(dispatchJobs).values(jobRecord);

				// Write to dispatch job projection feed for stream-processor projection
				await db.insert(dispatchJobProjectionFeed).values({
					dispatchJobId: jobId,
					operation: "INSERT",
					payload: jobRecord,
				});

				// Only add QUEUED jobs to notifications (PENDING jobs are held)
				if (status === "QUEUED") {
					notifications.push({
						id: jobId,
						dispatchPoolId: sub.dispatchPoolId,
						messageGroup,
					});
				}
			}

			return notifications;
		},
	};
}
