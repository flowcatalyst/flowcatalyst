import Fastify, { type FastifyInstance } from 'fastify';
import type { Logger } from '@flowcatalyst/logging';
import { createServices, type Services } from './services/index.js';
import { servicesPlugin } from './plugins/services-plugin.js';
import { authPlugin, type AuthConfig } from './plugins/auth-plugin.js';
import { healthRoutes } from './routes/health.js';
import { configRoutes } from './routes/config.js';
import { monitoringRoutes } from './routes/monitoring.js';
import { testRoutes } from './routes/test.js';
import { seedRoutes } from './routes/seed.js';
import { metricsRoutes } from './routes/metrics.js';
import { benchmarkRoutes } from './routes/benchmark.js';
import { env } from './env.js';

/**
 * Create the Fastify application with all routes
 */
export async function createApp(logger: Logger): Promise<{ app: FastifyInstance; services: Services }> {
	const app = Fastify({
		logger: false, // We use our own pino logger
	});

	// Initialize services
	const services = createServices(logger);

	// Register plugins
	await app.register(servicesPlugin, { services });

	// Auth plugin
	const authConfig: AuthConfig = {
		enabled: env.AUTHENTICATION_ENABLED,
		mode: env.AUTHENTICATION_MODE,
		basic:
			env.AUTH_BASIC_USERNAME && env.AUTH_BASIC_PASSWORD
				? { username: env.AUTH_BASIC_USERNAME, password: env.AUTH_BASIC_PASSWORD }
				: undefined,
		oidc: env.OIDC_ISSUER_URL
			? {
					issuerUrl: env.OIDC_ISSUER_URL,
					clientId: env.OIDC_CLIENT_ID,
					audience: env.OIDC_AUDIENCE || env.OIDC_CLIENT_ID,
				}
			: undefined,
	};
	await app.register(authPlugin, { config: authConfig, logger });

	// Request logging hook
	app.addHook('onResponse', (request, reply, done) => {
		const duration = reply.elapsedTime;
		// Skip logging for health checks in production unless slow
		if (!request.url.startsWith('/health') || duration > 100) {
			logger.info(
				{
					method: request.method,
					path: request.url,
					status: reply.statusCode,
					duration: Math.round(duration),
				},
				'Request completed',
			);
		}
		done();
	});

	// Static assets
	app.get('/tailwind.css', async (_request, reply) => {
		try {
			const fs = await import('node:fs/promises');
			const path = await import('node:path');
			const { fileURLToPath } = await import('node:url');

			const __filename = fileURLToPath(import.meta.url);
			const __dirname = path.dirname(__filename);
			const cssPath = path.join(__dirname, '../public/tailwind.css');

			const css = await fs.readFile(cssPath, 'utf-8');
			return reply.type('text/css').send(css);
		} catch {
			return reply.code(404).send('Not found');
		}
	});

	// Mount routes - exact paths matching Java API
	// Public routes (no auth required)
	await app.register(healthRoutes, { prefix: '/health' });
	await app.register(metricsRoutes, { prefix: '/metrics' });

	// Protected routes (auth required if enabled - handled by auth plugin hook)
	await app.register(configRoutes, { prefix: '/api/config' });
	await app.register(testRoutes, { prefix: '/api/test' });
	await app.register(seedRoutes, { prefix: '/api/seed' });
	await app.register(benchmarkRoutes, { prefix: '/api/benchmark' });
	await app.register(monitoringRoutes, { prefix: '/monitoring' });

	// OpenAPI documentation
	await app.register(import('@fastify/swagger'), {
		openapi: {
			openapi: '3.1.0',
			info: {
				title: 'FlowCatalyst Message Router API',
				version: '1.0.0',
				description: 'Message routing and processing service',
			},
		},
	});
	await app.register(import('@fastify/swagger-ui'), {
		routePrefix: '/docs',
	});

	// Error handler
	app.setErrorHandler((error, request, reply) => {
		const err = error as Error & { statusCode?: number };
		logger.error({ err, path: request.url }, 'Unhandled error');
		return reply.code(err.statusCode ?? 500).send({
			status: 'error',
			message: err.message,
		});
	});

	return { app, services };
}
