import type { FastifyPluginAsync } from 'fastify';
import { Type } from '@sinclair/typebox';
import {
	MonitoringHealthResponseSchema,
	QueueStatsSchema,
	PoolStatsSchema,
	WarningSchema,
	WarningAcknowledgeResponseSchema,
	CircuitBreakerStatsSchema,
	CircuitBreakerStateResponseSchema,
	InFlightMessageSchema,
	StandbyStatusResponseSchema,
	TrafficStatusResponseSchema,
	ConsumerHealthResponseSchema,
	StatusResponseSchema,
	OidcDiagnosticsResponseSchema,
	InfrastructureHealthResponseSchema,
	InFlightMessagesQuerySchema,
	ClearOldWarningsQuerySchema,
} from '../schemas/index.js';

export const monitoringRoutes: FastifyPluginAsync = async (fastify) => {
	// GET /monitoring/health
	fastify.get('/health', {
		schema: {
			tags: ['Monitoring'],
			summary: 'Get system health',
			response: { 200: MonitoringHealthResponseSchema },
		},
	}, (request) => {
		return request.services.health.getSystemHealth();
	});

	// GET /monitoring/queue-stats
	fastify.get('/queue-stats', {
		schema: {
			tags: ['Monitoring'],
			summary: 'Get queue statistics',
			response: { 200: Type.Record(Type.String(), QueueStatsSchema) },
		},
	}, (request) => {
		return request.services.queueManager.getQueueStats();
	});

	// GET /monitoring/pool-stats
	fastify.get('/pool-stats', {
		schema: {
			tags: ['Monitoring'],
			summary: 'Get pool statistics',
			response: { 200: Type.Record(Type.String(), PoolStatsSchema) },
		},
	}, (request) => {
		return request.services.queueManager.getPoolStats();
	});

	// GET /monitoring/warnings
	fastify.get('/warnings', {
		schema: {
			tags: ['Monitoring'],
			summary: 'Get all warnings',
			response: { 200: Type.Array(WarningSchema) },
		},
	}, (request) => {
		return request.services.warnings.getAll();
	});

	// GET /monitoring/warnings/unacknowledged
	fastify.get('/warnings/unacknowledged', {
		schema: {
			tags: ['Monitoring'],
			summary: 'Get unacknowledged warnings',
			response: { 200: Type.Array(WarningSchema) },
		},
	}, (request) => {
		return request.services.warnings.getUnacknowledged();
	});

	// GET /monitoring/warnings/severity/:severity
	fastify.get<{ Params: { severity: string } }>('/warnings/severity/:severity', {
		schema: {
			tags: ['Monitoring'],
			summary: 'Get warnings by severity',
			params: Type.Object({ severity: Type.String() }),
			response: { 200: Type.Array(WarningSchema) },
		},
	}, (request) => {
		return request.services.warnings.getBySeverity(request.params.severity);
	});

	// POST /monitoring/warnings/:warningId/acknowledge
	fastify.post<{ Params: { warningId: string } }>('/warnings/:warningId/acknowledge', {
		schema: {
			tags: ['Monitoring'],
			summary: 'Acknowledge a warning',
			params: Type.Object({ warningId: Type.String() }),
			response: {
				200: WarningAcknowledgeResponseSchema,
				404: WarningAcknowledgeResponseSchema,
			},
		},
	}, (request, reply) => {
		const acknowledged = request.services.warnings.acknowledge(request.params.warningId);
		if (acknowledged) {
			return { status: 'success' };
		}
		return reply.code(404).send({ status: 'error', message: 'Warning not found' });
	});

	// DELETE /monitoring/warnings
	fastify.delete('/warnings', {
		schema: {
			tags: ['Monitoring'],
			summary: 'Clear all warnings',
			response: { 200: StatusResponseSchema },
		},
	}, (request) => {
		request.services.warnings.clearAll();
		return { status: 'success' };
	});

	// DELETE /monitoring/warnings/old
	fastify.delete('/warnings/old', {
		schema: {
			tags: ['Monitoring'],
			summary: 'Clear old warnings',
			querystring: ClearOldWarningsQuerySchema,
			response: { 200: StatusResponseSchema },
		},
	}, (request) => {
		const { hours = '24' } = request.query as { hours?: string };
		request.services.warnings.clearOlderThan(Number(hours));
		return { status: 'success' };
	});

	// GET /monitoring/circuit-breakers
	fastify.get('/circuit-breakers', {
		schema: {
			tags: ['Monitoring'],
			summary: 'Get circuit breaker statistics',
			response: { 200: Type.Record(Type.String(), CircuitBreakerStatsSchema) },
		},
	}, (request) => {
		return request.services.circuitBreakers.getAllStats();
	});

	// GET /monitoring/circuit-breakers/:name/state
	fastify.get<{ Params: { name: string } }>('/circuit-breakers/:name/state', {
		schema: {
			tags: ['Monitoring'],
			summary: 'Get circuit breaker state',
			params: Type.Object({ name: Type.String() }),
			response: { 200: CircuitBreakerStateResponseSchema },
		},
	}, (request) => {
		const { name } = request.params;
		const breaker = request.services.circuitBreakers.getAll().get(decodeURIComponent(name));
		if (breaker) {
			return { name, state: breaker.getState() };
		}
		return { name, state: 'UNKNOWN' };
	});

	// POST /monitoring/circuit-breakers/:name/reset
	fastify.post<{ Params: { name: string } }>('/circuit-breakers/:name/reset', {
		schema: {
			tags: ['Monitoring'],
			summary: 'Reset circuit breaker',
			params: Type.Object({ name: Type.String() }),
			response: {
				200: StatusResponseSchema,
				500: StatusResponseSchema,
			},
		},
	}, (request, reply) => {
		const { name } = request.params;
		const success = request.services.circuitBreakers.reset(decodeURIComponent(name));
		if (success) {
			return { status: 'success' };
		}
		return reply.code(500).send({ status: 'error', message: 'Circuit breaker not found' });
	});

	// POST /monitoring/circuit-breakers/reset-all
	fastify.post('/circuit-breakers/reset-all', {
		schema: {
			tags: ['Monitoring'],
			summary: 'Reset all circuit breakers',
			response: { 200: StatusResponseSchema },
		},
	}, (request) => {
		request.services.circuitBreakers.resetAll();
		return { status: 'success' };
	});

	// GET /monitoring/in-flight-messages
	fastify.get('/in-flight-messages', {
		schema: {
			tags: ['Monitoring'],
			summary: 'Get in-flight messages',
			querystring: InFlightMessagesQuerySchema,
			response: { 200: Type.Array(InFlightMessageSchema) },
		},
	}, (request) => {
		const { limit = '100', messageId, poolCode } = request.query as {
			limit?: string;
			messageId?: string;
			poolCode?: string;
		};
		return request.services.queueManager.getInFlightMessages(Number(limit), messageId, poolCode);
	});

	// GET /monitoring/standby-status
	fastify.get('/standby-status', {
		schema: {
			tags: ['Monitoring'],
			summary: 'Get standby status',
			response: { 200: StandbyStatusResponseSchema },
		},
	}, (request) => {
		const stats = request.services.traffic.getStats();
		if (!stats.enabled) {
			return { standbyEnabled: false as const };
		}
		return {
			standbyEnabled: true as const,
			instanceId: process.env['INSTANCE_ID'] || 'unknown',
			role: stats.mode === 'PRIMARY' ? 'PRIMARY' : 'STANDBY',
			redisAvailable: false,
			currentLockHolder: '',
			lastSuccessfulRefresh: null,
			hasWarning: false,
		};
	});

	// GET /monitoring/traffic-status
	fastify.get('/traffic-status', {
		schema: {
			tags: ['Monitoring'],
			summary: 'Get traffic status',
			response: { 200: TrafficStatusResponseSchema },
		},
	}, (request) => {
		const stats = request.services.traffic.getStats();
		if (!stats.enabled) {
			return { enabled: false as const, message: 'Traffic management not enabled' };
		}
		return {
			enabled: true as const,
			strategyType: stats.strategyName,
			registered: stats.isRegistered,
			targetInfo: stats.mode,
			lastOperation: '',
			lastError: '',
		};
	});

	// POST /monitoring/become-primary
	fastify.post('/become-primary', {
		schema: {
			tags: ['Monitoring'],
			summary: 'Switch to PRIMARY mode',
			response: {
				200: StatusResponseSchema,
				500: StatusResponseSchema,
			},
		},
	}, async (request, reply) => {
		const result = await request.services.traffic.becomePrimary();
		return result.match(
			() => ({ status: 'success', mode: 'PRIMARY' }),
			(error) => reply.code(500).send({ status: 'error', message: `Failed to become primary: ${error.type}` }),
		);
	});

	// POST /monitoring/become-standby
	fastify.post('/become-standby', {
		schema: {
			tags: ['Monitoring'],
			summary: 'Switch to STANDBY mode',
			response: {
				200: StatusResponseSchema,
				500: StatusResponseSchema,
			},
		},
	}, async (request, reply) => {
		const result = await request.services.traffic.becomeStandby();
		return result.match(
			() => ({ status: 'success', mode: 'STANDBY' }),
			(error) => reply.code(500).send({ status: 'error', message: `Failed to become standby: ${error.type}` }),
		);
	});

	// GET /monitoring/consumer-health
	fastify.get('/consumer-health', {
		schema: {
			tags: ['Monitoring'],
			summary: 'Get consumer health',
			response: { 200: ConsumerHealthResponseSchema },
		},
	}, (request) => {
		return request.services.queueManager.getConsumerHealth();
	});

	// GET /monitoring/dashboard
	fastify.get('/dashboard', async (_request, reply) => {
		try {
			const fs = await import('node:fs/promises');
			const path = await import('node:path');
			const { fileURLToPath } = await import('node:url');

			const __filename = fileURLToPath(import.meta.url);
			const __dirname = path.dirname(__filename);
			const dashboardPath = path.join(__dirname, '../../public/dashboard.html');

			const html = await fs.readFile(dashboardPath, 'utf-8');
			return reply.type('text/html').send(html);
		} catch {
			return reply.code(404).send('Dashboard not found');
		}
	});

	// GET /monitoring/oidc-diagnostics
	fastify.get('/oidc-diagnostics', {
		schema: {
			tags: ['Monitoring'],
			summary: 'Get OIDC diagnostics',
			response: { 200: OidcDiagnosticsResponseSchema },
		},
	}, async (request) => {
		const authEnabled = process.env['AUTHENTICATION_ENABLED'] === 'true';
		const authMode = (process.env['AUTHENTICATION_MODE'] || 'NONE') as 'NONE' | 'BASIC' | 'OIDC';
		const issuerUrl = process.env['OIDC_ISSUER_URL'] || null;
		const clientId = process.env['OIDC_CLIENT_ID'] || null;
		const audience = process.env['OIDC_AUDIENCE'] || clientId;

		const oidcConfigured = authEnabled && authMode === 'OIDC' && !!issuerUrl;
		const discoveryEndpoint = issuerUrl
			? `${issuerUrl.endsWith('/') ? issuerUrl : issuerUrl + '/'}/.well-known/openid-configuration`
			: null;

		let jwksUri: string | null = null;
		let discoveryStatus: 'OK' | 'ERROR' | 'NOT_CONFIGURED' = 'NOT_CONFIGURED';
		let discoveryError: string | null = null;

		if (oidcConfigured && issuerUrl) {
			try {
				const wellKnownUrl = issuerUrl.endsWith('/')
					? `${issuerUrl}.well-known/openid-configuration`
					: `${issuerUrl}/.well-known/openid-configuration`;

				const response = await fetch(wellKnownUrl, { signal: AbortSignal.timeout(5000) });
				if (response.ok) {
					const discovery = (await response.json()) as { jwks_uri?: string };
					jwksUri = discovery.jwks_uri || null;
					discoveryStatus = 'OK';
				} else {
					discoveryStatus = 'ERROR';
					discoveryError = `HTTP ${response.status}: ${response.statusText}`;
				}
			} catch (error) {
				discoveryStatus = 'ERROR';
				discoveryError = error instanceof Error ? error.message : 'Unknown error';
				request.log.warn({ err: error, issuerUrl }, 'OIDC discovery check failed');
			}
		}

		return {
			authenticationEnabled: authEnabled,
			authenticationMode: authMode,
			oidcConfigured,
			issuerUrl,
			clientId,
			audience,
			discoveryEndpoint,
			jwksUri,
			discoveryStatus,
			discoveryError,
		};
	});

	// GET /monitoring/infrastructure-health
	fastify.get('/infrastructure-health', {
		schema: {
			tags: ['Monitoring'],
			summary: 'Get infrastructure health',
			response: { 200: InfrastructureHealthResponseSchema },
		},
	}, (request) => {
		const poolStats = request.services.queueManager.getPoolStats();
		const checks: Array<{ name: string; healthy: boolean; message: string }> = [];

		const poolCount = Object.keys(poolStats).length;
		checks.push({
			name: 'pools_exist',
			healthy: poolCount > 0,
			message: poolCount > 0 ? `${poolCount} pools configured` : 'No pools configured',
		});

		const stalledPools: string[] = [];
		for (const [poolCode, stats] of Object.entries(poolStats)) {
			if (stats.queueSize > 0 && stats.activeWorkers === 0) {
				stalledPools.push(poolCode);
			}
		}

		checks.push({
			name: 'pools_active',
			healthy: stalledPools.length === 0,
			message:
				stalledPools.length === 0
					? 'All pools are active'
					: `Stalled pools (have queued messages but no active workers): ${stalledPools.join(', ')}`,
		});

		const queueManagerRunning = request.services.queueManager.isRunning();
		checks.push({
			name: 'queue_manager_running',
			healthy: queueManagerRunning,
			message: queueManagerRunning ? 'Queue manager is running' : 'Queue manager is not running',
		});

		const healthy = checks.every((check) => check.healthy);
		return { healthy, checks, timestamp: new Date().toISOString() };
	});
};
