/**
 * MCP Server Definition
 *
 * Registers tools and resources for AI agent access to FlowCatalyst.
 */

import { McpServer, ResourceTemplate } from "@modelcontextprotocol/sdk/server/mcp.js";
import { z } from "zod";
import type { ApiClient } from "./api-client.js";

export function createServer(apiClient: ApiClient): McpServer {
	const server = new McpServer({
		name: "flowcatalyst",
		version: "0.1.0",
	});

	registerTools(server, apiClient);
	registerResources(server, apiClient);

	return server;
}

function registerTools(server: McpServer, api: ApiClient): void {
	// List event types
	server.registerTool(
		"list_event_types",
		{
			title: "List Event Types",
			description:
				"List event types registered in FlowCatalyst. Optionally filter by status, application, subdomain, or aggregate.",
			inputSchema: z.object({
				status: z
					.string()
					.optional()
					.describe("Filter by status (e.g. ACTIVE, ARCHIVED)"),
				application: z
					.string()
					.optional()
					.describe("Filter by application code"),
				subdomain: z
					.string()
					.optional()
					.describe("Filter by subdomain code"),
				aggregate: z
					.string()
					.optional()
					.describe("Filter by aggregate code"),
			}),
		},
		async ({ status, application, subdomain, aggregate }) => {
			const result = await api.listEventTypes({
				status,
				application,
				subdomain,
				aggregate,
			});
			return {
				content: [
					{
						type: "text" as const,
						text: JSON.stringify(result, null, 2),
					},
				],
			};
		},
	);

	// Get event type
	server.registerTool(
		"get_event_type",
		{
			title: "Get Event Type",
			description:
				"Get detailed information about a specific event type, including all schema versions.",
			inputSchema: z.object({
				id: z.string().describe("Event type ID"),
			}),
		},
		async ({ id }) => {
			const result = await api.getEventType(id);
			return {
				content: [
					{
						type: "text" as const,
						text: JSON.stringify(result, null, 2),
					},
				],
			};
		},
	);

	// Get schema
	server.registerTool(
		"get_schema",
		{
			title: "Get Schema",
			description:
				"Get the raw JSON Schema for a specific event type version. Defaults to the CURRENT version.",
			inputSchema: z.object({
				id: z.string().describe("Event type ID"),
				version: z
					.string()
					.optional()
					.describe(
						"Schema version (e.g. '1.0'). If omitted, returns the CURRENT version.",
					),
			}),
		},
		async ({ id, version }) => {
			const eventType = await api.getEventType(id);

			const specVersion = version
				? eventType.specVersions.find((sv) => sv.version === version)
				: eventType.specVersions.find((sv) => sv.status === "CURRENT");

			if (!specVersion) {
				return {
					content: [
						{
							type: "text" as const,
							text: version
								? `Schema version ${version} not found for event type ${eventType.code}`
								: `No CURRENT schema version found for event type ${eventType.code}`,
						},
					],
					isError: true,
				};
			}

			const schema =
				typeof specVersion.schemaContent === "string"
					? specVersion.schemaContent
					: JSON.stringify(specVersion.schemaContent, null, 2);

			return {
				content: [
					{
						type: "text" as const,
						text: schema,
					},
				],
			};
		},
	);

	// Generate code
	server.registerTool(
		"generate_code",
		{
			title: "Generate Code",
			description:
				"Generate language-specific code (TypeScript interface, PHP DTO, Python dataclass, or Java record) from an event type's JSON Schema.",
			inputSchema: z.object({
				id: z.string().describe("Event type ID"),
				language: z
					.enum(["typescript", "php", "python", "java"])
					.describe("Target language"),
				version: z
					.string()
					.optional()
					.describe(
						"Schema version (e.g. '1.0'). If omitted, uses the CURRENT version.",
					),
			}),
		},
		async ({ id, language, version }) => {
			const result = await api.generateCode(id, language, version);
			return {
				content: [
					{
						type: "text" as const,
						text: result.code,
					},
				],
			};
		},
	);

	// List subscriptions
	server.registerTool(
		"list_subscriptions",
		{
			title: "List Subscriptions",
			description:
				"List webhook subscriptions configured in FlowCatalyst (scoped to the authenticated client).",
			inputSchema: z.object({}),
		},
		async () => {
			const result = await api.listSubscriptions();
			return {
				content: [
					{
						type: "text" as const,
						text: JSON.stringify(result, null, 2),
					},
				],
			};
		},
	);

	// Get subscription
	server.registerTool(
		"get_subscription",
		{
			title: "Get Subscription",
			description: "Get detailed information about a specific subscription.",
			inputSchema: z.object({
				id: z.string().describe("Subscription ID"),
			}),
		},
		async ({ id }) => {
			const result = await api.getSubscription(id);
			return {
				content: [
					{
						type: "text" as const,
						text: JSON.stringify(result, null, 2),
					},
				],
			};
		},
	);
}

function registerResources(server: McpServer, api: ApiClient): void {
	// Event types list
	server.registerResource(
		"event-types-list",
		"flowcatalyst://event-types",
		{
			title: "Event Types",
			description: "List of all event types in FlowCatalyst",
			mimeType: "application/json",
		},
		async (uri) => {
			const result = await api.listEventTypes();
			return {
				contents: [
					{
						uri: uri.href,
						mimeType: "application/json",
						text: JSON.stringify(result, null, 2),
					},
				],
			};
		},
	);

	// Single event type
	server.registerResource(
		"event-type",
		new ResourceTemplate("flowcatalyst://event-types/{id}", {
			list: async () => {
				const result = await api.listEventTypes();
				return {
					resources: result.eventTypes.map((et) => ({
						uri: `flowcatalyst://event-types/${et.id}`,
						name: `${et.code} — ${et.name}`,
						description: et.description ?? undefined,
						mimeType: "application/json",
					})),
				};
			},
		}),
		{
			title: "Event Type Detail",
			description: "Detailed information about a specific event type",
			mimeType: "application/json",
		},
		async (uri, { id }) => {
			const result = await api.getEventType(id as string);
			return {
				contents: [
					{
						uri: uri.href,
						mimeType: "application/json",
						text: JSON.stringify(result, null, 2),
					},
				],
			};
		},
	);

	// Subscriptions list
	server.registerResource(
		"subscriptions-list",
		"flowcatalyst://subscriptions",
		{
			title: "Subscriptions",
			description: "List of all subscriptions in FlowCatalyst",
			mimeType: "application/json",
		},
		async (uri) => {
			const result = await api.listSubscriptions();
			return {
				contents: [
					{
						uri: uri.href,
						mimeType: "application/json",
						text: JSON.stringify(result, null, 2),
					},
				],
			};
		},
	);

	// Single subscription
	server.registerResource(
		"subscription",
		new ResourceTemplate("flowcatalyst://subscriptions/{id}", {
			list: async () => {
				const result = await api.listSubscriptions();
				return {
					resources: result.subscriptions.map((sub) => ({
						uri: `flowcatalyst://subscriptions/${sub.id}`,
						name: sub.name,
						description: sub.description ?? undefined,
						mimeType: "application/json",
					})),
				};
			},
		}),
		{
			title: "Subscription Detail",
			description: "Detailed information about a specific subscription",
			mimeType: "application/json",
		},
		async (uri, { id }) => {
			const result = await api.getSubscription(id as string);
			return {
				contents: [
					{
						uri: uri.href,
						mimeType: "application/json",
						text: JSON.stringify(result, null, 2),
					},
				],
			};
		},
	);
}
