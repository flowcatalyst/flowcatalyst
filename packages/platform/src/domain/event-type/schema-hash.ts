/**
 * Schema Hash Utility
 *
 * Computes a SHA-256 hash of a JSON Schema for change detection.
 * Uses canonical JSON (sorted keys) so that logically-identical
 * schemas always produce the same hash regardless of property order.
 */

import { createHash } from "node:crypto";

/**
 * Recursively sort object keys for canonical JSON serialisation.
 */
function sortKeys(value: unknown): unknown {
	if (value === null || typeof value !== "object") {
		return value;
	}
	if (Array.isArray(value)) {
		return value.map(sortKeys);
	}
	const sorted: Record<string, unknown> = {};
	for (const key of Object.keys(value as Record<string, unknown>).sort()) {
		sorted[key] = sortKeys((value as Record<string, unknown>)[key]);
	}
	return sorted;
}

/**
 * Compute a SHA-256 hash of a schema object.
 *
 * The schema is first canonicalised (keys sorted recursively)
 * then serialised to JSON and hashed.
 */
export function computeSchemaHash(schema: unknown): string {
	const canonical = JSON.stringify(sortKeys(schema));
	return createHash("sha256").update(canonical).digest("hex");
}
