#!/usr/bin/env node

/**
 * FlowCatalyst MCP Server
 *
 * Provides AI agents with read-only access to event types,
 * subscriptions, JSON schemas, and generated code.
 *
 * Usage:
 *   FLOWCATALYST_URL=https://... FLOWCATALYST_CLIENT_ID=... FLOWCATALYST_CLIENT_SECRET=... flowcatalyst-mcp
 *
 * Transports:
 *   stdio (default) — for Claude Code, Cursor, etc.
 *   --http           — streamable HTTP on port 3100 (or PORT env)
 */

import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { loadConfig } from "./config.js";
import { TokenManager } from "./auth.js";
import { ApiClient } from "./api-client.js";
import { createServer } from "./server.js";

async function main(): Promise<void> {
	const config = loadConfig();
	const tokenManager = new TokenManager(config);
	const apiClient = new ApiClient(config, tokenManager);
	const server = createServer(apiClient);

	const useHttp = process.argv.includes("--http");

	if (useHttp) {
		const { default: http } = await import("node:http");
		const { StreamableHTTPServerTransport } = await import(
			"@modelcontextprotocol/sdk/server/streamableHttp.js"
		);

		const port = Number(process.env["PORT"] ?? 3100);

		const transport = new StreamableHTTPServerTransport({
			sessionIdGenerator: undefined,
		});

		await server.connect(transport);

		const httpServer = http.createServer(async (req, res) => {
			if (req.method === "POST" && req.url === "/mcp") {
				const chunks: Buffer[] = [];
				for await (const chunk of req) {
					chunks.push(chunk as Buffer);
				}
				const body = JSON.parse(Buffer.concat(chunks).toString());
				await transport.handleRequest(req, res, body);
			} else {
				res.writeHead(404);
				res.end("Not Found");
			}
		});

		httpServer.listen(port, "127.0.0.1", () => {
			console.error(`FlowCatalyst MCP server listening on http://127.0.0.1:${port}/mcp`);
		});
	} else {
		// stdio transport (default)
		const transport = new StdioServerTransport();
		await server.connect(transport);
		console.error("FlowCatalyst MCP server running on stdio");
	}
}

main().catch((err) => {
	console.error("Fatal:", err);
	process.exit(1);
});
