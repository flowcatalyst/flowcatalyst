/**
 * HTTP Layer Types
 *
 * Type definitions for the HTTP layer including Fastify request decorators,
 * context variables, and common interfaces.
 */

import type { FastifyRequest, FastifyReply } from 'fastify';
import type { PrincipalInfo, ExecutionContext } from '@flowcatalyst/domain-core';
import type { Logger } from 'pino';

/**
 * Tracing data stored in request context.
 */
export interface TracingData {
	/** Correlation ID for distributed tracing (from header or generated) */
	readonly correlationId: string;
	/** Causation ID linking to parent event (from header, may be null) */
	readonly causationId: string | null;
	/** Unique execution ID for this request */
	readonly executionId: string;
	/** Request start time */
	readonly startTime: number;
}

/**
 * Audit data stored in request context.
 */
export interface AuditData {
	/** The authenticated principal ID (null if not authenticated) */
	readonly principalId: string | null;
	/** Full principal information (loaded lazily) */
	readonly principal: PrincipalInfo | null;
}

/**
 * Configuration for the tracing plugin.
 */
export interface TracingPluginOptions {
	/** Header name for correlation ID (default: X-Correlation-ID) */
	readonly correlationIdHeader?: string;
	/** Alternative header name for correlation ID (default: X-Request-ID) */
	readonly requestIdHeader?: string;
	/** Header name for causation ID (default: X-Causation-ID) */
	readonly causationIdHeader?: string;
	/** Whether to add correlation ID to response headers (default: true) */
	readonly propagateToResponse?: boolean;
}

/**
 * Configuration for the audit plugin.
 */
export interface AuditPluginOptions {
	/** Cookie name for session token (default: session) */
	readonly sessionCookieName?: string;
	/** Paths to skip authentication (e.g., /health, /metrics) */
	readonly skipPaths?: string[];
	/** Function to validate JWT and extract principal ID */
	readonly validateToken: (token: string) => Promise<string | null>;
	/** Function to load principal by ID (optional, for full principal loading) */
	readonly loadPrincipal?: (principalId: string) => Promise<PrincipalInfo | null>;
}

/**
 * Standard error response format.
 */
export interface ErrorResponse {
	/** Human-readable error message */
	readonly message: string;
	/** Machine-readable error code */
	readonly code: string;
	/** Additional error details */
	readonly details?: Record<string, unknown>;
}

/**
 * Fastify request augmentation for FlowCatalyst applications.
 */
declare module 'fastify' {
	interface FastifyRequest {
		/** Tracing context for distributed tracing */
		tracing: TracingData;
		/** Audit context for authentication/authorization */
		audit: AuditData;
		/** Execution context for use case calls */
		executionContext: ExecutionContext;
	}
}

export type { FastifyRequest, FastifyReply, Logger };
