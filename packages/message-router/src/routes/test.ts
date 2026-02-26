import type { FastifyPluginAsync } from "fastify";
import {
	TestEndpointResponseSchema,
	MediationResponseSchema,
	TestStatsResponseSchema,
	TestStatsResetResponseSchema,
} from "../schemas/index.js";

function sleep(ms: number): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, ms));
}

// Request counter for test endpoints
let requestCounter = 0;

export const testRoutes: FastifyPluginAsync = async (fastify) => {
	// POST /api/test/fast
	fastify.post(
		"/fast",
		{
			schema: {
				tags: ["Test"],
				summary: "Fast response endpoint",
				response: { 200: TestEndpointResponseSchema },
			},
		},
		async () => {
			const requestId = ++requestCounter;
			await sleep(100);
			return { status: "success", endpoint: "fast", requestId };
		},
	);

	// POST /api/test/slow
	fastify.post(
		"/slow",
		{
			schema: {
				tags: ["Test"],
				summary: "Slow response endpoint",
				response: { 200: TestEndpointResponseSchema },
			},
		},
		async () => {
			const requestId = ++requestCounter;
			await sleep(60000);
			return { status: "success", endpoint: "slow", requestId };
		},
	);

	// POST /api/test/faulty
	fastify.post(
		"/faulty",
		{
			schema: {
				tags: ["Test"],
				summary: "Faulty endpoint",
				response: {
					200: TestEndpointResponseSchema,
					400: TestEndpointResponseSchema,
					500: TestEndpointResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const requestId = ++requestCounter;
			const body = (request.body ?? {}) as { messageId?: string };
			const messageId = body.messageId || "unknown";
			const random = Math.random();

			if (random < 0.6) {
				return { status: "success", endpoint: "faulty", requestId, messageId };
			} else if (random < 0.8) {
				return reply.code(400).send({
					status: "error",
					endpoint: "faulty",
					requestId,
					messageId,
					error: "Bad Request",
				});
			} else {
				return reply.code(500).send({
					status: "error",
					endpoint: "faulty",
					requestId,
					messageId,
					error: "Internal Server Error",
				});
			}
		},
	);

	// POST /api/test/fail
	fastify.post(
		"/fail",
		{
			schema: {
				tags: ["Test"],
				summary: "Always fails endpoint",
				response: { 500: TestEndpointResponseSchema },
			},
		},
		(_request, reply) => {
			const requestId = ++requestCounter;
			return reply.code(500).send({
				status: "error",
				endpoint: "fail",
				requestId,
				error: "Always fails",
			});
		},
	);

	// POST /api/test/success
	fastify.post(
		"/success",
		{
			schema: {
				tags: ["Test"],
				summary: "Mediation success endpoint",
				response: { 200: MediationResponseSchema },
			},
		},
		() => {
			requestCounter++;
			return { ack: true, message: "" };
		},
	);

	// POST /api/test/pending
	fastify.post(
		"/pending",
		{
			schema: {
				tags: ["Test"],
				summary: "Mediation pending endpoint",
				response: { 200: MediationResponseSchema },
			},
		},
		() => {
			requestCounter++;
			return { ack: false, message: "notBefore time not reached" };
		},
	);

	// POST /api/test/client-error
	fastify.post(
		"/client-error",
		{
			schema: {
				tags: ["Test"],
				summary: "Client error endpoint",
				response: { 400: TestEndpointResponseSchema },
			},
		},
		(_request, reply) => {
			const requestId = ++requestCounter;
			return reply.code(400).send({
				status: "error",
				endpoint: "client-error",
				requestId,
				error: "Record not found",
			});
		},
	);

	// POST /api/test/server-error
	fastify.post(
		"/server-error",
		{
			schema: {
				tags: ["Test"],
				summary: "Server error endpoint",
				response: { 500: TestEndpointResponseSchema },
			},
		},
		(_request, reply) => {
			const requestId = ++requestCounter;
			return reply.code(500).send({
				status: "error",
				endpoint: "server-error",
				requestId,
			});
		},
	);

	// GET /api/test/stats
	fastify.get(
		"/stats",
		{
			schema: {
				tags: ["Test"],
				summary: "Get request statistics",
				response: { 200: TestStatsResponseSchema },
			},
		},
		() => {
			return { totalRequests: requestCounter };
		},
	);

	// POST /api/test/stats/reset
	fastify.post(
		"/stats/reset",
		{
			schema: {
				tags: ["Test"],
				summary: "Reset request statistics",
				response: { 200: TestStatsResetResponseSchema },
			},
		},
		() => {
			const previousCount = requestCounter;
			requestCounter = 0;
			return { previousCount, currentCount: 0 };
		},
	);
};
