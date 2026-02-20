/**
 * OpenAPI Spec Extraction Script
 *
 * Builds a minimal Fastify instance with only the swagger plugin and route
 * registrations — no database, no OIDC, no bootstrap. Dependencies are
 * satisfied by a recursive Proxy stub since route handlers are never invoked.
 *
 * Usage: tsx scripts/extract-openapi.ts
 */

import { writeFile, mkdir } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import Fastify from "fastify";
import swagger from "@fastify/swagger";
import {
	registerAdminRoutes,
	registerBffRoutes,
	registerSdkRoutes,
	registerMeApiRoutes,
	registerPublicApiRoutes,
	registerPlatformConfigApiRoutes,
	registerDebugBffRoutes,
} from "../src/platform/api/index.js";

const __dirname = dirname(fileURLToPath(import.meta.url));
const outputDir = resolve(__dirname, "../openapi");

/**
 * Recursive Proxy that satisfies any dependency shape.
 * Property access returns another stub; function calls return another stub.
 * Route handlers capture these refs but never call them during registration.
 */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function createStub(): any {
	// eslint-disable-next-line @typescript-eslint/no-empty-function
	return new Proxy(function () {}, {
		get: (_target, prop) => {
			// Prevent the runtime from treating this as a thenable/Promise
			if (prop === "then") return undefined;
			return createStub();
		},
		apply: () => createStub(),
	});
}

async function main() {
	console.log("Extracting OpenAPI spec (no database required)...");

	const fastify = Fastify({ logger: false });

	// Register swagger with the same config as startPlatform
	await fastify.register(swagger, {
		openapi: {
			openapi: "3.1.0",
			info: {
				title: "FlowCatalyst Platform API",
				version: "1.0.0",
				description:
					"IAM, Eventing, and Administration API for the FlowCatalyst platform.",
			},
			servers: [{ url: "/" }],
			components: {
				securitySchemes: {
					bearerAuth: {
						type: "http",
						scheme: "bearer",
						bearerFormat: "JWT",
					},
					cookieAuth: {
						type: "apiKey",
						in: "cookie",
						name: "fc_session",
					},
				},
			},
			security: [{ bearerAuth: [] }],
		},
	});

	// Register all API route groups with stub dependencies.
	// The schemas are declared statically during registration — handlers
	// (which reference deps) are never invoked.
	const stub = createStub();

	await registerAdminRoutes(fastify, stub);
	await registerBffRoutes(fastify, stub);
	await registerSdkRoutes(fastify, stub);
	await registerMeApiRoutes(fastify, stub);
	await registerPublicApiRoutes(fastify, stub);
	await registerPlatformConfigApiRoutes(fastify, stub);
	await registerDebugBffRoutes(fastify, stub);

	await fastify.ready();

	const spec = fastify.swagger();

	if (!spec || !spec.openapi) {
		console.error("Failed to extract OpenAPI spec - swagger() returned empty");
		process.exit(1);
	}

	// Ensure output directory exists
	await mkdir(outputDir, { recursive: true });

	// Write JSON
	const jsonPath = resolve(outputDir, "openapi.json");
	await writeFile(jsonPath, JSON.stringify(spec, null, 2), "utf-8");
	console.log(`Written: ${jsonPath}`);

	// Write YAML (simple JSON-to-YAML conversion)
	const yamlPath = resolve(outputDir, "openapi.yaml");
	const yaml = jsonToYaml(spec);
	await writeFile(yamlPath, yaml, "utf-8");
	console.log(`Written: ${yamlPath}`);

	const routeCount = Object.keys(spec.paths ?? {}).length;
	console.log(`\nOpenAPI spec extracted successfully (${routeCount} paths)`);

	await fastify.close();
}

/**
 * Simple JSON to YAML converter (no dependency needed).
 */
function jsonToYaml(obj: unknown, indent = 0): string {
	const prefix = "  ".repeat(indent);

	if (obj === null || obj === undefined) {
		return "null";
	}

	if (typeof obj === "string") {
		// Quote strings that could be ambiguous
		if (
			obj === "" ||
			obj.includes(":") ||
			obj.includes("#") ||
			obj.includes("\n") ||
			obj.startsWith("{") ||
			obj.startsWith("[") ||
			obj.startsWith('"') ||
			obj.startsWith("'") ||
			obj === "true" ||
			obj === "false" ||
			obj === "null" ||
			/^\d/.test(obj)
		) {
			return JSON.stringify(obj);
		}
		return obj;
	}

	if (typeof obj === "number" || typeof obj === "boolean") {
		return String(obj);
	}

	if (Array.isArray(obj)) {
		if (obj.length === 0) return "[]";
		return obj
			.map((item) => {
				const value = jsonToYaml(item, indent + 1);
				if (typeof item === "object" && item !== null) {
					return `${prefix}- ${value.trimStart()}`;
				}
				return `${prefix}- ${value}`;
			})
			.join("\n");
	}

	if (typeof obj === "object") {
		const entries = Object.entries(obj as Record<string, unknown>);
		if (entries.length === 0) return "{}";

		return entries
			.map(([key, value]) => {
				const yamlKey = /^[a-zA-Z_][a-zA-Z0-9_]*$/.test(key)
					? key
					: JSON.stringify(key);

				if (value === null || value === undefined) {
					return `${prefix}${yamlKey}: null`;
				}

				if (typeof value === "object" && !Array.isArray(value)) {
					const inner = jsonToYaml(value, indent + 1);
					if (inner === "{}") {
						return `${prefix}${yamlKey}: {}`;
					}
					return `${prefix}${yamlKey}:\n${inner}`;
				}

				if (Array.isArray(value)) {
					if (value.length === 0) {
						return `${prefix}${yamlKey}: []`;
					}
					const inner = jsonToYaml(value, indent + 1);
					return `${prefix}${yamlKey}:\n${inner}`;
				}

				return `${prefix}${yamlKey}: ${jsonToYaml(value, indent)}`;
			})
			.join("\n");
	}

	return String(obj);
}

main();
