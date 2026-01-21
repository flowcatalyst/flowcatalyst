import { serve } from '@hono/node-server';
import { createLogger, setDefaultLogger } from '@flowcatalyst/logging';
import { env } from './env.js';
import { createApp } from './app.js';

// Initialize logger
const logger = createLogger({
	level: env.LOG_LEVEL,
	serviceName: 'message-router',
	pretty: env.NODE_ENV === 'development',
	base: {
		instanceId: env.INSTANCE_ID,
	},
});
setDefaultLogger(logger);

// Create Hono app
const app = createApp(logger);

// Start server
const server = serve(
	{
		fetch: app.fetch,
		port: env.PORT,
		hostname: env.HOST,
	},
	(info) => {
		logger.info(
			{
				host: info.address,
				port: info.port,
				env: env.NODE_ENV,
				queueType: env.QUEUE_TYPE,
			},
			'Message router started',
		);
	},
);

// Graceful shutdown
const shutdown = async () => {
	logger.info('Shutting down...');
	server.close();
	process.exit(0);
};

process.on('SIGINT', shutdown);
process.on('SIGTERM', shutdown);
