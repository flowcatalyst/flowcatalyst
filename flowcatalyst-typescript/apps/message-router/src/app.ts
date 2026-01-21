import { OpenAPIHono } from '@hono/zod-openapi';
import type { Logger } from '@flowcatalyst/logging';
import { healthRoutes } from './api/health.js';
import { configRoutes } from './api/config.js';
import { monitoringRoutes } from './api/monitoring.js';
import { testRoutes } from './api/test.js';
import { seedRoutes } from './api/seed.js';
import { metricsRoutes } from './api/metrics.js';
import { benchmarkRoutes } from './api/benchmark.js';
import { createServices, type Services } from './services/index.js';
import { createAuthMiddleware, type AuthConfig, type AuthUser } from './security/index.js';
import { env } from './env.js';

/**
 * Application context available in all routes
 */
export interface AppContext {
	Variables: {
		logger: Logger;
		services: Services;
		user?: AuthUser;
	};
}

/**
 * Create the Hono application with all routes
 */
export function createApp(logger: Logger) {
	const app = new OpenAPIHono<AppContext>();

	// Initialize services
	const services = createServices(logger);

	// Middleware to inject logger and services
	app.use('*', async (c, next) => {
		c.set('logger', logger);
		c.set('services', services);
		await next();
	});

	// Request logging middleware
	app.use('*', async (c, next) => {
		const start = Date.now();
		await next();
		const duration = Date.now() - start;

		// Skip logging for health checks in production
		if (!c.req.path.startsWith('/health') || duration > 100) {
			logger.info(
				{
					method: c.req.method,
					path: c.req.path,
					status: c.res.status,
					duration,
				},
				'Request completed',
			);
		}
	});

	// Static assets
	app.get('/tailwind.css', async (c) => {
		try {
			const fs = await import('node:fs/promises');
			const path = await import('node:path');
			const { fileURLToPath } = await import('node:url');

			const __filename = fileURLToPath(import.meta.url);
			const __dirname = path.dirname(__filename);
			const cssPath = path.join(__dirname, '../public/tailwind.css');

			const css = await fs.readFile(cssPath, 'utf-8');
			return c.text(css, 200, { 'Content-Type': 'text/css' });
		} catch {
			return c.text('Not found', 404);
		}
	});

	// Create authentication middleware
	const authConfig: AuthConfig = {
		enabled: env.AUTHENTICATION_ENABLED,
		mode: env.AUTHENTICATION_MODE,
		basic: env.AUTH_BASIC_USERNAME && env.AUTH_BASIC_PASSWORD
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

	const authMiddleware = createAuthMiddleware(authConfig, logger);

	// Apply authentication to protected routes
	app.use('/api/*', authMiddleware);
	app.use('/monitoring/*', authMiddleware);

	// Mount routes - exact paths matching Java API
	// Public routes (no auth required)
	app.route('/health', healthRoutes);
	app.route('/metrics', metricsRoutes);

	// Protected routes (auth required if enabled)
	app.route('/api/config', configRoutes);
	app.route('/api/test', testRoutes);
	app.route('/api/seed', seedRoutes);
	app.route('/api/benchmark', benchmarkRoutes);
	app.route('/monitoring', monitoringRoutes);

	// OpenAPI documentation
	app.doc('/openapi.json', {
		openapi: '3.1.0',
		info: {
			title: 'FlowCatalyst Message Router API',
			version: '1.0.0',
			description: 'Message routing and processing service',
		},
	});

	// Error handler
	app.onError((err, c) => {
		logger.error({ err, path: c.req.path }, 'Unhandled error');
		return c.json(
			{
				status: 'error',
				message: err.message,
			},
			500,
		);
	});

	return app;
}
