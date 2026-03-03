/**
 * Sync Platform Schemas
 *
 * After the event-type sync creates/updates event type entries,
 * this function attaches TypeBox-derived JSON Schemas to each one.
 *
 * Versioning logic:
 * - No spec versions → insert version 1.0 as CURRENT
 * - Latest CURRENT hash matches → skip (unchanged)
 * - Latest CURRENT hash differs → insert next minor version as CURRENT, deprecate previous
 */

import type { TObject } from "@sinclair/typebox";
import { generate } from "@flowcatalyst/tsid";
import type { EventTypeRepository } from "../../../infrastructure/persistence/index.js";
import { computeSchemaHash } from "../../../domain/event-type/schema-hash.js";
import type { SpecVersion, NewSpecVersion } from "../../../domain/index.js";

export interface SchemaSyncResult {
	readonly schemasCreated: number;
	readonly schemasUpdated: number;
	readonly schemasUnchanged: number;
}

const MIME_TYPE = "application/schema+json";
const SCHEMA_TYPE = "JSON_SCHEMA" as const;

/**
 * Sync TypeBox schemas to event type spec versions.
 */
export async function syncPlatformSchemas(
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	schemas: ReadonlyMap<string, TObject<any>>,
	eventTypeRepository: EventTypeRepository,
): Promise<SchemaSyncResult> {
	let schemasCreated = 0;
	let schemasUpdated = 0;
	let schemasUnchanged = 0;

	for (const [code, schema] of schemas) {
		const eventType = await eventTypeRepository.findByCode(code);
		if (!eventType) continue;

		const newHash = computeSchemaHash(schema);
		const specVersions = eventType.specVersions;
		const currentVersion = specVersions.find((sv) => sv.status === "CURRENT");

		if (!currentVersion) {
			// No CURRENT version — create 1.0
			const newSpec: NewSpecVersion = {
				id: generate("SCHEMA"),
				eventTypeId: eventType.id,
				version: "1.0",
				mimeType: MIME_TYPE,
				schemaContent: schema,
				schemaType: SCHEMA_TYPE,
				status: "CURRENT",
			};
			await eventTypeRepository.insertSpecVersion(newSpec);
			schemasCreated++;
		} else {
			// Compare hashes
			const existingHash = computeSchemaHash(currentVersion.schemaContent);

			if (existingHash === newHash) {
				schemasUnchanged++;
			} else {
				// Deprecate previous CURRENT
				const deprecated: SpecVersion = {
					...currentVersion,
					status: "DEPRECATED",
					updatedAt: new Date(),
				};
				await eventTypeRepository.updateSpecVersion(deprecated);

				// Create next minor version
				const nextVersion = bumpMinorVersion(currentVersion.version);
				const newSpec: NewSpecVersion = {
					id: generate("SCHEMA"),
					eventTypeId: eventType.id,
					version: nextVersion,
					mimeType: MIME_TYPE,
					schemaContent: schema,
					schemaType: SCHEMA_TYPE,
					status: "CURRENT",
				};
				await eventTypeRepository.insertSpecVersion(newSpec);
				schemasUpdated++;
			}
		}
	}

	return { schemasCreated, schemasUpdated, schemasUnchanged };
}

/**
 * Bump the minor version: "1.0" → "1.1", "1.5" → "1.6", etc.
 */
function bumpMinorVersion(version: string): string {
	const [major, minor] = version.split(".");
	return `${major}.${parseInt(minor!, 10) + 1}`;
}
