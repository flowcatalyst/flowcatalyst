import type { FastifyPluginAsync } from "fastify";
import { LocalConfigResponseSchema } from "../schemas/index.js";

export const configRoutes: FastifyPluginAsync = async (fastify) => {
	fastify.get(
		"/",
		{
			schema: {
				tags: ["Configuration"],
				summary: "Get local configuration",
				response: { 200: LocalConfigResponseSchema },
			},
		},
		(request) => {
			return request.services.queueManager.getConfig();
		},
	);
};
